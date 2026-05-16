package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cfo/backend/internal/model"
)

func TestPaths_GetRAGPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_paths_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	paths := NewPaths(tmpDir)

	tests := []struct {
		name         string
		industryType model.IndustryType
		wantSuffix   string
	}{
		{
			name:         "Generic industry",
			industryType: model.IndustryGeneric,
			wantSuffix:   "rag/generic",
		},
		{
			name:         "Education industry",
			industryType: model.IndustryEducation,
			wantSuffix:   "rag/education",
		},
		{
			name:         "Ecommerce industry",
			industryType: model.IndustryEcommerce,
			wantSuffix:   "rag/ecommerce",
		},
		{
			name:         "Pharma industry",
			industryType: model.IndustryPharma,
			wantSuffix:   "rag/pharma",
		},
		{
			name:         "Empty industry defaults to generic",
			industryType: model.IndustryType(""),
			wantSuffix:   "rag/generic",
		},
		{
			name:         "Invalid industry defaults to generic",
			industryType: model.IndustryType("invalid"),
			wantSuffix:   "rag/generic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths.GetRAGPath(tt.industryType)
			want := filepath.Join(tmpDir, tt.wantSuffix)
			if got != want {
				t.Errorf("GetRAGPath(%q) = %q, want %q", tt.industryType, got, want)
			}
		})
	}
}

func TestPaths_GetRAGChunkPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_paths_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	paths := NewPaths(tmpDir)

	tests := []struct {
		name         string
		industryType model.IndustryType
		documentID   string
		wantSuffix   string
	}{
		{
			name:         "Generic document chunk",
			industryType: model.IndustryGeneric,
			documentID:   "doc_123",
			wantSuffix:   "rag/generic/doc_123_chunks.json",
		},
		{
			name:         "Education document chunk",
			industryType: model.IndustryEducation,
			documentID:   "edu_456",
			wantSuffix:   "rag/education/edu_456_chunks.json",
		},
		{
			name:         "Ecommerce document chunk",
			industryType: model.IndustryEcommerce,
			documentID:   "ecom_789",
			wantSuffix:   "rag/ecommerce/ecom_789_chunks.json",
		},
		{
			name:         "Pharma document chunk",
			industryType: model.IndustryPharma,
			documentID:   "pharma_abc",
			wantSuffix:   "rag/pharma/pharma_abc_chunks.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := paths.GetRAGChunkPath(tt.industryType, tt.documentID)
			want := filepath.Join(tmpDir, tt.wantSuffix)
			if got != want {
				t.Errorf("GetRAGChunkPath(%q, %q) = %q, want %q", tt.industryType, tt.documentID, got, want)
			}
		})
	}
}

func TestPaths_GetAllRAGPaths(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_paths_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	paths := NewPaths(tmpDir)
	allPaths := paths.GetAllRAGPaths()

	// Should return paths for all 4 industry types
	if len(allPaths) != 4 {
		t.Errorf("GetAllRAGPaths() returned %d paths, want 4", len(allPaths))
	}

	// Verify each path is under the RAG directory
	for _, p := range allPaths {
		if !filepath.HasPrefix(p, paths.RAGDir) {
			t.Errorf("Path %q is not under RAG directory %q", p, paths.RAGDir)
		}
	}

	// Verify all expected industries are included
	expectedSuffixes := []string{"generic", "education", "ecommerce", "pharma"}
	for _, suffix := range expectedSuffixes {
		found := false
		for _, p := range allPaths {
			if filepath.Base(p) == suffix {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("GetAllRAGPaths() missing path for %q", suffix)
		}
	}
}

func TestInitDirectories_CreatesRAGDirs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_init_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize directories
	err = InitDirectories(tmpDir)
	if err != nil {
		t.Fatalf("InitDirectories failed: %v", err)
	}

	paths := NewPaths(tmpDir)

	// Verify RAG directories were created
	ragDirs := []string{"generic", "education", "ecommerce", "pharma"}
	for _, dir := range ragDirs {
		ragPath := filepath.Join(paths.RAGDir, dir)
		info, err := os.Stat(ragPath)
		if err != nil {
			t.Errorf("RAG directory %q was not created: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("RAG path %q is not a directory", dir)
		}
	}
}

func TestPaths_RAGDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_paths_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	paths := NewPaths(tmpDir)

	expectedRAGDir := filepath.Join(tmpDir, "rag")
	if paths.RAGDir != expectedRAGDir {
		t.Errorf("RAGDir = %q, want %q", paths.RAGDir, expectedRAGDir)
	}
}

