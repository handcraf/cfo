package service

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// Context limits for LLM (llama3 supports ~8K tokens, we use ~6K for safety)
const (
	MaxContextChars   = 20000 // ~5K tokens
	MaxChunksToUse    = 15    // Max chunks to include in context
	MinRelevanceScore = 1     // Minimum score to include a chunk
)

// ScoredChunk represents a text chunk with its relevance score
type ScoredChunk struct {
	Chunk      model.TextChunk
	Score      int
	DocumentID string
	Filename   string
}

// RAGService handles retrieval-augmented generation
type RAGService struct {
	store *storage.FileStore
}

// NewRAGService creates a new RAGService
func NewRAGService(store *storage.FileStore) *RAGService {
	return &RAGService{store: store}
}

// SearchResult contains the search results for the LLM
type SearchResult struct {
	Context     string   // Combined relevant text
	Sources     []string // Document IDs used
	SourceFiles []string // Original filenames
	ChunksUsed  int      // Number of chunks included
	TotalChunks int      // Total chunks searched
	Truncated   bool     // Whether context was truncated
}

// Search performs enhanced search across all documents
// Returns relevant context from multiple documents for the LLM
func (r *RAGService) Search(query string) (string, []string) {
	result := r.SearchEnhanced(query)
	return result.Context, result.Sources
}

// SearchEnhanced performs enhanced search with detailed results
func (r *RAGService) SearchEnhanced(query string) SearchResult {
	result := SearchResult{
		Sources:     []string{},
		SourceFiles: []string{},
	}

	docs, err := r.store.LoadAllParsedDocuments()
	if err != nil || len(docs) == 0 {
		return result
	}

	// Extract keywords from query
	keywords := r.extractKeywords(query)

	// Score and collect all chunks from all documents
	var allScoredChunks []ScoredChunk

	for _, doc := range docs {
		// Score chunks
		for _, chunk := range doc.Chunks {
			score := r.scoreText(chunk.Text, keywords)
			if score >= MinRelevanceScore {
				allScoredChunks = append(allScoredChunks, ScoredChunk{
					Chunk:      chunk,
					Score:      score,
					DocumentID: doc.DocumentID,
					Filename:   doc.Filename,
				})
			}
		}

		// Also score the raw text if no chunks or for additional context
		if len(doc.Chunks) == 0 && doc.RawText != "" {
			score := r.scoreText(doc.RawText, keywords)
			if score >= MinRelevanceScore {
				// Create a chunk from raw text
				allScoredChunks = append(allScoredChunks, ScoredChunk{
					Chunk: model.TextChunk{
						Text:   truncateText(doc.RawText, 1000),
						Source: doc.Filename,
					},
					Score:      score,
					DocumentID: doc.DocumentID,
					Filename:   doc.Filename,
				})
			}
		}

		result.TotalChunks += len(doc.Chunks)
		if len(doc.Chunks) == 0 {
			result.TotalChunks++
		}
	}

	// Sort by relevance score (highest first)
	sort.Slice(allScoredChunks, func(i, j int) bool {
		return allScoredChunks[i].Score > allScoredChunks[j].Score
	})

	// Build context from top chunks
	var contextBuilder strings.Builder
	seenSources := make(map[string]bool)
	seenFiles := make(map[string]bool)
	currentLen := 0

	for _, sc := range allScoredChunks {
		if result.ChunksUsed >= MaxChunksToUse {
			result.Truncated = true
			break
		}

		chunkText := sc.Chunk.Text
		newLen := currentLen + len(chunkText) + 100 // 100 for formatting

		if newLen > MaxContextChars {
			// Try to fit a truncated version
			remaining := MaxContextChars - currentLen - 150
			if remaining > 200 {
				chunkText = truncateText(chunkText, remaining)
			} else {
				result.Truncated = true
				break
			}
		}

		// Add document header if new source
		if !seenFiles[sc.Filename] {
			contextBuilder.WriteString(fmt.Sprintf("\n\n📄 [From: %s]\n", sc.Filename))
			seenFiles[sc.Filename] = true
		}

		contextBuilder.WriteString(chunkText)
		contextBuilder.WriteString("\n---\n")

		currentLen = contextBuilder.Len()
		result.ChunksUsed++

		if !seenSources[sc.DocumentID] {
			result.Sources = append(result.Sources, sc.DocumentID)
			seenSources[sc.DocumentID] = true
		}

		if !seenFiles[sc.Filename] {
			result.SourceFiles = append(result.SourceFiles, sc.Filename)
		}
	}

	result.Context = contextBuilder.String()

	return result
}

