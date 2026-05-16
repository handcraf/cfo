// Package service — Qdrant adapter for the Store interface.
//
// Why HTTP, not gRPC?
//   - Zero new protobuf/grpc dependencies.
//   - Qdrant's REST API exposes the full filter DSL we need.
//   - Latency overhead is negligible at our QPS (single-digit per second).
//
// This adapter is intentionally conservative: it does the minimum required
// to satisfy the Store interface. Production-grade Qdrant usage (named
// vectors, sparse vectors, server-side hybrid) can be layered on later
// without touching callers — that's the whole point of the interface.
//
// TODO: Add retry/backoff for transient 5xx / connection errors.
// TODO: Batch embedding calls when AddDocuments gets a large slice; today
//       we rely on the embedder's own batch logic upstream.
// TODO: Ingest-side BM25 / sparse vectors for server-side hybrid. Today we
//       do client-side hybrid (see hybrid_retrieval.go).
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cfo/backend/internal/model"
)

// QdrantStore implements Store against a Qdrant server over REST.
type QdrantStore struct {
	baseURL    string
	collection string
	apiKey     string // optional; empty = no auth header
	client     *http.Client
	embedder   *EmbeddingService
	vectorDim  int
}

// QdrantConfig configures the adapter.
type QdrantConfig struct {
	BaseURL    string // e.g. http://qdrant:6333
	Collection string // e.g. cfo_chunks
	APIKey     string // optional
	Embedder   *EmbeddingService
	VectorDim  int    // embedding dimension; required for collection create
	HTTPTimeout time.Duration
}

// NewQdrantStore builds a Qdrant-backed Store. It does NOT create the
// collection — call EnsureCollection explicitly so the caller decides when
// schema changes are safe.
func NewQdrantStore(cfg QdrantConfig) *QdrantStore {
	timeout := cfg.HTTPTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &QdrantStore{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		collection: cfg.Collection,
		apiKey:     cfg.APIKey,
		client:     &http.Client{Timeout: timeout},
		embedder:   cfg.Embedder,
		vectorDim:  cfg.VectorDim,
	}
}

// EnsureCollection creates the collection if it doesn't exist, with cosine
// distance and the configured vector dimension. Safe to call repeatedly.
//
// This also creates payload indexes on the fields the filter uses, which
// is what lets Qdrant push filters down to the vector index rather than
// doing post-hoc filtering. Without these indexes, filtered search is
// slow and (worse) imprecise at scale.
func (q *QdrantStore) EnsureCollection(ctx context.Context) error {
	exists, err := q.collectionExists(ctx)
	if err != nil {
		return err
	}
	if !exists {
		if q.vectorDim <= 0 {
			return fmt.Errorf("qdrant: VectorDim must be set to create collection")
		}
		body := map[string]any{
			"vectors": map[string]any{
				"size":     q.vectorDim,
				"distance": "Cosine",
			},
		}
		if err := q.do(ctx, http.MethodPut, "/collections/"+q.collection, body, nil); err != nil {
			return fmt.Errorf("qdrant: create collection: %w", err)
		}
		log.Printf("[Qdrant] created collection %s (dim=%d)", q.collection, q.vectorDim)
	}
	// Indexes are cheap and idempotent via Qdrant's upsert semantics.
	for _, field := range []struct {
		name string
		typ  string
	}{
		{"industry_type", "keyword"},
		{"doc_type", "keyword"},
		{"source", "keyword"},
		{"document_id", "keyword"},
		{"period_start", "keyword"},
		{"period_end", "keyword"},
	} {
		body := map[string]any{"field_name": field.name, "field_schema": field.typ}
		if err := q.do(ctx, http.MethodPut,
			"/collections/"+q.collection+"/index", body, nil); err != nil {
			// Non-fatal: index creation on an existing index is a 4xx on some
			// Qdrant versions. Log and continue.
			log.Printf("[Qdrant] index %s: %v (may already exist)", field.name, err)
		}
	}
	return nil
}

func (q *QdrantStore) collectionExists(ctx context.Context) (bool, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		q.baseURL+"/collections/"+q.collection, nil)
	q.addAuth(req)
	resp, err := q.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("qdrant: ping collection: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		body, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("qdrant: unexpected %d: %s", resp.StatusCode, body)
	}
}

// ============================================================================
// Store interface implementation
// ============================================================================