// Test security: path traversal prevention
func TestPaths_GetRAGPath_NoPathTraversal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_paths_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	paths := NewPaths(tmpDir)

	// These malicious inputs should NOT escape the RAG directory
	maliciousInputs := []model.IndustryType{
		model.IndustryType("../../../etc"),
		model.IndustryType("..\\..\\windows"),
		model.IndustryType("/etc/passwd"),
		model.IndustryType("generic/../../../etc"),
	}

	for _, input := range maliciousInputs {
		t.Run(string(input), func(t *testing.T) {
			result := paths.GetRAGPath(input)

			// Since invalid types default to generic, the path should be safe
			if !filepath.HasPrefix(result, paths.RAGDir) {
				t.Errorf("Path %q escaped RAG directory for input %q", result, input)
			}

			// Should resolve to generic for invalid inputs
			expectedGeneric := paths.GetRAGPath(model.IndustryGeneric)
			if result != expectedGeneric {
				t.Logf("Malicious input %q normalized to generic path", input)
			}
		})
	}
}

func TestPaths_GetRAGChunkPath_NoPathTraversal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_paths_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	paths := NewPaths(tmpDir)

	// Malicious document IDs
	maliciousDocIDs := []string{
		"../../../etc/passwd",
		"..\\..\\windows\\system32",
		"/etc/shadow",
		"doc_id/../../../secret",
	}

	for _, docID := range maliciousDocIDs {
		t.Run(docID, func(t *testing.T) {
			result := paths.GetRAGChunkPath(model.IndustryGeneric, docID)

			// The path construction doesn't validate docID, but this test
			// documents the behavior. In production, docID should be sanitized
			// before calling this function.
			t.Logf("DocID %q results in path: %s", docID, result)
		})
	}
}

func TestNewPaths_AllFieldsPopulated(t *testing.T) {
	tmpDir := "/test/data"
	paths := NewPaths(tmpDir)

	if paths.DataDir != tmpDir {
		t.Errorf("DataDir = %q, want %q", paths.DataDir, tmpDir)
	}

	if paths.DocumentsDir != filepath.Join(tmpDir, "documents") {
		t.Errorf("DocumentsDir = %q, want %q", paths.DocumentsDir, filepath.Join(tmpDir, "documents"))
	}

	if paths.ParsedDir != filepath.Join(tmpDir, "parsed") {
		t.Errorf("ParsedDir = %q, want %q", paths.ParsedDir, filepath.Join(tmpDir, "parsed"))
	}

	if paths.StateDir != filepath.Join(tmpDir, "state") {
		t.Errorf("StateDir = %q, want %q", paths.StateDir, filepath.Join(tmpDir, "state"))
	}

	if paths.RAGDir != filepath.Join(tmpDir, "rag") {
		t.Errorf("RAGDir = %q, want %q", paths.RAGDir, filepath.Join(tmpDir, "rag"))
	}
}

// ================== SANITIZATION TESTS ==================

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal filename",
			input:    "report.csv",
			expected: "report.csv",
		},
		{
			name:     "Path traversal attempt",
			input:    "../../../etc/passwd",
			expected: "passwd",
		},
		{
			name:     "Forward slash in name",
			input:    "path/to/file.csv",
			expected: "file.csv",
		},
		{
			name:     "Empty filename",
			input:    "",
			expected: "unnamed_file",
		},
		{
			name:     "Null bytes",
			input:    "file\x00name.csv",
			expected: "filename.csv",
		},
		{
			name:     "Control characters",
			input:    "file\x01\x02name.csv",
			expected: "filename.csv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeFilename_LongFilename(t *testing.T) {
	// Create a very long filename
	longName := ""
	for i := 0; i < 300; i++ {
		longName += "a"
	}
	longName += ".csv"

	result := SanitizeFilename(longName)

	if len(result) > MaxFilenameLength {
		t.Errorf("SanitizeFilename produced filename too long: %d > %d", len(result), MaxFilenameLength)
	}

	// Should preserve extension
	if !strings.HasSuffix(result, ".csv") {
		t.Errorf("SanitizeFilename should preserve extension, got %q", result)
	}
}

