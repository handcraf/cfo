// Package storage provides comprehensive security tests for storage operations.
// These tests ensure protection against common security vulnerabilities.
package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cfo/backend/internal/model"
)

// ================== PATH TRAVERSAL SECURITY TESTS ==================

func TestSecurity_PathTraversal_Filenames(t *testing.T) {
	maliciousFilenames := []struct {
		name     string
		input    string
		wantSafe bool // Should be sanitized to a safe value
	}{
		{"Unix path traversal", "../../../etc/passwd", true},
		{"Windows path traversal", "..\\..\\..\\windows\\system32\\config", true},
		{"Hidden file creation", ".htaccess", true},
		{"Double encoding", "..%2F..%2F..%2Fetc%2Fpasswd", true},
		{"Null byte injection", "file\x00.csv", true},
		{"Unicode path traversal", "..%c0%af..%c0%af", true},
		{"Mixed separators", "../foo\\..\\etc/passwd", true},
		{"URL encoded dots", "%2e%2e%2f%2e%2e%2f", true},
		{"Long path overflow", strings.Repeat("a", 500) + ".csv", true},
		{"Control characters", "file\x01\x02\x03.csv", true},
	}

	for _, tt := range maliciousFilenames {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)

			// Should not contain path traversal sequences
			if strings.Contains(result, "..") {
				t.Errorf("Sanitized filename contains '..': %q", result)
			}

			// Should not contain path separators
			if strings.ContainsAny(result, "/\\") {
				t.Errorf("Sanitized filename contains path separators: %q", result)
			}

			// Should not be empty
			if result == "" {
				t.Error("Sanitized filename is empty")
			}

			// Should not exceed max length
			if len(result) > MaxFilenameLength {
				t.Errorf("Sanitized filename too long: %d > %d", len(result), MaxFilenameLength)
			}
		})
	}
}

func TestSecurity_PathTraversal_DocumentIDs(t *testing.T) {
	maliciousIDs := []struct {
		name  string
		input string
	}{
		{"Path traversal", "../../../etc/passwd"},
		{"Windows path", "..\\..\\system32"},
		{"Absolute path", "/etc/passwd"},
		{"URL encoded", "%2e%2e%2fetc"},
		{"Null byte", "doc\x00id"},
		{"Special chars", "doc;rm -rf /"},
		{"SQL injection attempt", "'; DROP TABLE documents;--"},
	}

	for _, tt := range maliciousIDs {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeDocumentID(tt.input)

			// Should only contain safe characters
			for _, r := range result {
				if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
					(r >= '0' && r <= '9') || r == '_' || r == '-') {
					t.Errorf("Sanitized ID contains unsafe character %q in %q", r, result)
				}
			}
		})
	}
}

func TestSecurity_PathValidation(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "security_test_*")
	defer os.RemoveAll(tmpDir)

	paths := NewPaths(tmpDir)

	tests := []struct {
		name       string
		basePath   string
		targetPath string
		wantValid  bool
	}{
		{
			name:       "Valid document path",
			basePath:   paths.DocumentsDir,
			targetPath: filepath.Join(paths.DocumentsDir, "report.csv"),
			wantValid:  true,
		},
		{
			name:       "Path traversal attack",
			basePath:   paths.DocumentsDir,
			targetPath: filepath.Join(paths.DocumentsDir, "..", "secrets", "file.txt"),
			wantValid:  false,
		},
		{
			name:       "Absolute path outside",
			basePath:   paths.DocumentsDir,
			targetPath: "/etc/passwd",
			wantValid:  false,
		},
		{
			name:       "Symlink-like traversal",
			basePath:   paths.DocumentsDir,
			targetPath: filepath.Join(paths.DocumentsDir, "subdir", "..", "..", "secrets"),
			wantValid:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPathWithinDirectory(tt.basePath, tt.targetPath)
			if result != tt.wantValid {
				t.Errorf("IsPathWithinDirectory() = %v, want %v", result, tt.wantValid)
			}
		})
	}
}

// ================== FILE OPERATION SECURITY TESTS ==================

func TestSecurity_FileSizeLimits(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "filesize_test_*")
	defer os.RemoveAll(tmpDir)

	if err := InitDirectories(tmpDir); err != nil {
		t.Fatalf("InitDirectories failed: %v", err)
	}

	store := NewFileStore(tmpDir)

	// Test with oversized file
	oversizedData := bytes.Repeat([]byte("x"), MaxUploadSize+1000)
	reader := bytes.NewReader(oversizedData)

	_, err := store.SaveDocument("large_file.csv", reader)
	if err == nil {
		t.Error("Expected error for oversized file, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "too large") {
		t.Errorf("Expected 'too large' error, got: %v", err)
	}
}

func TestSecurity_RAGChunkLimits(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "chunk_limit_test_*")
	defer os.RemoveAll(tmpDir)

	if err := InitDirectories(tmpDir); err != nil {
		t.Fatalf("InitDirectories failed: %v", err)
	}

	store := NewFileStore(tmpDir)

	// Create document with too many chunks
	chunks := make([]RAGChunk, MaxChunksPerDocument+10)
	for i := range chunks {
		chunks[i] = RAGChunk{ChunkID: "chunk", Text: "test"}
	}

	ragDoc := &RAGDocument{
		DocumentID:   "test_doc",
		IndustryType: model.IndustryGeneric,
		Chunks:       chunks,
	}

	err := store.SaveRAGDocument(ragDoc)
	if err == nil {
		t.Error("Expected error for too many chunks, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "too many chunks") {
		t.Errorf("Expected 'too many chunks' error, got: %v", err)
	}
}

// ================== CONCURRENT ACCESS SECURITY TESTS ==================

