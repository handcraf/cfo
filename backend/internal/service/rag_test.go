package service

import (
	"os"
	"testing"
	"time"

	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

func TestRAGService_ExtractKeywords(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	rag := NewRAGService(store)

	tests := []struct {
		name     string
		query    string
		expected []string // At least these should be present
	}{
		{
			name:     "Simple query",
			query:    "What is the cash position?",
			expected: []string{"cash", "position"},
		},
		{
			name:     "Query with stop words",
			query:    "How is the revenue doing this month?",
			expected: []string{"revenue", "month"},
		},
		{
			name:     "Query with financial terms",
			query:    "Explain the burn rate and runway",
			expected: []string{"burn", "rate", "runway"},
		},
		{
			name:     "Empty query",
			query:    "",
			expected: []string{},
		},
		{
			name:     "Only stop words",
			query:    "the a an is are",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := rag.extractKeywords(tt.query)

			for _, exp := range tt.expected {
				found := false
				for _, kw := range result {
					if kw == exp {
						found = true
						break
					}
				}
				if !found && len(tt.expected) > 0 {
					t.Errorf("Expected keyword '%s' not found in result %v", exp, result)
				}
			}
		})
	}
}

func TestRAGService_ScoreDocument(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	rag := NewRAGService(store)

	tests := []struct {
		name     string
		text     string
		keywords []string
		minScore int
	}{
		{
			name:     "High relevance",
			text:     "The revenue was $1M. Revenue increased. Total revenue is growing.",
			keywords: []string{"revenue"},
			minScore: 3,
		},
		{
			name:     "No relevance",
			text:     "The weather is nice today.",
			keywords: []string{"revenue", "cash", "expenses"},
			minScore: 0,
		},
		{
			name:     "Multiple keywords",
			text:     "Cash position is good. Revenue is up. Expenses are controlled.",
			keywords: []string{"cash", "revenue", "expenses"},
			minScore: 3,
		},
		{
			name:     "Case insensitive",
			text:     "CASH REVENUE EXPENSES",
			keywords: []string{"cash", "revenue", "expenses"},
			minScore: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := rag.scoreText(tt.text, tt.keywords)
			if score < tt.minScore {
				t.Errorf("Score %d is less than expected minimum %d", score, tt.minScore)
			}
		})
	}
}

func TestRAGService_SearchEnhanced(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	rag := NewRAGService(store)

	// Test SearchEnhanced returns proper structure
	result := rag.SearchEnhanced("What is our revenue?")

	// With no documents, should return empty result
	if result.Context != "" {
		t.Error("Expected empty context with no documents")
	}

	if len(result.Sources) != 0 {
		t.Error("Expected no sources with no documents")
	}

	if result.ChunksUsed != 0 {
		t.Error("Expected no chunks used with no documents")
	}
}

func TestRAGService_Search_NoDocuments(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	rag := NewRAGService(store)

	context, sources := rag.Search("What is the cash position?")

	if context != "" {
		t.Error("Expected empty context with no documents")
	}

	if len(sources) != 0 {
		t.Error("Expected no sources with no documents")
	}
}

func TestRAGService_Search_WithDocuments(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create documents with relevant content
	doc1 := &model.ParsedDocument{
		DocumentID: "doc_1",
		DocType:    model.DocTypePnL,
		RawText:    "Revenue: $1,000,000\nExpenses: $800,000\nNet Income: $200,000",
		Data:       map[string]float64{"revenue": 1000000, "expenses": 800000},
		ParsedAt:   time.Now(),
	}

	doc2 := &model.ParsedDocument{
		DocumentID: "doc_2",
		DocType:    model.DocTypeBalanceSheet,
		RawText:    "Cash: $500,000\nTotal Assets: $2,000,000\nEquity: $1,000,000",
		Data:       map[string]float64{"cash": 500000},
		ParsedAt:   time.Now(),
	}

	store.SaveParsedDocument(doc1)
	store.SaveParsedDocument(doc2)

	rag := NewRAGService(store)

	// Search for revenue
	context, sources := rag.Search("What is the revenue?")

	if context == "" {
		t.Error("Expected non-empty context")
	}

	if !findSubstring(context, "Revenue") && !findSubstring(context, "revenue") {
		t.Error("Context should contain revenue information")
	}

	if len(sources) == 0 {
		t.Error("Expected at least one source")
	}

	// Search for cash
	context2, sources2 := rag.Search("What is the cash position?")

	if context2 == "" {
		t.Error("Expected non-empty context for cash query")
	}

	if len(sources2) == 0 {
		t.Error("Expected sources for cash query")
	}
}

func TestRAGService_Search_ContextLimiting(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create a document with lots of text
	longText := ""
	for i := 0; i < 100; i++ {
		longText += "Revenue line " + string(rune(i+'0')) + " with some data about financial metrics.\n"
	}

	doc := &model.ParsedDocument{
		DocumentID: "doc_long",
		DocType:    model.DocTypePnL,
		RawText:    longText,
		Data:       map[string]float64{"revenue": 1000000},
		ParsedAt:   time.Now(),
	}

	store.SaveParsedDocument(doc)

	rag := NewRAGService(store)
	context, _ := rag.Search("What is the revenue?")

	// Context should be limited to ~2000 chars
	if len(context) > 2100 { // Some buffer for the "..."
		t.Errorf("Context too long: %d chars", len(context))
	}
}

// Helper function
func capitalizeFirst(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}

// Setup helper
func setupTestStorageForRAG(t *testing.T) (*storage.FileStore, func()) {
	tmpDir, err := os.MkdirTemp("", "cfo_rag_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	err = storage.InitDirectories(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init directories: %v", err)
	}

	store := storage.NewFileStore(tmpDir)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return store, cleanup
}
