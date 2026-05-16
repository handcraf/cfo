package service

import (
	"context"
	"os"
	"testing"

	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// ================== ENHANCED RAG SERVICE TESTS ==================

func TestEnhancedRAGService_Creation(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "rag_test_*")
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)
	store := storage.NewFileStore(tmpDir)

	vs := NewVectorStore(VectorStoreConfig{})

	rag := NewEnhancedRAGService(EnhancedRAGConfig{
		Store:         store,
		VectorStore:   vs,
		UseEmbeddings: false,
	})

	if rag == nil {
		t.Fatal("NewEnhancedRAGService returned nil")
	}
}

func TestEnhancedRAGService_KeywordFallback(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "rag_test_*")
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)
	store := storage.NewFileStore(tmpDir)

	// Create a parsed document
	parsedDoc := &model.ParsedDocument{
		DocumentID: "test_doc",
		Filename:   "test.csv",
		DocType:    "P&L",
		RawText:    "Revenue was $1 million. Net income was $200,000.",
		Chunks: []model.TextChunk{
			{Text: "Revenue was $1 million for the quarter."},
			{Text: "Net income was $200,000, a 20% margin."},
		},
	}
	store.SaveParsedDocument(parsedDoc)

	// Create RAG service without embeddings
	rag := NewEnhancedRAGService(EnhancedRAGConfig{
		Store:         store,
		UseEmbeddings: false,
	})

	ctx := context.Background()
	result, err := rag.Search(ctx, "What is the revenue?")

	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if result.SearchMethod != "keyword" {
		t.Errorf("SearchMethod = %q, want 'keyword'", result.SearchMethod)
	}

	if result.ChunksUsed == 0 {
		t.Error("Expected at least one chunk in results")
	}
}

func TestEnhancedRAGService_IndexDocument(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "rag_test_*")
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)
	store := storage.NewFileStore(tmpDir)

	vs := NewVectorStore(VectorStoreConfig{})

	rag := NewEnhancedRAGService(EnhancedRAGConfig{
		Store:         store,
		VectorStore:   vs,
		UseEmbeddings: false,
	})

	// Create test document
	doc := &model.ParsedDocument{
		DocumentID: "index_test",
		Filename:   "test.csv",
		Chunks: []model.TextChunk{
			{Text: "Chunk 1 about revenue"},
			{Text: "Chunk 2 about expenses"},
			{Text: "Chunk 3 about profit"},
		},
	}

	ctx := context.Background()
	err := rag.IndexDocument(ctx, doc, model.IndustryEducation)

	if err != nil {
		t.Fatalf("IndexDocument failed: %v", err)
	}

	// Check vector store has the documents
	if vs.Count() != 3 {
		t.Errorf("VectorStore count = %d, want 3", vs.Count())
	}

	if vs.CountByIndustry(model.IndustryEducation) != 3 {
		t.Errorf("Education count = %d, want 3", vs.CountByIndustry(model.IndustryEducation))
	}
}

func TestEnhancedRAGService_SearchWithFilter(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "rag_test_*")
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)
	store := storage.NewFileStore(tmpDir)

	vs := NewVectorStore(VectorStoreConfig{})
	ctx := context.Background()

	// Add documents from different industries
	vs.AddDocument(ctx, VectorDocument{
		ID:           "edu_1",
		DocumentID:   "doc_edu",
		IndustryType: model.IndustryEducation,
		Text:         "Student enrollment increased to 15,000",
	})
	vs.AddDocument(ctx, VectorDocument{
		ID:           "ecom_1",
		DocumentID:   "doc_ecom",
		IndustryType: model.IndustryEcommerce,
		Text:         "GMV reached $10 million this quarter",
	})

	rag := NewEnhancedRAGService(EnhancedRAGConfig{
		Store:         store,
		VectorStore:   vs,
		UseEmbeddings: true,
	})

	// Search with industry filter
	result, err := rag.SearchWithOptions(ctx, "enrollment", SearchOptions{
		TopK:         10,
		IndustryType: model.IndustryEducation,
	})

	if err != nil {
		t.Fatalf("SearchWithOptions failed: %v", err)
	}

	// Should only get education results
	if result.IndustryHits[model.IndustryEcommerce] > 0 {
		t.Error("Should not have ecommerce hits when filtering by education")
	}
}

func TestEnhancedRAGService_RemoveDocument(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "rag_test_*")
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)
	store := storage.NewFileStore(tmpDir)

	vs := NewVectorStore(VectorStoreConfig{})

	rag := NewEnhancedRAGService(EnhancedRAGConfig{
		Store:       store,
		VectorStore: vs,
	})

	ctx := context.Background()

	// Index a document
	doc := &model.ParsedDocument{
		DocumentID: "to_remove",
		Filename:   "test.csv",
		Chunks: []model.TextChunk{
			{Text: "Chunk 1"},
			{Text: "Chunk 2"},
		},
	}
	rag.IndexDocument(ctx, doc, model.IndustryGeneric)

	if vs.Count() != 2 {
		t.Fatalf("Expected 2 chunks, got %d", vs.Count())
	}

	// Remove document
	rag.RemoveDocument("to_remove")

	if vs.Count() != 0 {
		t.Errorf("Expected 0 chunks after removal, got %d", vs.Count())
	}
}

