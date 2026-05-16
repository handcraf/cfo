// Package service — hybrid retrieval layer.
//
// This retrieves evidence chunks by combining semantic (vector) search and
// keyword (BM25-like) search using Reciprocal Rank Fusion (RRF). RRF is the
// industry-standard hybrid approach because:
//
//   - it is parameter-light (single k constant, typically 60)
//   - it works on ranks, not raw scores, so it's robust to the two
//     retrievers having very different score distributions
//   - it never lets one retriever drown the other
//
// This module is an EVIDENCE layer. It does not compute financial metrics
// and does not choose documents for the financial_logic pipeline. Those
// remain deterministic (see AGENTS.md: "Backend decides facts, LLM explains").
package service

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// RRFConstant is the smoothing constant from Cormack et al. 2009.
// 60 is the community default and is not worth tuning without A/B data.
const RRFConstant = 60

// DefaultHybridTopN is how many candidates we pull from each retriever
// before fusing. Wider than final-K because fusion drops duplicates and
// rerank further trims.
const DefaultHybridTopN = 20

// EvidenceChunk is a retrieval result with enough metadata to be audited.
// It is the ONLY shape allowed to cross from retrieval into ask.go and
// from there into the LLM prompt builder.
//
// The Score is a unit-less fusion score; higher is better. Downstream
// consumers must not interpret Score as a probability.
type EvidenceChunk struct {
	ID           string             `json:"id"`
	DocumentID   string             `json:"document_id"`
	Source       string             `json:"source"`
	Text         string             `json:"text"`
	DocType      model.DocType      `json:"doc_type,omitempty"`
	PeriodStart  string             `json:"period_start,omitempty"`
	PeriodEnd    string             `json:"period_end,omitempty"`
	Industry     model.IndustryType `json:"industry,omitempty"`
	Score        float32            `json:"score"`         // fusion score (post-RRF)
	SemanticRank int                `json:"semantic_rank"` // 1-indexed rank in semantic results; 0 = not present
	KeywordRank  int                `json:"keyword_rank"`  // 1-indexed rank in keyword results; 0 = not present
}

// HybridRetriever runs semantic + keyword search in parallel (well, sequential
// today — see TODO) and fuses results with RRF.
type HybridRetriever struct {
	vectorStore Store
	keywordRAG  *RAGService
	store       *storage.FileStore
}

// NewHybridRetriever wires a retriever from its dependencies.
// Either dependency may be nil; the retriever degrades gracefully:
//   - vectorStore == nil  → keyword-only
//   - keywordRAG  == nil  → semantic-only
//   - both nil            → always returns empty (caller should treat as low-confidence)
func NewHybridRetriever(vs Store, rag *RAGService, store *storage.FileStore) *HybridRetriever {
	return &HybridRetriever{vectorStore: vs, keywordRAG: rag, store: store}
}

// HybridQuery bundles retrieval inputs.
type HybridQuery struct {
	Text   string
	TopN   int // candidates per retriever before fusion
	Filter VectorFilter
}

// Retrieve returns fused candidates ranked by RRF score.
//
// Flow:
//  1. Run semantic search (top-N) with filter applied pre-retrieval.
//  2. Run keyword search (top-N) with same filter applied post-hoc
//     (keyword RAG is file-scan based and can't push filters to storage).
//  3. Fuse with Reciprocal Rank Fusion.
//  4. Sort by fusion score, return top-N.
//
// Reranking and final top-K cutting happen in the reranker, not here.
func (h *HybridRetriever) Retrieve(ctx context.Context, q HybridQuery) ([]EvidenceChunk, error) {
	topN := q.TopN
	if topN <= 0 {
		topN = DefaultHybridTopN
	}

	semanticResults, err := h.runSemantic(ctx, q.Text, topN, q.Filter)
	if err != nil {
		log.Printf("[Hybrid] semantic search error: %v", err)
	}

	keywordResults, err := h.runKeyword(q.Text, topN, q.Filter)
	if err != nil {
		log.Printf("[Hybrid] keyword search error: %v", err)
	}

	fused := rrfFuse(semanticResults, keywordResults, RRFConstant)

	if len(fused) > topN {
		fused = fused[:topN]
	}
	return fused, nil
}

// runSemantic performs vector search through the Store abstraction.
func (h *HybridRetriever) runSemantic(ctx context.Context, text string, topN int, filter VectorFilter) ([]EvidenceChunk, error) {
	if h.vectorStore == nil {
		return nil, nil
	}
	raw, err := h.vectorStore.Search(ctx, SearchQuery{
		Text:   text,
		TopK:   topN,
		Filter: filter,
	})
	if err != nil {
		return nil, err
	}
	out := make([]EvidenceChunk, 0, len(raw))
	for _, r := range raw {
		out = append(out, evidenceFromVector(r))
	}
	return out, nil
}