// AddDocuments embeds (if needed) then upserts points into Qdrant.
// Payload fields are the typed VectorDocument fields + the Metadata map
// spread under a "metadata" key.
func (q *QdrantStore) AddDocuments(ctx context.Context, docs []VectorDocument) error {
	if len(docs) == 0 {
		return nil
	}

	// Embed any docs missing an embedding.
	var pending []int
	var pendingTexts []string
	for i, d := range docs {
		if len(d.Embedding) == 0 {
			pending = append(pending, i)
			pendingTexts = append(pendingTexts, d.Text)
		}
	}
	if len(pending) > 0 {
		if q.embedder == nil {
			return fmt.Errorf("qdrant: %d docs missing embeddings and no embedder configured", len(pending))
		}
		embs, err := q.embedder.EmbedTexts(ctx, pendingTexts)
		if err != nil {
			return fmt.Errorf("qdrant: embed: %w", err)
		}
		for j, idx := range pending {
			if j < len(embs) {
				docs[idx].Embedding = embs[j]
			}
		}
	}

	points := make([]map[string]any, 0, len(docs))
	for _, d := range docs {
		if d.CreatedAt.IsZero() {
			d.CreatedAt = time.Now().UTC()
		}
		points = append(points, map[string]any{
			"id":     d.ID,
			"vector": d.Embedding,
			"payload": map[string]any{
				"document_id":   d.DocumentID,
				"industry_type": string(d.IndustryType),
				"doc_type":      string(d.DocType),
				"period_start":  d.PeriodStart,
				"period_end":    d.PeriodEnd,
				"source":        d.Source,
				"text":          d.Text,
				"metadata":      d.Metadata,
				"created_at":    d.CreatedAt.Format(time.RFC3339),
			},
		})
	}

	body := map[string]any{"points": points}
	return q.do(ctx, http.MethodPut,
		"/collections/"+q.collection+"/points?wait=true", body, nil)
}