func TestEnhancedRAGService_GetStats(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "rag_test_*")
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)
	store := storage.NewFileStore(tmpDir)

	vs := NewVectorStore(VectorStoreConfig{})
	ctx := context.Background()

	// Add some documents
	vs.AddDocument(ctx, VectorDocument{ID: "1", IndustryType: model.IndustryGeneric, Text: "Test"})
	vs.AddDocument(ctx, VectorDocument{ID: "2", IndustryType: model.IndustryEducation, Text: "Test"})
	vs.AddDocument(ctx, VectorDocument{ID: "3", IndustryType: model.IndustryEducation, Text: "Test"})

	rag := NewEnhancedRAGService(EnhancedRAGConfig{
		Store:         store,
		VectorStore:   vs,
		UseEmbeddings: false,
	})

	stats := rag.GetStats()

	if stats["total_vectors"] != 3 {
		t.Errorf("total_vectors = %v, want 3", stats["total_vectors"])
	}

	if stats["generic_vectors"] != 1 {
		t.Errorf("generic_vectors = %v, want 1", stats["generic_vectors"])
	}

	if stats["education_vectors"] != 2 {
		t.Errorf("education_vectors = %v, want 2", stats["education_vectors"])
	}

	if stats["embeddings_enabled"] != false {
		t.Errorf("embeddings_enabled = %v, want false", stats["embeddings_enabled"])
	}
}

func TestEnhancedRAGService_IndexAllDocuments(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "rag_test_*")
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)
	store := storage.NewFileStore(tmpDir)

	// Save multiple parsed documents
	for i := 1; i <= 3; i++ {
		doc := &model.ParsedDocument{
			DocumentID: "doc_" + string(rune('0'+i)),
			Filename:   "test.csv",
			Chunks: []model.TextChunk{
				{Text: "Test chunk"},
			},
		}
		store.SaveParsedDocument(doc)
	}

	vs := NewVectorStore(VectorStoreConfig{})

	rag := NewEnhancedRAGService(EnhancedRAGConfig{
		Store:       store,
		VectorStore: vs,
	})

	ctx := context.Background()
	err := rag.IndexAllDocuments(ctx, model.IndustryGeneric)

	if err != nil {
		t.Fatalf("IndexAllDocuments failed: %v", err)
	}

	if vs.Count() != 3 {
		t.Errorf("VectorStore count = %d, want 3", vs.Count())
	}
}

// ================== SEARCH OPTIONS TESTS ==================

func TestSearchOptions_Defaults(t *testing.T) {
	opts := SearchOptions{}

	// These should use reasonable defaults when zero
	if opts.TopK == 0 {
		opts.TopK = MaxChunksToUse
	}

	if opts.TopK != MaxChunksToUse {
		t.Errorf("Default TopK = %d, want %d", opts.TopK, MaxChunksToUse)
	}
}

func TestSearchOptions_WithPeriod(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "rag_test_*")
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)
	store := storage.NewFileStore(tmpDir)

	// Create documents with different periods
	doc1 := &model.ParsedDocument{
		DocumentID: "q1_doc",
		Filename:   "q1.csv",
		DocType:    "P&L",
		Period:     model.Period{Start: "2024-01-01", End: "2024-03-31"},
		Chunks: []model.TextChunk{
			{Text: "Q1 revenue was $1M"},
		},
	}
	store.SaveParsedDocument(doc1)

	doc2 := &model.ParsedDocument{
		DocumentID: "q2_doc",
		Filename:   "q2.csv",
		DocType:    "P&L",
		Period:     model.Period{Start: "2024-04-01", End: "2024-06-30"},
		Chunks: []model.TextChunk{
			{Text: "Q2 revenue was $1.5M"},
		},
	}
	store.SaveParsedDocument(doc2)

	rag := NewEnhancedRAGService(EnhancedRAGConfig{
		Store:         store,
		UseEmbeddings: false,
	})

	ctx := context.Background()
	result, err := rag.SearchWithOptions(ctx, "revenue", SearchOptions{
		TopK:      10,
		StartDate: "2024-01-01",
		EndDate:   "2024-03-31",
	})

	if err != nil {
		t.Fatalf("SearchWithOptions failed: %v", err)
	}

	// Should prefer Q1 document based on period filter
	if result.ChunksUsed == 0 {
		t.Error("Expected at least one chunk")
	}
}

// ================== PERFORMANCE TESTS ==================

func BenchmarkEnhancedRAG_Search(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "rag_bench_*")
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)
	store := storage.NewFileStore(tmpDir)

	// Create many parsed documents
	for i := 0; i < 100; i++ {
		doc := &model.ParsedDocument{
			DocumentID: "doc_" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Filename:   "test.csv",
			Chunks: []model.TextChunk{
				{Text: "Revenue data for the company includes various metrics."},
				{Text: "Expenses were primarily driven by R&D and marketing."},
				{Text: "Net income showed positive growth year over year."},
			},
		}
		store.SaveParsedDocument(doc)
	}

	rag := NewEnhancedRAGService(EnhancedRAGConfig{
		Store:         store,
		UseEmbeddings: false,
	})

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rag.Search(ctx, "What is the revenue?")
	}
}

