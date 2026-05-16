package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/cfo/backend/internal/model"
)

func TestVectorStore_AddAndSearch(t *testing.T) {
	vs := NewVectorStore(VectorStoreConfig{
		// No embedding service = keyword fallback
	})

	ctx := context.Background()

	// Add test documents
	docs := []VectorDocument{
		{
			ID:           "doc1_chunk1",
			DocumentID:   "doc1",
			IndustryType: model.IndustryGeneric,
			Text:         "Total revenue for Q4 2024 was $25 million, a 15% increase year over year.",
			Source:       "q4_2024_report.csv",
		},
		{
			ID:           "doc1_chunk2",
			DocumentID:   "doc1",
			IndustryType: model.IndustryGeneric,
			Text:         "Operating expenses totaled $18 million, primarily driven by R&D investments.",
			Source:       "q4_2024_report.csv",
		},
		{
			ID:           "doc2_chunk1",
			DocumentID:   "doc2",
			IndustryType: model.IndustryEducation,
			Text:         "Student enrollment increased to 15,000 for the fall semester, up 5% from last year.",
			Source:       "enrollment_report.csv",
		},
	}

	for _, doc := range docs {
		err := vs.AddDocument(ctx, doc)
		if err != nil {
			t.Fatalf("AddDocument failed: %v", err)
		}
	}

	if vs.Count() != 3 {
		t.Errorf("Count = %d, want 3", vs.Count())
	}

	// Search for revenue
	results, err := vs.SimilaritySearch(ctx, "What was the revenue?", 5)
	if err != nil {
		t.Fatalf("SimilaritySearch failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected at least one result")
	}

	// First result should be about revenue
	found := false
	for _, r := range results {
		if r.Document.ID == "doc1_chunk1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected to find revenue document in results")
	}
}

func TestVectorStore_SearchByIndustry(t *testing.T) {
	vs := NewVectorStore(VectorStoreConfig{})
	ctx := context.Background()

	// Add documents from different industries
	vs.AddDocument(ctx, VectorDocument{
		ID:           "generic_1",
		IndustryType: model.IndustryGeneric,
		Text:         "Cash flow analysis shows positive trends",
	})
	vs.AddDocument(ctx, VectorDocument{
		ID:           "edu_1",
		IndustryType: model.IndustryEducation,
		Text:         "Student retention rate improved to 87%",
	})
	vs.AddDocument(ctx, VectorDocument{
		ID:           "ecom_1",
		IndustryType: model.IndustryEcommerce,
		Text:         "GMV increased by 25% this quarter",
	})

	// Search only in education
	results, err := vs.SearchByIndustry(ctx, "retention", model.IndustryEducation, 10)
	if err != nil {
		t.Fatalf("SearchByIndustry failed: %v", err)
	}

	// Should only get education results
	for _, r := range results {
		if r.Document.IndustryType != model.IndustryEducation {
			t.Errorf("Got result from wrong industry: %s", r.Document.IndustryType)
		}
	}
}

func TestVectorStore_DeleteDocument(t *testing.T) {
	vs := NewVectorStore(VectorStoreConfig{})
	ctx := context.Background()

	// Add documents
	vs.AddDocument(ctx, VectorDocument{ID: "doc1_chunk1", DocumentID: "doc1", Text: "Test 1"})
	vs.AddDocument(ctx, VectorDocument{ID: "doc1_chunk2", DocumentID: "doc1", Text: "Test 2"})
	vs.AddDocument(ctx, VectorDocument{ID: "doc2_chunk1", DocumentID: "doc2", Text: "Test 3"})

	if vs.Count() != 3 {
		t.Errorf("Initial count = %d, want 3", vs.Count())
	}

	// Delete all chunks from doc1
	deleted := vs.DeleteByDocumentID("doc1")
	if deleted != 2 {
		t.Errorf("Deleted = %d, want 2", deleted)
	}

	if vs.Count() != 1 {
		t.Errorf("Count after delete = %d, want 1", vs.Count())
	}
}

func TestVectorStore_ClearByIndustry(t *testing.T) {
	vs := NewVectorStore(VectorStoreConfig{})
	ctx := context.Background()

	// Add documents from different industries
	vs.AddDocument(ctx, VectorDocument{ID: "gen_1", IndustryType: model.IndustryGeneric, Text: "Generic"})
	vs.AddDocument(ctx, VectorDocument{ID: "edu_1", IndustryType: model.IndustryEducation, Text: "Education"})
	vs.AddDocument(ctx, VectorDocument{ID: "edu_2", IndustryType: model.IndustryEducation, Text: "Education 2"})

	if vs.CountByIndustry(model.IndustryEducation) != 2 {
		t.Errorf("Education count = %d, want 2", vs.CountByIndustry(model.IndustryEducation))
	}

	// Clear education
	cleared := vs.ClearByIndustry(model.IndustryEducation)
	if cleared != 2 {
		t.Errorf("Cleared = %d, want 2", cleared)
	}

	if vs.CountByIndustry(model.IndustryEducation) != 0 {
		t.Error("Education should be empty after clear")
	}

	if vs.CountByIndustry(model.IndustryGeneric) != 1 {
		t.Error("Generic should still have 1 document")
	}
}

func TestVectorStore_SaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vectorstore_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filename := filepath.Join(tmpDir, "vectors.json")

	// Create and populate store
	vs1 := NewVectorStore(VectorStoreConfig{})
	ctx := context.Background()

	vs1.AddDocument(ctx, VectorDocument{
		ID:           "test_1",
		DocumentID:   "doc_1",
		IndustryType: model.IndustryEducation,
		Text:         "Test document for persistence",
		Source:       "test.csv",
	})

	// Save
	err = vs1.Save(filename)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load into new store
	vs2 := NewVectorStore(VectorStoreConfig{})
	err = vs2.Load(filename)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if vs2.Count() != 1 {
		t.Errorf("Loaded count = %d, want 1", vs2.Count())
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
		epsilon  float32
	}{
		{
			name:     "Identical vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{1, 0, 0},
			expected: 1.0,
			epsilon:  0.001,
		},
		{
			name:     "Orthogonal vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{0, 1, 0},
			expected: 0.0,
			epsilon:  0.001,
		},
		{
			name:     "Opposite vectors",
			a:        []float32{1, 0, 0},
			b:        []float32{-1, 0, 0},
			expected: -1.0,
			epsilon:  0.001,
		},
		{
			name:     "Similar vectors",
			a:        []float32{1, 1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.707, // 1/sqrt(2)
			epsilon:  0.01,
		},
		{
			name:     "Empty vectors",
			a:        []float32{},
			b:        []float32{},
			expected: 0.0,
			epsilon:  0.001,
		},
		{
			name:     "Different lengths",
			a:        []float32{1, 0},
			b:        []float32{1, 0, 0},
			expected: 0.0,
			epsilon:  0.001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > tt.epsilon {
				t.Errorf("CosineSimilarity(%v, %v) = %f, want %f", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestExtractSearchKeywords(t *testing.T) {
	tests := []struct {
		query    string
		expected int // minimum expected keywords
	}{
		{"What is the revenue?", 1},                 // "revenue"
		{"Show me the cash flow analysis", 2},       // "cash", "flow", "analysis"
		{"the a an is are", 0},                      // all stop words
		{"student enrollment retention", 3},         // all should be keywords
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			keywords := extractSearchKeywords(tt.query)
			if len(keywords) < tt.expected {
				t.Errorf("extractSearchKeywords(%q) = %d keywords, want at least %d", tt.query, len(keywords), tt.expected)
			}
		})
	}
}