// extractKeywords extracts search keywords from a query
func (r *RAGService) extractKeywords(query string) []string {
	query = strings.ToLower(query)

	// Remove common stop words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true, "being": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"may": true, "might": true, "must": true, "can": true,
		"what": true, "how": true, "why": true, "when": true, "where": true,
		"who": true, "which": true, "that": true, "this": true, "these": true,
		"those": true, "my": true, "your": true, "our": true, "their": true,
		"its": true, "i": true, "you": true, "we": true, "they": true,
		"me": true, "him": true, "her": true, "us": true, "them": true,
		"and": true, "or": true, "but": true, "if": true, "then": true,
		"of": true, "to": true, "in": true, "on": true, "at": true,
		"by": true, "for": true, "with": true, "about": true, "from": true,
		"tell": true, "show": true, "give": true, "please": true,
	}

	words := strings.Fields(query)
	var keywords []string

	for _, word := range words {
		word = strings.Trim(word, ".,!?;:'\"")

		if len(word) > 2 && !stopWords[word] {
			keywords = append(keywords, word)
		}
	}

	// Add financial term synonyms
	synonyms := map[string][]string{
		"cash":      {"cash", "money", "funds", "liquidity", "balance"},
		"burn":      {"burn", "spending", "expenses", "costs", "outflow"},
		"runway":    {"runway", "months", "sustainability", "survive"},
		"revenue":   {"revenue", "sales", "income", "earnings", "turnover"},
		"profit":    {"profit", "margin", "earnings", "net income", "gain"},
		"loss":      {"loss", "deficit", "negative", "shortfall"},
		"assets":    {"assets", "property", "equipment", "resources"},
		"debt":      {"debt", "liabilities", "loans", "borrowing", "credit"},
		"balance":   {"balance", "sheet", "position", "statement"},
		"financial": {"financial", "finance", "fiscal", "monetary"},
		"quarter":   {"quarter", "q1", "q2", "q3", "q4", "quarterly"},
		"annual":    {"annual", "yearly", "year", "fy"},
	}

	var expanded []string
	for _, kw := range keywords {
		if syns, ok := synonyms[kw]; ok {
			expanded = append(expanded, syns...)
		} else {
			expanded = append(expanded, kw)
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	var unique []string
	for _, kw := range expanded {
		if !seen[kw] {
			seen[kw] = true
			unique = append(unique, kw)
		}
	}

	return unique
}

// scoreText scores how relevant text is to the keywords
func (r *RAGService) scoreText(text string, keywords []string) int {
	text = strings.ToLower(text)
	score := 0

	for _, kw := range keywords {
		count := strings.Count(text, kw)
		score += count
	}

	return score
}

// truncateText truncates text to a maximum length
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	// Try to truncate at a sentence boundary
	truncated := text[:maxLen]
	lastPeriod := strings.LastIndex(truncated, ".")
	lastNewline := strings.LastIndex(truncated, "\n")

	cutPoint := maxLen
	if lastPeriod > maxLen/2 {
		cutPoint = lastPeriod + 1
	} else if lastNewline > maxLen/2 {
		cutPoint = lastNewline + 1
	}

	return text[:cutPoint] + "..."
}

// SearchWithPeriod performs search filtered by a specific time period
func (r *RAGService) SearchWithPeriod(query string, startDate, endDate string) (string, []string) {
	result := r.SearchEnhancedWithPeriod(query, startDate, endDate)
	return result.Context, result.Sources
}

