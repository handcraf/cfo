// Package service provides a simple in-memory vector store for RAG.
//
// This module implements a lightweight vector store using langchaingo patterns.
// For production use, consider integrating with dedicated vector databases
// like Chroma, Pinecone, Weaviate, or pgvector.
//
// TODO: Add support for persistent vector stores (Chroma, Pinecone, etc.)
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// VectorDocument represents a document chunk with its embedding.
//
// Typed metadata fields (DocType, PeriodStart, PeriodEnd, Source) are
// promoted to top-level so filters and re-ranking don't have to poke into
// the free-form Metadata map. This also matches what Qdrant wants for its
// native filter index.
//
// Keep Metadata for bag-of-extras (page, sheet, chunk_idx, etc.).
type VectorDocument struct {
	ID           string                 `json:"id"`
	DocumentID   string                 `json:"document_id"`
	IndustryType model.IndustryType     `json:"industry_type"`
	DocType      model.DocType          `json:"doc_type,omitempty"`
	PeriodStart  string                 `json:"period_start,omitempty"` // YYYY-MM-DD
	PeriodEnd    string                 `json:"period_end,omitempty"`   // YYYY-MM-DD
	Source       string                 `json:"source,omitempty"`
	Text         string                 `json:"text"`
	Embedding    []float32              `json:"embedding"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
}

// SimilarityResult represents a search result with similarity score
type SimilarityResult struct {
	Document   VectorDocument
	Similarity float32
}

// VectorStore provides in-memory vector storage and similarity search.
// Uses cosine similarity for finding relevant documents.
type VectorStore struct {
	documents      map[string]VectorDocument
	embedService   *EmbeddingService
	paths          *storage.Paths
	mu             sync.RWMutex
	persistEnabled bool
}

// VectorStoreConfig holds configuration for the vector store
type VectorStoreConfig struct {
	EmbeddingService *EmbeddingService
	Paths            *storage.Paths
	PersistEnabled   bool // Whether to persist vectors to disk
}

// NewVectorStore creates a new in-memory vector store
func NewVectorStore(cfg VectorStoreConfig) *VectorStore {
	return &VectorStore{
		documents:      make(map[string]VectorDocument),
		embedService:   cfg.EmbeddingService,
		paths:          cfg.Paths,
		persistEnabled: cfg.PersistEnabled,
	}
}

// AddDocument adds a document chunk with its text, generating an embedding.
func (vs *VectorStore) AddDocument(ctx context.Context, doc VectorDocument) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Generate embedding if not provided
	if len(doc.Embedding) == 0 && vs.embedService != nil {
		embedding, err := vs.embedService.EmbedText(ctx, doc.Text)
		if err != nil {
			log.Printf("[VectorStore] Warning: Failed to generate embedding: %v", err)
			// Continue without embedding - will fall back to keyword search
		} else {
			doc.Embedding = embedding
		}
	}

	doc.CreatedAt = time.Now()
	vs.documents[doc.ID] = doc

	log.Printf("[VectorStore] Added document: %s (embedding dim: %d)", doc.ID, len(doc.Embedding))
	return nil
}

// AddDocuments adds multiple document chunks in batch.
func (vs *VectorStore) AddDocuments(ctx context.Context, docs []VectorDocument) error {
	// Collect texts that need embeddings
	var textsToEmbed []string
	var docsNeedingEmbedding []int

	for i, doc := range docs {
		if len(doc.Embedding) == 0 {
			textsToEmbed = append(textsToEmbed, doc.Text)
			docsNeedingEmbedding = append(docsNeedingEmbedding, i)
		}
	}

	// Batch embed if needed
	if len(textsToEmbed) > 0 && vs.embedService != nil {
		embeddings, err := vs.embedService.EmbedTexts(ctx, textsToEmbed)
		if err != nil {
			log.Printf("[VectorStore] Warning: Failed to batch embed: %v", err)
		} else {
			for i, idx := range docsNeedingEmbedding {
				if i < len(embeddings) {
					docs[idx].Embedding = embeddings[i]
				}
			}
		}
	}

	// Add all documents
	vs.mu.Lock()
	defer vs.mu.Unlock()

	for _, doc := range docs {
		doc.CreatedAt = time.Now()
		vs.documents[doc.ID] = doc
	}

	log.Printf("[VectorStore] Added %d documents in batch", len(docs))
	return nil
}

// SimilaritySearch finds the most similar documents to a query.
// Returns up to topK results sorted by similarity (highest first).
func (vs *VectorStore) SimilaritySearch(ctx context.Context, query string, topK int) ([]SimilarityResult, error) {
	return vs.SimilaritySearchWithFilter(ctx, query, topK, nil)
}

// SimilaritySearchWithFilter finds similar documents with optional filtering.
func (vs *VectorStore) SimilaritySearchWithFilter(
	ctx context.Context,
	query string,
	topK int,
	filter func(VectorDocument) bool,
) ([]SimilarityResult, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	if len(vs.documents) == 0 {
		return nil, nil
	}

	// Get query embedding
	var queryEmbedding []float32
	if vs.embedService != nil {
		var err error
		queryEmbedding, err = vs.embedService.EmbedQuery(ctx, query)
		if err != nil {
			log.Printf("[VectorStore] Warning: Failed to embed query, falling back to keyword: %v", err)
			return vs.keywordSearchLocked(query, topK, filter), nil
		}
	} else {
		// Fall back to keyword search if no embedding service
		return vs.keywordSearchLocked(query, topK, filter), nil
	}

	// Calculate similarities
	var results []SimilarityResult
	for _, doc := range vs.documents {
		// Apply filter if provided
		if filter != nil && !filter(doc) {
			continue
		}

		if len(doc.Embedding) == 0 {
			// No embedding, use keyword matching as fallback
			continue
		}

		similarity := CosineSimilarity(queryEmbedding, doc.Embedding)
		results = append(results, SimilarityResult{
			Document:   doc,
			Similarity: similarity,
		})
	}

	// Sort by similarity (highest first)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	// Return top K
	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// keywordSearchLocked performs keyword-based search as a fallback.
// Must be called with read lock held.
func (vs *VectorStore) keywordSearchLocked(query string, topK int, filter func(VectorDocument) bool) []SimilarityResult {
	// Simple keyword matching for fallback
	var results []SimilarityResult

	keywords := extractSearchKeywords(query)

	for _, doc := range vs.documents {
		if filter != nil && !filter(doc) {
			continue
		}

		score := scoreTextWithKeywords(doc.Text, keywords)
		if score > 0 {
			results = append(results, SimilarityResult{
				Document:   doc,
				Similarity: float32(score) / float32(len(keywords)+1), // Normalize to 0-1
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// SearchByIndustry searches documents filtered by industry type.
func (vs *VectorStore) SearchByIndustry(
	ctx context.Context,
	query string,
	industryType model.IndustryType,
	topK int,
) ([]SimilarityResult, error) {
	filter := func(doc VectorDocument) bool {
		return doc.IndustryType == industryType
	}
	return vs.SimilaritySearchWithFilter(ctx, query, topK, filter)
}

// DeleteDocument removes a document by ID.
func (vs *VectorStore) DeleteDocument(id string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	delete(vs.documents, id)
	log.Printf("[VectorStore] Deleted document: %s", id)
}

// DeleteByDocumentID removes all chunks for a source document.
func (vs *VectorStore) DeleteByDocumentID(documentID string) int {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	count := 0
	for id, doc := range vs.documents {
		if doc.DocumentID == documentID {
			delete(vs.documents, id)
			count++
		}
	}

	log.Printf("[VectorStore] Deleted %d chunks for document: %s", count, documentID)
	return count
}

// ClearByIndustry removes all documents for an industry type.
func (vs *VectorStore) ClearByIndustry(industryType model.IndustryType) int {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	count := 0
	for id, doc := range vs.documents {
		if doc.IndustryType == industryType {
			delete(vs.documents, id)
			count++
		}
	}

	log.Printf("[VectorStore] Cleared %d documents for industry: %s", count, industryType)
	return count
}

// Clear removes all documents from the store.
func (vs *VectorStore) Clear() {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.documents = make(map[string]VectorDocument)
	log.Printf("[VectorStore] Cleared all documents")
}

// Count returns the total number of documents in the store.
func (vs *VectorStore) Count() int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return len(vs.documents)
}

// CountByIndustry returns document count for an industry type.
func (vs *VectorStore) CountByIndustry(industryType model.IndustryType) int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	count := 0
	for _, doc := range vs.documents {
		if doc.IndustryType == industryType {
			count++
		}
	}
	return count
}

// Save persists the vector store to disk.
func (vs *VectorStore) Save(filename string) error {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	docs := make([]VectorDocument, 0, len(vs.documents))
	for _, doc := range vs.documents {
		docs = append(docs, doc)
	}

	data, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal vector store: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write vector store: %w", err)
	}

	log.Printf("[VectorStore] Saved %d documents to %s", len(docs), filename)
	return nil
}

// Load restores the vector store from disk.
func (vs *VectorStore) Load(filename string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, start fresh
		}
		return fmt.Errorf("failed to read vector store: %w", err)
	}

	var docs []VectorDocument
	if err := json.Unmarshal(data, &docs); err != nil {
		return fmt.Errorf("failed to unmarshal vector store: %w", err)
	}

	vs.documents = make(map[string]VectorDocument, len(docs))
	for _, doc := range docs {
		vs.documents[doc.ID] = doc
	}

	log.Printf("[VectorStore] Loaded %d documents from %s", len(docs), filename)
	return nil
}

// SaveToIndustryPath saves vectors to the appropriate industry RAG directory.
func (vs *VectorStore) SaveToIndustryPath(industryType model.IndustryType) error {
	if vs.paths == nil {
		return fmt.Errorf("paths not configured")
	}

	ragPath := vs.paths.GetRAGPath(industryType)
	filename := filepath.Join(ragPath, "vectors.json")

	// Filter documents for this industry
	vs.mu.RLock()
	var docs []VectorDocument
	for _, doc := range vs.documents {
		if doc.IndustryType == industryType {
			docs = append(docs, doc)
		}
	}
	vs.mu.RUnlock()

	data, err := json.MarshalIndent(docs, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write: %w", err)
	}

	log.Printf("[VectorStore] Saved %d vectors to %s", len(docs), filename)
	return nil
}

// LoadFromIndustryPath loads vectors from an industry RAG directory.
func (vs *VectorStore) LoadFromIndustryPath(industryType model.IndustryType) error {
	if vs.paths == nil {
		return fmt.Errorf("paths not configured")
	}

	ragPath := vs.paths.GetRAGPath(industryType)
	filename := filepath.Join(ragPath, "vectors.json")

	return vs.Load(filename)
}

// ============================================================================
// Store interface implementation (see vectorstore_interface.go)
// ============================================================================

// Search implements the Store interface. It applies VectorFilter BEFORE
// similarity search (cheap for in-memory; correctness-critical for Qdrant).
// It also applies MinScore and TopK.
func (vs *VectorStore) Search(ctx context.Context, q SearchQuery) ([]SimilarityResult, error) {
	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}

	filterFn := func(doc VectorDocument) bool {
		return matchesFilter(doc, q.Filter)
	}

	raw, err := vs.SimilaritySearchWithFilter(ctx, q.Text, topK, filterFn)
	if err != nil {
		return nil, err
	}

	if q.MinScore > 0 {
		filtered := raw[:0]
		for _, r := range raw {
			if r.Similarity >= q.MinScore {
				filtered = append(filtered, r)
			}
		}
		return filtered, nil
	}
	return raw, nil
}

// DeleteByDocumentIDCtx is the context-aware version used via the Store
// interface. It wraps the legacy DeleteByDocumentID.
func (vs *VectorStore) DeleteByDocumentIDCtx(_ context.Context, documentID string) (int, error) {
	return vs.DeleteByDocumentID(documentID), nil
}

// CountFiltered returns the number of documents matching the filter.
// Zero filter = total count (same as Count()).
func (vs *VectorStore) CountFiltered(_ context.Context, filter VectorFilter) (int, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	if filter.IsZero() {
		return len(vs.documents), nil
	}

	count := 0
	for _, doc := range vs.documents {
		if matchesFilter(doc, filter) {
			count++
		}
	}
	return count, nil
}

// ClearFiltered removes all documents matching the filter. Zero filter = wipe all.
func (vs *VectorStore) ClearFiltered(_ context.Context, filter VectorFilter) (int, error) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	if filter.IsZero() {
		n := len(vs.documents)
		vs.documents = make(map[string]VectorDocument)
		log.Printf("[VectorStore] Cleared all %d documents (filter=zero)", n)
		return n, nil
	}

	count := 0
	for id, doc := range vs.documents {
		if matchesFilter(doc, filter) {
			delete(vs.documents, id)
			count++
		}
	}
	log.Printf("[VectorStore] Cleared %d documents (filter=%+v)", count, filter)
	return count, nil
}

// Close is a no-op for the in-memory store. Included to satisfy the Store
// interface; remote backends (Qdrant) use this to close connections.
func (vs *VectorStore) Close() error { return nil }

// storeAdapter wraps *VectorStore to satisfy the Store interface with the
// exact method signatures. We use an adapter instead of renaming the
// existing methods to avoid churning every test and caller.
type inMemoryStoreAdapter struct{ *VectorStore }

// NewInMemoryStore wraps a *VectorStore so it can be used as a Store. This
// is what callers should use going forward. Legacy *VectorStore methods
// remain for backward compatibility and persistence.
func NewInMemoryStore(vs *VectorStore) Store { return inMemoryStoreAdapter{vs} }

func (a inMemoryStoreAdapter) DeleteByDocumentID(ctx context.Context, id string) (int, error) {
	return a.VectorStore.DeleteByDocumentIDCtx(ctx, id)
}

func (a inMemoryStoreAdapter) Count(ctx context.Context, f VectorFilter) (int, error) {
	return a.VectorStore.CountFiltered(ctx, f)
}

func (a inMemoryStoreAdapter) Clear(ctx context.Context, f VectorFilter) (int, error) {
	return a.VectorStore.ClearFiltered(ctx, f)
}

// Helper function to extract keywords from query
func extractSearchKeywords(query string) []string {
	// Reuse the RAG service's keyword extraction logic
	// This is a simplified version
	words := make([]string, 0)
	for _, word := range splitWords(query) {
		if len(word) > 2 && !isStopWord(word) {
			words = append(words, word)
		}
	}
	return words
}

// Helper function to score text against keywords
func scoreTextWithKeywords(text string, keywords []string) int {
	score := 0
	lowerText := toLower(text)
	for _, kw := range keywords {
		if containsWord(lowerText, toLower(kw)) {
			score++
		}
	}
	return score
}

// Simple string helpers to avoid importing strings in many places
func splitWords(s string) []string {
	var words []string
	word := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			word += string(r)
		} else if word != "" {
			words = append(words, word)
			word = ""
		}
	}
	if word != "" {
		words = append(words, word)
	}
	return words
}

func toLower(s string) string {
	result := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		result[i] = c
	}
	return string(result)
}

func containsWord(text, word string) bool {
	for i := 0; i <= len(text)-len(word); i++ {
		if text[i:i+len(word)] == word {
			return true
		}
	}
	return false
}

func isStopWord(word string) bool {
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"what": true, "how": true, "why": true, "when": true, "where": true,
		"who": true, "which": true, "that": true, "this": true,
		"and": true, "or": true, "but": true, "if": true,
		"of": true, "to": true, "in": true, "on": true, "at": true,
		"by": true, "for": true, "with": true, "about": true, "from": true,
	}
	return stopWords[toLower(word)]
}