// runKeyword performs keyword search through the legacy RAGService and
// applies the same VectorFilter. This keeps the two retrievers observing
// identical scope — critical for RRF to be meaningful.
func (h *HybridRetriever) runKeyword(text string, topN int, filter VectorFilter) ([]EvidenceChunk, error) {
	if h.keywordRAG == nil {
		return nil, nil
	}

	// The keyword RAG has two search modes; period-scoped or full. Prefer
	// the period-scoped mode when the filter has a period.
	var basic SearchResult
	if filter.PeriodStart != "" && filter.PeriodEnd != "" {
		basic = h.keywordRAG.SearchEnhancedWithPeriod(text, filter.PeriodStart, filter.PeriodEnd)
	} else {
		basic = h.keywordRAG.SearchEnhanced(text)
	}

	// SearchEnhanced returns a combined context string, not individual chunks.
	// We recover the top chunks by rescanning the store ourselves so we can
	// attach metadata and ranks. This is O(docs) — acceptable at current scale.
	//
	// TODO: Refactor RAGService to expose raw ScoredChunk list so we don't
	// have to re-score here.
	_ = basic // basic.TotalChunks etc. are useful for diagnostics; unused for now.

	if h.store == nil {
		return nil, nil
	}
	docs, err := h.store.LoadAllParsedDocuments()
	if err != nil {
		return nil, fmt.Errorf("load parsed docs for keyword rerun: %w", err)
	}

	keywords := h.keywordRAG.extractKeywords(text)
	type scored struct {
		doc   *model.ParsedDocument
		chunk model.TextChunk
		score int
		idx   int
	}
	var hits []scored
	for _, d := range docs {
		if !passesParsedDocFilter(d, filter) {
			continue
		}
		for i, ch := range d.Chunks {
			s := h.keywordRAG.scoreText(ch.Text, keywords)
			if s >= MinRelevanceScore {
				hits = append(hits, scored{doc: d, chunk: ch, score: s, idx: i})
			}
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > topN {
		hits = hits[:topN]
	}

	out := make([]EvidenceChunk, 0, len(hits))
	for _, h2 := range hits {
		out = append(out, EvidenceChunk{
			ID:          fmt.Sprintf("%s_kw_%d", h2.doc.DocumentID, h2.idx),
			DocumentID:  h2.doc.DocumentID,
			Source:      h2.doc.Filename,
			Text:        h2.chunk.Text,
			DocType:     h2.doc.DocType,
			PeriodStart: h2.doc.Period.Start,
			PeriodEnd:   h2.doc.Period.End,
			Score:       float32(h2.score),
		})
	}
	return out, nil
}

// passesParsedDocFilter is the keyword-path equivalent of matchesFilter.
// ParsedDocument has no IndustryType — that's a company-level attribute,
// so we can't filter on industry here. The caller must supply keyword
// filters that don't depend on industry (or rely on the semantic side
// for industry-scoped retrieval).
func passesParsedDocFilter(d *model.ParsedDocument, f VectorFilter) bool {
	if f.DocType != "" && d.DocType != f.DocType {
		return false
	}
	if f.Source != "" && d.Filename != f.Source {
		return false
	}
	if len(f.DocumentIDs) > 0 {
		found := false
		for _, id := range f.DocumentIDs {
			if d.DocumentID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if f.PeriodStart != "" && f.PeriodEnd != "" {
		if d.Period.Start == "" || d.Period.End == "" {
			return false
		}
		if !periodsOverlap(d.Period.Start, d.Period.End, f.PeriodStart, f.PeriodEnd) {
			return false
		}
	}
	return true
}

// rrfFuse computes Reciprocal Rank Fusion over two ranked lists.
//
// For each unique chunk ID, score = sum over lists of 1/(k + rank).
// Duplicate IDs across retrievers are additive (that's the whole point).
//
// If a chunk appears in only one list, it still scores based on that list's
// rank; there's no penalty for single-source chunks.
//
// The formula uses 1-indexed ranks (rank 1 = top).
func rrfFuse(semantic, keyword []EvidenceChunk, k int) []EvidenceChunk {
	scores := make(map[string]*EvidenceChunk)

	for rank, c := range semantic {
		c := c
		c.SemanticRank = rank + 1
		c.Score = 1.0 / float32(k+rank+1)
		scores[c.ID] = &c
	}

	for rank, c := range keyword {
		bump := float32(1.0 / float64(k+rank+1))
		if existing, ok := scores[c.ID]; ok {
			existing.KeywordRank = rank + 1
			existing.Score += bump
			// Keep the richer metadata from whichever source has more fields.
			if existing.Source == "" {
				existing.Source = c.Source
			}
			if existing.Text == "" {
				existing.Text = c.Text
			}
			continue
		}
		c := c
		c.KeywordRank = rank + 1
		c.Score = bump
		scores[c.ID] = &c
	}

	out := make([]EvidenceChunk, 0, len(scores))
	for _, v := range scores {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		// Deterministic tiebreaker: lower semantic rank wins, then by ID.
		// Semantic rank 0 means "not present"; treat as +inf.
		ai, bi := out[i].SemanticRank, out[j].SemanticRank
		if ai == 0 {
			ai = int(^uint(0) >> 1)
		}
		if bi == 0 {
			bi = int(^uint(0) >> 1)
		}
		if ai != bi {
			return ai < bi
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// evidenceFromVector lifts a SimilarityResult into an EvidenceChunk with
// the similarity treated as the initial score (overwritten by RRF downstream).
func evidenceFromVector(r SimilarityResult) EvidenceChunk {
	return EvidenceChunk{
		ID:          r.Document.ID,
		DocumentID:  r.Document.DocumentID,
		Source:      r.Document.Source,
		Text:        r.Document.Text,
		DocType:     r.Document.DocType,
		PeriodStart: r.Document.PeriodStart,
		PeriodEnd:   r.Document.PeriodEnd,
		Industry:    r.Document.IndustryType,
		Score:       r.Similarity,
	}
}

// JoinTexts is a tiny helper used by the prompt builder. Kept here so the
// EvidenceChunk shape owns its own rendering.
func JoinTexts(chunks []EvidenceChunk, sep string) string {
	parts := make([]string, 0, len(chunks))
	for _, c := range chunks {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, sep)
}