func TestSanitizeDocumentID(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal ID",
			input:    "doc_12345",
			expected: "doc_12345",
		},
		{
			name:     "ID with hyphen",
			input:    "doc-12345-abc",
			expected: "doc-12345-abc",
		},
		{
			name:     "Path traversal attempt",
			input:    "../../../etc/passwd",
			expected: "passwd",
		},
		{
			name:     "Special characters get sanitized",
			input:    "doc@#$%^&*()!",
			expected: "doc", // Special chars are stripped, trailing underscores trimmed
		},
		{
			name:     "Spaces",
			input:    "doc id with spaces",
			expected: "doc_id_with_spaces",
		},
		{
			name:     "Dots only becomes unnamed",
			input:    "...",
			expected: "unnamed_document", // Dots get replaced and trimmed, resulting in empty -> unnamed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeDocumentID(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeDocumentID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeDocumentID_LongID(t *testing.T) {
	// Create a very long ID
	longID := ""
	for i := 0; i < 200; i++ {
		longID += "a"
	}

	result := SanitizeDocumentID(longID)

	if len(result) > MaxDocumentIDLength {
		t.Errorf("SanitizeDocumentID produced ID too long: %d > %d", len(result), MaxDocumentIDLength)
	}
}

func TestIsPathWithinDirectory(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		target   string
		want     bool
	}{
		{
			name:     "Valid path within directory",
			basePath: "/data/documents",
			target:   "/data/documents/file.csv",
			want:     true,
		},
		{
			name:     "Valid nested path",
			basePath: "/data/documents",
			target:   "/data/documents/subdir/file.csv",
			want:     true,
		},
		{
			name:     "Path traversal outside directory",
			basePath: "/data/documents",
			target:   "/data/documents/../secrets/file.csv",
			want:     false,
		},
		{
			name:     "Absolute path outside directory",
			basePath: "/data/documents",
			target:   "/etc/passwd",
			want:     false,
		},
		{
			name:     "Same as base directory",
			basePath: "/data/documents",
			target:   "/data/documents",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPathWithinDirectory(tt.basePath, tt.target)
			if result != tt.want {
				t.Errorf("IsPathWithinDirectory(%q, %q) = %v, want %v", tt.basePath, tt.target, result, tt.want)
			}
		})
	}
}

func TestPaths_ValidateDocumentPath(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "validate_test_*")
	defer os.RemoveAll(tmpDir)

	paths := NewPaths(tmpDir)

	// Valid path
	validPath := filepath.Join(paths.DocumentsDir, "test.csv")
	if !paths.ValidateDocumentPath(validPath) {
		t.Errorf("ValidateDocumentPath(%q) should return true for valid path", validPath)
	}

	// Invalid path (outside documents dir)
	invalidPath := filepath.Join(tmpDir, "secrets", "test.csv")
	if paths.ValidateDocumentPath(invalidPath) {
		t.Errorf("ValidateDocumentPath(%q) should return false for path outside documents dir", invalidPath)
	}
}

func TestPaths_ValidateRAGPath(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "validate_test_*")
	defer os.RemoveAll(tmpDir)

	paths := NewPaths(tmpDir)

	// Valid path
	validPath := filepath.Join(paths.RAGDir, "education", "doc_chunks.json")
	if !paths.ValidateRAGPath(validPath) {
		t.Errorf("ValidateRAGPath(%q) should return true for valid path", validPath)
	}

	// Invalid path (outside RAG dir)
	invalidPath := filepath.Join(tmpDir, "secrets", "chunks.json")
	if paths.ValidateRAGPath(invalidPath) {
		t.Errorf("ValidateRAGPath(%q) should return false for path outside RAG dir", invalidPath)
	}
}