// Search translates our SearchQuery into Qdrant's /points/search body,
// with the filter pushed down so Qdrant does pre-filtering natively.
func (q *QdrantStore) Search(ctx context.Context, sq SearchQuery) ([]SimilarityResult, error) {
	if q.embedder == nil {
		return nil, fmt.Errorf("qdrant: embedder required for Search")
	}
	qvec, err := q.embedder.EmbedQuery(ctx, sq.Text)
	if err != nil {
		return nil, fmt.Errorf("qdrant: embed query: %w", err)
	}

	topK := sq.TopK
	if topK <= 0 {
		topK = 10
	}

	body := map[string]any{
		"vector":       qvec,
		"limit":        topK,
		"with_payload": true,
	}
	if f := qdrantFilterFromVectorFilter(sq.Filter); f != nil {
		body["filter"] = f
	}
	if sq.MinScore > 0 {
		body["score_threshold"] = sq.MinScore
	}

	var resp struct {
		Result []struct {
			ID      any            `json:"id"`
			Score   float32        `json:"score"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := q.do(ctx, http.MethodPost,
		"/collections/"+q.collection+"/points/search", body, &resp); err != nil {
		return nil, err
	}

	out := make([]SimilarityResult, 0, len(resp.Result))
	for _, r := range resp.Result {
		out = append(out, SimilarityResult{
			Similarity: r.Score,
			Document:   vectorDocFromQdrantPayload(r.ID, r.Payload),
		})
	}
	return out, nil
}

// DeleteByDocumentID removes every point whose payload.document_id matches.
func (q *QdrantStore) DeleteByDocumentID(ctx context.Context, documentID string) (int, error) {
	body := map[string]any{
		"filter": map[string]any{
			"must": []map[string]any{{
				"key":   "document_id",
				"match": map[string]any{"value": documentID},
			}},
		},
	}
	var resp struct {
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	if err := q.do(ctx, http.MethodPost,
		"/collections/"+q.collection+"/points/delete?wait=true", body, &resp); err != nil {
		return 0, err
	}
	// Qdrant doesn't return an exact count from delete-by-filter. The caller
	// uses this for housekeeping, not for precise accounting.
	// TODO: follow-up count via Count(ctx, VectorFilter{DocumentIDs:[id]}) if needed.
	return -1, nil
}

// Count returns the count of points matching filter.
func (q *QdrantStore) Count(ctx context.Context, filter VectorFilter) (int, error) {
	body := map[string]any{"exact": true}
	if f := qdrantFilterFromVectorFilter(filter); f != nil {
		body["filter"] = f
	}
	var resp struct {
		Result struct {
			Count int `json:"count"`
		} `json:"result"`
	}
	if err := q.do(ctx, http.MethodPost,
		"/collections/"+q.collection+"/points/count", body, &resp); err != nil {
		return 0, err
	}
	return resp.Result.Count, nil
}

// Clear removes points matching filter. Zero filter = wipe collection.
func (q *QdrantStore) Clear(ctx context.Context, filter VectorFilter) (int, error) {
	before, err := q.Count(ctx, filter)
	if err != nil {
		return 0, err
	}
	if filter.IsZero() {
		// Simpler and cheaper: drop & recreate the collection.
		if err := q.do(ctx, http.MethodDelete,
			"/collections/"+q.collection, nil, nil); err != nil {
			return 0, err
		}
		if err := q.EnsureCollection(ctx); err != nil {
			return 0, err
		}
		return before, nil
	}
	body := map[string]any{"filter": qdrantFilterFromVectorFilter(filter)}
	if err := q.do(ctx, http.MethodPost,
		"/collections/"+q.collection+"/points/delete?wait=true", body, nil); err != nil {
		return 0, err
	}
	return before, nil
}

// Close is a no-op; the HTTP client has no persistent resources to release.
func (q *QdrantStore) Close() error { return nil }

// ============================================================================
// helpers
// ============================================================================

// qdrantFilterFromVectorFilter translates our typed filter into Qdrant's
// filter DSL. Returns nil when the filter is empty (no server-side filter
// applied).
//
// Semantics:
//   - Industry / DocType / Source / DocumentIDs → exact keyword matches.
//   - DocumentIDs with multiple values → "should" (OR) inside a "must" wrapper.
//   - PeriodStart/PeriodEnd → overlap expressed as two inequalities. Qdrant's
//     native range is "gte/lte"; we reason on YYYY-MM-DD lex sort, same as
//     everywhere else in the codebase.
func qdrantFilterFromVectorFilter(f VectorFilter) map[string]any {
	if f.IsZero() {
		return nil
	}
	var must []map[string]any

	if f.Industry != "" {
		must = append(must, map[string]any{
			"key":   "industry_type",
			"match": map[string]any{"value": string(f.Industry)},
		})
	}
	if f.DocType != "" {
		must = append(must, map[string]any{
			"key":   "doc_type",
			"match": map[string]any{"value": string(f.DocType)},
		})
	}
	if f.Source != "" {
		must = append(must, map[string]any{
			"key":   "source",
			"match": map[string]any{"value": f.Source},
		})
	}
	if len(f.DocumentIDs) > 0 {
		shouldList := make([]map[string]any, 0, len(f.DocumentIDs))
		for _, id := range f.DocumentIDs {
			shouldList = append(shouldList, map[string]any{
				"key":   "document_id",
				"match": map[string]any{"value": id},
			})
		}
		must = append(must, map[string]any{"should": shouldList})
	}
	if f.PeriodStart != "" && f.PeriodEnd != "" {
		// Overlap condition: NOT (docEnd < queryStart OR queryEnd < docStart)
		// Equivalently: docEnd >= queryStart AND docStart <= queryEnd.
		must = append(must, map[string]any{
			"key":   "period_end",
			"range": map[string]any{"gte": f.PeriodStart},
		})
		must = append(must, map[string]any{
			"key":   "period_start",
			"range": map[string]any{"lte": f.PeriodEnd},
		})
	}

	return map[string]any{"must": must}
}

// vectorDocFromQdrantPayload reconstructs a VectorDocument from Qdrant's
// payload map. Any field the payload didn't contain is left zero-valued.
func vectorDocFromQdrantPayload(id any, p map[string]any) VectorDocument {
	get := func(key string) string {
		if v, ok := p[key].(string); ok {
			return v
		}
		return ""
	}
	doc := VectorDocument{
		ID:           fmt.Sprintf("%v", id),
		DocumentID:   get("document_id"),
		IndustryType: model.IndustryType(get("industry_type")),
		DocType:      model.DocType(get("doc_type")),
		PeriodStart:  get("period_start"),
		PeriodEnd:    get("period_end"),
		Source:       get("source"),
		Text:         get("text"),
	}
	if raw, ok := p["metadata"].(map[string]any); ok {
		doc.Metadata = raw
	}
	if s, ok := p["created_at"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			doc.CreatedAt = t
		}
	}
	return doc
}

// do is the tiny HTTP helper. Marshals body, parses response into out (if
// non-nil). Treats anything outside 2xx as an error with the body attached.
func (q *QdrantStore) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("qdrant: marshal body: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, q.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	q.addAuth(req)

	resp, err := q.client.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant: %s %s: %d: %s", method, path, resp.StatusCode, respBody)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (q *QdrantStore) addAuth(req *http.Request) {
	if q.apiKey != "" {
		req.Header.Set("api-key", q.apiKey)
	}
}