func TestSecurity_ConcurrentAccess(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "concurrent_test_*")
	defer os.RemoveAll(tmpDir)

	if err := InitDirectories(tmpDir); err != nil {
		t.Fatalf("InitDirectories failed: %v", err)
	}

	store := NewFileStore(tmpDir)

	// Run concurrent operations
	done := make(chan bool, 10)

	// Concurrent company saves
	for i := 0; i < 5; i++ {
		go func(id int) {
			company := &model.Company{
				Name:         "Test Company",
				IndustryType: model.IndustryGeneric,
				UpdatedAt:    time.Now(),
			}
			err := store.SaveCompany(company)
			if err != nil {
				t.Errorf("Concurrent SaveCompany failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Concurrent company loads
	for i := 0; i < 5; i++ {
		go func(id int) {
			_, err := store.LoadCompany()
			if err != nil {
				t.Errorf("Concurrent LoadCompany failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

// ================== INPUT VALIDATION TESTS ==================

func TestSecurity_IndustryTypeValidation(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"generic", true},
		{"education", true},
		{"ecommerce", true},
		{"pharma", true},
		{"", false},
		{"invalid", false},
		{"Generic", false},    // Case sensitive
		{"EDUCATION", false},  // Case sensitive
		{"../etc/passwd", false},
		{"<script>alert('xss')</script>", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := model.IsValidIndustryType(model.IndustryType(tt.input))
			if result != tt.valid {
				t.Errorf("IsValidIndustryType(%q) = %v, want %v", tt.input, result, tt.valid)
			}
		})
	}
}

// ================== ATOMIC WRITE TESTS ==================

func TestSecurity_AtomicWrites(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "atomic_test_*")
	defer os.RemoveAll(tmpDir)

	if err := InitDirectories(tmpDir); err != nil {
		t.Fatalf("InitDirectories failed: %v", err)
	}

	store := NewFileStore(tmpDir)

	// Save initial company
	company := &model.Company{
		Name:           "Original Company",
		IndustryType:   model.IndustryGeneric,
		SetupCompleted: true,
	}

	if err := store.SaveCompany(company); err != nil {
		t.Fatalf("Initial SaveCompany failed: %v", err)
	}

	// Update company
	company.Name = "Updated Company"
	if err := store.SaveCompany(company); err != nil {
		t.Fatalf("Update SaveCompany failed: %v", err)
	}

	// Verify update was atomic
	loaded, err := store.LoadCompany()
	if err != nil {
		t.Fatalf("LoadCompany failed: %v", err)
	}

	if loaded.Name != "Updated Company" {
		t.Errorf("Company name = %q, want %q", loaded.Name, "Updated Company")
	}

	// Verify no temp files left behind
	files, _ := os.ReadDir(filepath.Dir(store.GetPaths().CompanyFilePath()))
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".tmp_") {
			t.Errorf("Temp file left behind: %s", f.Name())
		}
	}
}

// ================== RESET/CLEANUP SECURITY TESTS ==================

func TestSecurity_ResetCleansAllData(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "reset_test_*")
	defer os.RemoveAll(tmpDir)

	if err := InitDirectories(tmpDir); err != nil {
		t.Fatalf("InitDirectories failed: %v", err)
	}

	store := NewFileStore(tmpDir)

	// Create various data
	company := &model.Company{Name: "Test", SetupCompleted: true}
	store.SaveCompany(company)

	docList := &model.DocumentList{Documents: []model.Document{{ID: "doc1"}}}
	store.SaveDocumentList(docList)

	ragDoc := &RAGDocument{
		DocumentID:   "rag_test",
		IndustryType: model.IndustryEducation,
		Chunks:       []RAGChunk{{ChunkID: "c1", Text: "test"}},
	}
	store.SaveRAGDocument(ragDoc)

	// Save a test file
	store.SaveDocument("test.csv", strings.NewReader("a,b,c\n1,2,3"))

	// Reset all data
	err := store.ResetAllData()
	if err != nil {
		t.Fatalf("ResetAllData failed: %v", err)
	}

	// Verify company is gone
	loadedCompany, _ := store.LoadCompany()
	if loadedCompany != nil {
		t.Error("Company should be nil after reset")
	}

	// Verify documents list is empty
	loadedDocs, _ := store.LoadDocumentList()
	if len(loadedDocs.Documents) > 0 {
		t.Error("Documents should be empty after reset")
	}

	// Verify RAG documents are gone
	ragDocs, _ := store.LoadAllRAGDocuments(model.IndustryEducation)
	if len(ragDocs) > 0 {
		t.Error("RAG documents should be empty after reset")
	}
}

// ================== EDGE CASE TESTS ==================

func TestSecurity_EmptyInputs(t *testing.T) {
	// Empty filename
	result := SanitizeFilename("")
	if result == "" {
		t.Error("SanitizeFilename should not return empty for empty input")
	}

	// Empty document ID
	result = SanitizeDocumentID("")
	if result == "" {
		t.Error("SanitizeDocumentID should not return empty for empty input")
	}
}

func TestSecurity_UnicodeHandling(t *testing.T) {
	unicodeInputs := []struct {
		name  string
		input string
	}{
		{"Chinese characters", "文档.csv"},
		{"Japanese characters", "ドキュメント.csv"},
		{"Arabic characters", "وثيقة.csv"},
		{"Emoji", "📄report.csv"},
		{"Mixed", "report_文档_📄.csv"},
	}

	for _, tt := range unicodeInputs {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			// Should not panic and should return something
			if result == "" {
				t.Error("SanitizeFilename returned empty for unicode input")
			}
		})
	}
}

