// Package service provides embedding capabilities using langchaingo.
//
// This module provides vector embeddings for semantic search in RAG.
// It uses Ollama's embedding models for local, privacy-preserving embeddings.
//
// TODO: Add support for additional embedding providers (OpenAI, Cohere, etc.)
package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/tmc/langchaingo/embeddings"
	"github.com/tmc/langchaingo/llms/ollama"
)

// EmbeddingService provides text embedding capabilities using langchaingo.
// It wraps Ollama's embedding models for semantic search in the RAG system.
type EmbeddingService struct {
	embedder    embeddings.Embedder
	modelName   string
	ollamaHost  string
	initialized bool
	mu          sync.RWMutex
}

// EmbeddingConfig holds configuration for the embedding service
type EmbeddingConfig struct {
	OllamaHost string // e.g., "http://localhost:11434"
	ModelName  string // e.g., "nomic-embed-text" or "all-minilm"
}

// DefaultEmbeddingConfig returns default embedding configuration
func DefaultEmbeddingConfig() EmbeddingConfig {
	return EmbeddingConfig{
		OllamaHost: "http://localhost:11434",
		ModelName:  "nomic-embed-text", // Good balance of quality and speed
	}
}

// NewEmbeddingService creates a new embedding service.
// It lazily initializes the embedder on first use.
func NewEmbeddingService(cfg EmbeddingConfig) *EmbeddingService {
	return &EmbeddingService{
		ollamaHost: cfg.OllamaHost,
		modelName:  cfg.ModelName,
	}
}

// Initialize sets up the embedder. Called lazily on first use.
func (e *EmbeddingService) Initialize(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.initialized {
		return nil
	}

	log.Printf("[Embeddings] Initializing with model: %s at %s", e.modelName, e.ollamaHost)

	// Create Ollama LLM client for embeddings
	llm, err := ollama.New(
		ollama.WithServerURL(e.ollamaHost),
		ollama.WithModel(e.modelName),
	)
	if err != nil {
		return fmt.Errorf("failed to create ollama client: %w", err)
	}

	// Create embedder from LLM
	embedder, err := embeddings.NewEmbedder(llm)
	if err != nil {
		return fmt.Errorf("failed to create embedder: %w", err)
	}

	e.embedder = embedder
	e.initialized = true
	log.Printf("[Embeddings] Successfully initialized")
	return nil
}

// EmbedText generates an embedding vector for a single text string.
func (e *EmbeddingService) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if err := e.Initialize(ctx); err != nil {
		return nil, err
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	embeddings, err := e.embedder.EmbedDocuments(ctx, []string{text})
	if err != nil {
		return nil, fmt.Errorf("failed to embed text: %w", err)
	}

	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return embeddings[0], nil
}

// EmbedTexts generates embedding vectors for multiple texts in batch.
func (e *EmbeddingService) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if err := e.Initialize(ctx); err != nil {
		return nil, err
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	embeddings, err := e.embedder.EmbedDocuments(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("failed to embed texts: %w", err)
	}

	return embeddings, nil
}

// EmbedQuery generates an embedding for a search query.
// This may use a different encoding strategy optimized for queries.
func (e *EmbeddingService) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	if err := e.Initialize(ctx); err != nil {
		return nil, err
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	embedding, err := e.embedder.EmbedQuery(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	return embedding, nil
}

// CosineSimilarity calculates the cosine similarity between two vectors.
// Returns a value between -1 and 1, where 1 means identical direction.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float32
	for i := range a {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt32(normA) * sqrt32(normB))
}

// sqrt32 computes square root for float32
func sqrt32(x float32) float32 {
	// Using Newton's method for float32
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// IsAvailable checks if the embedding service can connect to Ollama.
func (e *EmbeddingService) IsAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := e.Initialize(ctx)
	return err == nil
}

// GetModelName returns the current embedding model name
func (e *EmbeddingService) GetModelName() string {
	return e.modelName
}

