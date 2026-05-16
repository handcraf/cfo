// Package service — deterministic reranker for evidence chunks.
//
// Why deterministic and not a cross-encoder model?
//
// AGENTS.md: "Backend decides facts. LLM explains facts." A reranker that
// reorders evidence using an opaque neural model would blur the boundary
// between backend logic and model-ish reasoning. Here we use an auditable
// scoring function — every boost has a reason a user can read in the logs.
//
// A cross-encoder rerank can be added later as an optional "quality boost"
// stage (see TODO at end of file).
package service

import (
	"log"
	"sort"
	"strings"

	"github.com/cfo/backend/internal/model"
)

// RerankSignals lets the caller tell the reranker what the "ideal" chunk
// looks like. A chunk gets boosts for matching these signals.
//
// Pass the zero value to disable a signal.
type RerankSignals struct {
	Industry    model.IndustryType
	DocType     model.DocType
	PeriodStart string // YYYY-MM-DD
	PeriodEnd   string
	// PreferredSources is a list of filenames that should rank higher,
	// e.g., when a deterministic metric calc already cited a document.
	PreferredSources []string
}

// RerankWeights controls the contribution of each signal. Defaults are
// intentionally modest; the base score (from RRF) still dominates.
type RerankWeights struct {
	Base            float32 // multiplier on the incoming fused score (default 1.0)
	IndustryBoost   float32 // added if chunk.Industry matches (default 0.15)
	DocTypeBoost    float32 // added if chunk.DocType matches (default 0.10)
	PeriodBoost     float32 // added if chunk's period overlaps target period (default 0.20)
	SourceBoost     float32 // added if chunk.Source is in PreferredSources (default 0.10)
	HybridBothBoost float32 // added if chunk was found by BOTH retrievers (default 0.05)
	LengthPenalty   float32 // subtracted for chunks over LengthPenaltyChars (default 0.05)
}

// LengthPenaltyChars is the soft ceiling beyond which long chunks get
// penalized. Keeps context windows tight and discourages dumping raw pages.
const LengthPenaltyChars = 1500

// DefaultRerankWeights returns the recommended baseline.
func DefaultRerankWeights() RerankWeights {
	return RerankWeights{
		Base:            1.0,
		IndustryBoost:   0.15,
		DocTypeBoost:    0.10,
		PeriodBoost:     0.20,
		SourceBoost:     0.10,
		HybridBothBoost: 0.05,
		LengthPenalty:   0.05,
	}
}

// Reranker computes a new Score per chunk and re-sorts.
type Reranker struct {
	weights RerankWeights
}

// NewReranker builds a reranker with the given weights. Pass
// DefaultRerankWeights() if you don't have strong opinions.
func NewReranker(w RerankWeights) *Reranker { return &Reranker{weights: w} }

// Rerank returns chunks re-sorted by an adjusted score, capped to topK.
// The input is not mutated.
//
// Each chunk's final score is:
//
//	score = base * chunk.Score
//	      + industryBoost if chunk.Industry == signals.Industry
//	      + docTypeBoost  if chunk.DocType  == signals.DocType
//	      + periodBoost   if chunk period overlaps signals period
//	      + sourceBoost   if chunk.Source in signals.PreferredSources
//	      + hybridBothBoost if chunk was in both semantic and keyword results
//	      - lengthPenalty if len(chunk.Text) > LengthPenaltyChars
//
// Every boost decision is logged at debug level so the retrieval is auditable.
func (r *Reranker) Rerank(chunks []EvidenceChunk, signals RerankSignals, topK int) []EvidenceChunk {
	if topK <= 0 {
		topK = 5
	}

	out := make([]EvidenceChunk, len(chunks))
	copy(out, chunks)

	for i := range out {
		c := &out[i]
		before := c.Score
		score := r.weights.Base * c.Score

		if signals.Industry != "" && c.Industry == signals.Industry {
			score += r.weights.IndustryBoost
		}
		if signals.DocType != "" && c.DocType == signals.DocType {
			score += r.weights.DocTypeBoost
		}
		if signals.PeriodStart != "" && signals.PeriodEnd != "" &&
			c.PeriodStart != "" && c.PeriodEnd != "" &&
			periodsOverlap(c.PeriodStart, c.PeriodEnd, signals.PeriodStart, signals.PeriodEnd) {
			score += r.weights.PeriodBoost
		}
		if len(signals.PreferredSources) > 0 && containsSource(signals.PreferredSources, c.Source) {
			score += r.weights.SourceBoost
		}
		if c.SemanticRank > 0 && c.KeywordRank > 0 {
			score += r.weights.HybridBothBoost
		}
		if len(c.Text) > LengthPenaltyChars {
			score -= r.weights.LengthPenalty
		}

		c.Score = score
		if score != before {
			log.Printf("[Rerank] chunk=%s base=%.4f final=%.4f (semRank=%d kwRank=%d period=%s..%s doctype=%s industry=%s)",
				c.ID, before, score, c.SemanticRank, c.KeywordRank, c.PeriodStart, c.PeriodEnd, c.DocType, c.Industry)
		}
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })

	if len(out) > topK {
		out = out[:topK]
	}
	return out
}

func containsSource(sources []string, s string) bool {
	for _, v := range sources {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// TODO: Optional cross-encoder rerank stage.
// Add a second-pass reranker that calls a local cross-encoder model via
// Ollama (e.g., `ms-marco-MiniLM` port). Gated by config flag; must remain
// optional because it violates the "backend is auditable" principle and
// requires explicit opt-in.
//
// TODO: Learn-to-rank from ask_audit.
// When the audit log grows large enough, expose a weights-tuning CLI that
// uses historical (query, top-K, user-approved) tuples to fit RerankWeights
// with logistic regression. Still deterministic at inference time.