// SearchEnhancedWithPeriod performs enhanced search filtered by period
func (r *RAGService) SearchEnhancedWithPeriod(query string, startDate, endDate string) SearchResult {
	result := SearchResult{
		Sources:     []string{},
		SourceFiles: []string{},
	}

	docs, err := r.store.LoadAllParsedDocuments()
	if err != nil || len(docs) == 0 {
		return result
	}

	// Create period parser for matching
	periodParser := NewPeriodParser()
	targetPeriod := ParsedPeriod{
		Start:    startDate,
		End:      endDate,
		Detected: true,
	}

	// Extract keywords from query
	keywords := r.extractKeywords(query)

	// Score and collect all chunks from matching documents
	var allScoredChunks []ScoredChunk

	for _, doc := range docs {
		// Filter by period first
		if !periodParser.IsPeriodMatch(doc.Period.Start, doc.Period.End, targetPeriod) {
			continue
		}

		// Score chunks
		for _, chunk := range doc.Chunks {
			score := r.scoreText(chunk.Text, keywords)
			if score >= MinRelevanceScore {
				allScoredChunks = append(allScoredChunks, ScoredChunk{
					Chunk:      chunk,
					Score:      score,
					DocumentID: doc.DocumentID,
					Filename:   doc.Filename,
				})
			}
		}

		// Also include raw text if no chunks
		if len(doc.Chunks) == 0 && doc.RawText != "" {
			score := r.scoreText(doc.RawText, keywords)
			if score >= MinRelevanceScore {
				allScoredChunks = append(allScoredChunks, ScoredChunk{
					Chunk: model.TextChunk{
						Text:   truncateText(doc.RawText, 1000),
						Source: doc.Filename,
					},
					Score:      score,
					DocumentID: doc.DocumentID,
					Filename:   doc.Filename,
				})
			}
		}

		result.TotalChunks += len(doc.Chunks)
		if len(doc.Chunks) == 0 {
			result.TotalChunks++
		}
	}

	// Sort by relevance score (highest first)
	sort.Slice(allScoredChunks, func(i, j int) bool {
		return allScoredChunks[i].Score > allScoredChunks[j].Score
	})

	// Build context from top chunks
	var contextBuilder strings.Builder
	seenSources := make(map[string]bool)
	seenFiles := make(map[string]bool)
	currentLen := 0

	// Add period context header
	contextBuilder.WriteString(fmt.Sprintf("📅 Data for period: %s to %s\n\n", startDate, endDate))

	for _, sc := range allScoredChunks {
		if result.ChunksUsed >= MaxChunksToUse {
			result.Truncated = true
			break
		}

		chunkText := sc.Chunk.Text
		newLen := currentLen + len(chunkText) + 100

		if newLen > MaxContextChars {
			remaining := MaxContextChars - currentLen - 150
			if remaining > 200 {
				chunkText = truncateText(chunkText, remaining)
			} else {
				result.Truncated = true
				break
			}
		}

		if !seenFiles[sc.Filename] {
			contextBuilder.WriteString(fmt.Sprintf("\n\n📄 [From: %s]\n", sc.Filename))
			seenFiles[sc.Filename] = true
		}

		contextBuilder.WriteString(chunkText)
		contextBuilder.WriteString("\n---\n")

		currentLen = contextBuilder.Len()
		result.ChunksUsed++

		if !seenSources[sc.DocumentID] {
			result.Sources = append(result.Sources, sc.DocumentID)
			seenSources[sc.DocumentID] = true
		}
	}

	result.Context = contextBuilder.String()

	return result
}

// GetAllDocumentSummaries returns a summary of all documents for context
func (r *RAGService) GetAllDocumentSummaries() string {
	docs, err := r.store.LoadAllParsedDocuments()
	if err != nil || len(docs) == 0 {
		return "No documents uploaded."
	}

	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("Company has %d document(s) uploaded:\n", len(docs)))

	for i, doc := range docs {
		summary.WriteString(fmt.Sprintf("%d. %s (%s", i+1, doc.Filename, doc.DocType))
		if doc.Period.Start != "" && doc.Period.End != "" {
			summary.WriteString(fmt.Sprintf(", Period: %s to %s", doc.Period.Start, doc.Period.End))
		}
		if doc.PageCount > 0 {
			summary.WriteString(fmt.Sprintf(", %d pages", doc.PageCount))
		}
		summary.WriteString(")\n")
	}

	return summary.String()
}