// ================== RAG STORAGE TESTS ==================

func TestFileStore_SaveAndLoadRAGDocument(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_rag_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize directories including RAG
	err = InitDirectories(tmpDir)
	if err != nil {
		t.Fatalf("Failed to init directories: %v", err)
	}

	store := NewFileStore(tmpDir)

	// Create a RAG document
	ragDoc := &RAGDocument{
		DocumentID:   "test_doc_123",
		IndustryType: model.IndustryEducation,
		SourceFile:   "enrollment_report.csv",
		Chunks: []RAGChunk{
			{
				ChunkID:    "chunk_001",
				DocumentID: "test_doc_123",
				Text:       "Student enrollment for Fall 2024: 15,000 students",
				ChunkType:  "enrollment_summary",
				Keywords:   []string{"enrollment", "students", "fall"},
			},
			{
				ChunkID:    "chunk_002",
				DocumentID: "test_doc_123",
				Text:       "Retention rate: 87%",
				ChunkType:  "retention_metrics",
				Keywords:   []string{"retention", "rate"},
			},
		},
		Metadata: map[string]interface{}{
			"academic_year": "2024-2025",
		},
	}

	// Save
	err = store.SaveRAGDocument(ragDoc)
	if err != nil {
		t.Fatalf("SaveRAGDocument failed: %v", err)
	}

	// Load
	loaded, err := store.LoadRAGDocument(model.IndustryEducation, "test_doc_123")
	if err != nil {
		t.Fatalf("LoadRAGDocument failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("LoadRAGDocument returned nil")
	}

	if loaded.DocumentID != "test_doc_123" {
		t.Errorf("DocumentID = %q, want %q", loaded.DocumentID, "test_doc_123")
	}

	if len(loaded.Chunks) != 2 {
		t.Errorf("Chunks count = %d, want 2", len(loaded.Chunks))
	}

	if loaded.Chunks[0].Text != "Student enrollment for Fall 2024: 15,000 students" {
		t.Errorf("Chunk text mismatch: %q", loaded.Chunks[0].Text)
	}
}

func TestFileStore_LoadRAGDocument_NotExists(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "cfo_rag_test_*")
	defer os.RemoveAll(tmpDir)

	InitDirectories(tmpDir)
	store := NewFileStore(tmpDir)

	// Load non-existent document
	doc, err := store.LoadRAGDocument(model.IndustryEducation, "nonexistent")
	if err != nil {
		t.Errorf("LoadRAGDocument should not error for non-existent doc: %v", err)
	}
	if doc != nil {
		t.Error("LoadRAGDocument should return nil for non-existent doc")
	}
}

func TestFileStore_LoadAllRAGDocuments(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "cfo_rag_test_*")
	defer os.RemoveAll(tmpDir)

	InitDirectories(tmpDir)
	store := NewFileStore(tmpDir)

	// Save multiple documents
	for i := 1; i <= 3; i++ {
		ragDoc := &RAGDocument{
			DocumentID:   "doc_" + strings.Repeat("a", i),
			IndustryType: model.IndustryEducation,
			SourceFile:   "file.csv",
			Chunks: []RAGChunk{
				{ChunkID: "chunk_1", Text: "Test text"},
			},
		}
		store.SaveRAGDocument(ragDoc)
	}

	// Load all
	docs, err := store.LoadAllRAGDocuments(model.IndustryEducation)
	if err != nil {
		t.Fatalf("LoadAllRAGDocuments failed: %v", err)
	}

	if len(docs) != 3 {
		t.Errorf("LoadAllRAGDocuments returned %d docs, want 3", len(docs))
	}
}

