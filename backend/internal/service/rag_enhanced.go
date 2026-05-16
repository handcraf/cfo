// Package service provides enhanced RAG capabilities using langchaingo.
//
// This module provides semantic search using embeddings instead of keyword matching.
// It integrates with the VectorStore for efficient similarity search.
//
// TODO: Add support for hybrid search (combining keyword and semantic)
// TODO: Add support for re-ranking results
package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// EnhancedRAGService provides semantic search capabilities using embeddings.
// It combines vector similarity search with keyword matching for best results.
type EnhancedRAGService struct {
	store         *storage.FileStore
	vectorStore   *VectorStore
	embedService  *EmbeddingService
	keywordRAG    *RAGService // Fallback to keyword-based search
	useEmbeddings bool
}

// EnhancedRAGConfig holds configuration for the enhanced RAG service
type EnhancedRAGConfig struct {
	Store         *storage.FileStore
	EmbedService  *EmbeddingService
	VectorStore   *VectorStore
	UseEmbeddings bool // Whether to use embeddings (falls back to keyword if false)
}

// NewEnhancedRAGService creates a new enhanced RAG service.
func NewEnhancedRAGService(cfg EnhancedRAGConfig) *EnhancedRAGService {
	return &EnhancedRAGService{
		store:         cfg.Store,
		vectorStore:   cfg.VectorStore,
		embedService:  cfg.EmbedService,
		keywordRAG:    NewRAGService(cfg.Store),
		useEmbeddings: cfg.UseEmbeddings,
	}
}

// EnhancedSearchResult contains detailed search results
type EnhancedSearchResult struct {
	Context       string                 // Combined relevant text for LLM
	Sources       []string               // Document IDs used
	SourceFiles   []string               // Original filenames
	ChunksUsed    int                    // Number of chunks included
	TotalChunks   int                    // Total chunks searched
	Truncated     bool                   // Whether context was truncated
	SearchMethod  string                 // "semantic", "keyword", or "hybrid"
	TopScores     []float32              // Similarity scores of top results
	IndustryHits  map[model.IndustryType]int // Hits per industry
}

// Search performs enhanced search using embeddings if available.
func (r *EnhancedRAGService) Search(ctx context.Context, query string) (*EnhancedSearchResult, error) {
	return r.SearchWithOptions(ctx, query, SearchOptions{
		TopK:      MaxChunksToUse,
		MinScore:  0.3,
		MaxTokens: MaxContextChars,
	})
}

// SearchOptions configures the search behavior
type SearchOptions struct {
	TopK         int                // Maximum chunks to return
	MinScore     float32            // Minimum similarity score (0-1)
	MaxTokens    int                // Maximum context length
	IndustryType model.IndustryType // Optional: filter by industry
	StartDate    string             // Optional: filter by period start
	EndDate      string             // Optional: filter by period end
}

// SearchWithOptions performs search with custom options.
func (r *EnhancedRAGService) SearchWithOptions(
	ctx context.Context,
	query string,
	opts SearchOptions,
) (*EnhancedSearchResult, error) {
	result := &EnhancedSearchResult{
		Sources:      make([]string, 0),
		SourceFiles:  make([]string, 0),
		IndustryHits: make(map[model.IndustryType]int),
	}

	// Try semantic search first if embeddings are enabled
	if r.useEmbeddings && r.vectorStore != nil && r.vectorStore.Count() > 0 {
		semanticResult, err := r.semanticSearch(ctx, query, opts)
		if err != nil {
			log.Printf("[EnhancedRAG] Semantic search failed, falling back to keyword: %v", err)
		} else if semanticResult.ChunksUsed > 0 {
			result = semanticResult
			result.SearchMethod = "semantic"
			return result, nil
		}
	}

	// Fall back to keyword search
	keywordResult := r.keywordSearch(query, opts)
	keywordResult.SearchMethod = "keyword"
	return keywordResult, nil
}

// semanticSearch performs vector similarity search.
//
// IMPORTANT: Filters are applied BEFORE similarity search, not after.
// This fixes an earlier bug where opts.StartDate/EndDate were ignored on
// the semantic path, causing period-scoped questions ("Q1 2024") to leak
// evidence from other periods.
func (r *EnhancedRAGService) semanticSearch(
	ctx context.Context,
	query string,
	opts SearchOptions,
) (*EnhancedSearchResult, error) {
	result := &EnhancedSearchResult{
		Sources:      make([]string, 0),
		SourceFiles:  make([]string, 0),
		IndustryHits: make(map[model.IndustryType]int),
	}

	vf := VectorFilter{
		Industry:    opts.IndustryType,
		PeriodStart: opts.StartDate,
		PeriodEnd:   opts.EndDate,
	}
	filter := func(doc VectorDocument) bool { return matchesFilter(doc, vf) }

	// Perform similarity search
	results, err := r.vectorStore.SimilaritySearchWithFilter(ctx, query, opts.TopK, filter)
	if err != nil {
		return nil, err
	}

	// Build context from results
	var contextBuilder strings.Builder
	seenSources := make(map[string]bool)
	seenFiles := make(map[string]bool)
	currentLen := 0

	for _, sr := range results {
		// Skip low-scoring results
		if sr.Similarity < opts.MinScore {
			continue
		}

		chunkText := sr.Document.Text
		newLen := currentLen + len(chunkText) + 100

		if newLen > opts.MaxTokens {
			remaining := opts.MaxTokens - currentLen - 150
			if remaining > 200 {
				chunkText = truncateText(chunkText, remaining)
			} else {
				result.Truncated = true
				break
			}
		}

		// Add document header
		if !seenFiles[sr.Document.Source] {
			contextBuilder.WriteString(fmt.Sprintf("\n\n📄 [From: %s] (score: %.2f)\n", sr.Document.Source, sr.Similarity))
			seenFiles[sr.Document.Source] = true
		}

		contextBuilder.WriteString(chunkText)
		contextBuilder.WriteString("\n---\n")

		currentLen = contextBuilder.Len()
		result.ChunksUsed++
		result.TopScores = append(result.TopScores, sr.Similarity)

		if !seenSources[sr.Document.DocumentID] {
			result.Sources = append(result.Sources, sr.Document.DocumentID)
			seenSources[sr.Document.DocumentID] = true
		}

		if sr.Document.IndustryType != "" {
			result.IndustryHits[sr.Document.IndustryType]++
		}
	}

	result.Context = contextBuilder.String()
	result.TotalChunks = r.vectorStore.Count()

	return result, nil
}