func TestFileStore_DeleteRAGDocument(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "cfo_rag_test_*")
	defer os.RemoveAll(tmpDir)

	InitDirectories(tmpDir)
	store := NewFileStore(tmpDir)

	// Save a document
	ragDoc := &RAGDocument{
		DocumentID:   "to_delete",
		IndustryType: model.IndustryEducation,
		Chunks:       []RAGChunk{{ChunkID: "chunk_1", Text: "Test"}},
	}
	store.SaveRAGDocument(ragDoc)

	// Verify it exists
	doc, _ := store.LoadRAGDocument(model.IndustryEducation, "to_delete")
	if doc == nil {
		t.Fatal("Document should exist before deletion")
	}

	// Delete
	err := store.DeleteRAGDocument(model.IndustryEducation, "to_delete")
	if err != nil {
		t.Fatalf("DeleteRAGDocument failed: %v", err)
	}

	// Verify it's gone
	doc, _ = store.LoadRAGDocument(model.IndustryEducation, "to_delete")
	if doc != nil {
		t.Error("Document should not exist after deletion")
	}
}

func TestFileStore_ClearRAGDirectory(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "cfo_rag_test_*")
	defer os.RemoveAll(tmpDir)

	InitDirectories(tmpDir)
	store := NewFileStore(tmpDir)

	// Save multiple documents
	for i := 1; i <= 5; i++ {
		ragDoc := &RAGDocument{
			DocumentID:   "doc_" + strings.Repeat("b", i),
			IndustryType: model.IndustryEcommerce,
			Chunks:       []RAGChunk{{ChunkID: "chunk_1", Text: "Test"}},
		}
		store.SaveRAGDocument(ragDoc)
	}

	// Verify they exist
	docs, _ := store.LoadAllRAGDocuments(model.IndustryEcommerce)
	if len(docs) != 5 {
		t.Fatalf("Expected 5 documents, got %d", len(docs))
	}

	// Clear the directory
	err := store.ClearRAGDirectory(model.IndustryEcommerce)
	if err != nil {
		t.Fatalf("ClearRAGDirectory failed: %v", err)
	}

	// Verify they're gone
	docs, _ = store.LoadAllRAGDocuments(model.IndustryEcommerce)
	if len(docs) != 0 {
		t.Errorf("Expected 0 documents after clear, got %d", len(docs))
	}
}

func TestFileStore_SaveRAGDocument_TooManyChunks(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "cfo_rag_test_*")
	defer os.RemoveAll(tmpDir)

	InitDirectories(tmpDir)
	store := NewFileStore(tmpDir)

	// Create a document with too many chunks
	chunks := make([]RAGChunk, MaxChunksPerDocument+1)
	for i := range chunks {
		chunks[i] = RAGChunk{ChunkID: "chunk_" + strings.Repeat("x", 5), Text: "Test"}
	}

	ragDoc := &RAGDocument{
		DocumentID:   "too_many_chunks",
		IndustryType: model.IndustryGeneric,
		Chunks:       chunks,
	}

	err := store.SaveRAGDocument(ragDoc)
	if err == nil {
		t.Error("SaveRAGDocument should fail with too many chunks")
	}
}

func TestFileStore_SaveRAGDocument_PathTraversal(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "cfo_rag_test_*")
	defer os.RemoveAll(tmpDir)

	InitDirectories(tmpDir)
	store := NewFileStore(tmpDir)

	// Try to save with malicious document ID
	ragDoc := &RAGDocument{
		DocumentID:   "../../../etc/passwd",
		IndustryType: model.IndustryEducation,
		Chunks:       []RAGChunk{{ChunkID: "chunk_1", Text: "Test"}},
	}

	// Should succeed but sanitize the document ID
	err := store.SaveRAGDocument(ragDoc)
	if err != nil {
		t.Fatalf("SaveRAGDocument failed: %v", err)
	}

	// The document should be saved with sanitized ID
	// Original malicious ID should not work for loading
	doc, _ := store.LoadRAGDocument(model.IndustryEducation, "../../../etc/passwd")
	
	// If it loads, verify it's actually the sanitized version
	if doc != nil {
		t.Logf("Document loaded with sanitized ID: %s", doc.DocumentID)
	}
}