// keywordSearch falls back to keyword-based search.
func (r *EnhancedRAGService) keywordSearch(query string, opts SearchOptions) *EnhancedSearchResult {
	var basicResult SearchResult

	if opts.StartDate != "" && opts.EndDate != "" {
		basicResult = r.keywordRAG.SearchEnhancedWithPeriod(query, opts.StartDate, opts.EndDate)
	} else {
		basicResult = r.keywordRAG.SearchEnhanced(query)
	}

	return &EnhancedSearchResult{
		Context:     basicResult.Context,
		Sources:     basicResult.Sources,
		SourceFiles: basicResult.SourceFiles,
		ChunksUsed:  basicResult.ChunksUsed,
		TotalChunks: basicResult.TotalChunks,
		Truncated:   basicResult.Truncated,
	}
}

// IndexDocument adds a parsed document to the vector store.
func (r *EnhancedRAGService) IndexDocument(
	ctx context.Context,
	doc *model.ParsedDocument,
	industryType model.IndustryType,
) error {
	if r.vectorStore == nil {
		return fmt.Errorf("vector store not configured")
	}

	if !model.IsValidIndustryType(industryType) {
		industryType = model.IndustryGeneric
	}

	var vectorDocs []VectorDocument

	for i, chunk := range doc.Chunks {
		vectorDoc := VectorDocument{
			ID:           fmt.Sprintf("%s_chunk_%d", doc.DocumentID, i),
			DocumentID:   doc.DocumentID,
			IndustryType: industryType,
			DocType:      doc.DocType,
			PeriodStart:  doc.Period.Start,
			PeriodEnd:    doc.Period.End,
			Source:       doc.Filename,
			Text:         chunk.Text,
			Metadata: map[string]interface{}{
				"page":         chunk.Page,
				"sheet":        chunk.Sheet,
				"chunk_idx":    i,
				"total_chunks": len(doc.Chunks),
			},
		}
		vectorDocs = append(vectorDocs, vectorDoc)
	}

	if len(vectorDocs) == 0 && doc.RawText != "" {
		vectorDocs = append(vectorDocs, VectorDocument{
			ID:           fmt.Sprintf("%s_raw", doc.DocumentID),
			DocumentID:   doc.DocumentID,
			IndustryType: industryType,
			DocType:      doc.DocType,
			PeriodStart:  doc.Period.Start,
			PeriodEnd:    doc.Period.End,
			Source:       doc.Filename,
			Text:         truncateText(doc.RawText, 2000),
			Metadata: map[string]interface{}{
				"raw": true,
			},
		})
	}

	if err := r.vectorStore.AddDocuments(ctx, vectorDocs); err != nil {
		return fmt.Errorf("failed to add documents to vector store: %w", err)
	}

	log.Printf("[EnhancedRAG] Indexed %d chunks from document %s for industry %s",
		len(vectorDocs), doc.DocumentID, industryType)

	return nil
}

// IndexAllDocuments indexes all parsed documents into the vector store.
func (r *EnhancedRAGService) IndexAllDocuments(
	ctx context.Context,
	industryType model.IndustryType,
) error {
	docs, err := r.store.LoadAllParsedDocuments()
	if err != nil {
		return fmt.Errorf("failed to load documents: %w", err)
	}

	for _, doc := range docs {
		if err := r.IndexDocument(ctx, doc, industryType); err != nil {
			log.Printf("[EnhancedRAG] Warning: Failed to index document %s: %v", doc.DocumentID, err)
			// Continue indexing other documents
		}
	}

	log.Printf("[EnhancedRAG] Indexed %d documents", len(docs))
	return nil
}

// RemoveDocument removes a document from the vector store.
func (r *EnhancedRAGService) RemoveDocument(documentID string) {
	if r.vectorStore != nil {
		r.vectorStore.DeleteByDocumentID(documentID)
	}
}

// GetStats returns statistics about the RAG service.
func (r *EnhancedRAGService) GetStats() map[string]interface{} {
	stats := map[string]interface{}{
		"embeddings_enabled": r.useEmbeddings,
	}

	if r.vectorStore != nil {
		stats["total_vectors"] = r.vectorStore.Count()
		stats["generic_vectors"] = r.vectorStore.CountByIndustry(model.IndustryGeneric)
		stats["education_vectors"] = r.vectorStore.CountByIndustry(model.IndustryEducation)
		stats["ecommerce_vectors"] = r.vectorStore.CountByIndustry(model.IndustryEcommerce)
		stats["pharma_vectors"] = r.vectorStore.CountByIndustry(model.IndustryPharma)
	}

	if r.embedService != nil {
		stats["embedding_model"] = r.embedService.GetModelName()
	}

	return stats
}

