package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cfo/backend/internal/model"
)

func TestInitDirectories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	err = InitDirectories(tmpDir)
	if err != nil {
		t.Fatalf("InitDirectories failed: %v", err)
	}

	// Verify all directories were created
	paths := NewPaths(tmpDir)
	dirs := []string{
		paths.DocumentsDir,
		paths.ParsedDir,
		paths.StateDir,
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Directory not created: %s", dir)
		}
	}
}

func TestInitDirectories_AlreadyExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_storage_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Call twice - should not error
	err = InitDirectories(tmpDir)
	if err != nil {
		t.Fatalf("First InitDirectories failed: %v", err)
	}

	err = InitDirectories(tmpDir)
	if err != nil {
		t.Fatalf("Second InitDirectories should not fail: %v", err)
	}
}

func TestFileStore_SaveAndLoadCompany(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	company := &model.Company{
		Name:           "Test Corp",
		Industry:       "Technology",
		FiscalYearEnd:  "December",
		Currency:       "USD",
		SetupCompleted: true,
		CreatedAt:      time.Now(),
	}

	// Save company
	err := store.SaveCompany(company)
	if err != nil {
		t.Fatalf("SaveCompany failed: %v", err)
	}

	// Load company
	loaded, err := store.LoadCompany()
	if err != nil {
		t.Fatalf("LoadCompany failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("Loaded company is nil")
	}

	if loaded.Name != company.Name {
		t.Errorf("Name mismatch: expected %s, got %s", company.Name, loaded.Name)
	}

	if loaded.Industry != company.Industry {
		t.Errorf("Industry mismatch: expected %s, got %s", company.Industry, loaded.Industry)
	}

	if !loaded.SetupCompleted {
		t.Error("SetupCompleted should be true")
	}
}

func TestFileStore_LoadCompany_NotExists(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	// Load company before saving
	loaded, err := store.LoadCompany()
	if err != nil {
		t.Fatalf("LoadCompany should not error for non-existent file: %v", err)
	}

	if loaded != nil {
		t.Error("Loaded company should be nil when file doesn't exist")
	}
}

func TestFileStore_SaveAndLoadDocumentList(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	docs := &model.DocumentList{
		Documents: []model.Document{
			{
				ID:          "doc_1",
				Filename:    "test1.csv",
				DocType:     model.DocTypePnL,
				PeriodStart: "2024-01-01",
				PeriodEnd:   "2024-12-31",
				UploadedAt:  time.Now(),
			},
			{
				ID:          "doc_2",
				Filename:    "test2.csv",
				DocType:     model.DocTypeBalanceSheet,
				PeriodStart: "2024-01-01",
				PeriodEnd:   "2024-12-31",
				UploadedAt:  time.Now(),
			},
		},
	}

	// Save
	err := store.SaveDocumentList(docs)
	if err != nil {
		t.Fatalf("SaveDocumentList failed: %v", err)
	}

	// Load
	loaded, err := store.LoadDocumentList()
	if err != nil {
		t.Fatalf("LoadDocumentList failed: %v", err)
	}

	if len(loaded.Documents) != 2 {
		t.Errorf("Expected 2 documents, got %d", len(loaded.Documents))
	}

	if loaded.Documents[0].ID != "doc_1" {
		t.Errorf("First doc ID mismatch: expected doc_1, got %s", loaded.Documents[0].ID)
	}
}

func TestFileStore_LoadDocumentList_Empty(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	// Load before any documents
	loaded, err := store.LoadDocumentList()
	if err != nil {
		t.Fatalf("LoadDocumentList should not error: %v", err)
	}

	if loaded == nil {
		t.Fatal("Loaded list should not be nil")
	}

	if len(loaded.Documents) != 0 {
		t.Errorf("Expected 0 documents, got %d", len(loaded.Documents))
	}
}

func TestFileStore_SaveDocument(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	content := "Metric,Value\nRevenue,1000000"
	reader := strings.NewReader(content)

	path, err := store.SaveDocument("test.csv", reader)
	if err != nil {
		t.Fatalf("SaveDocument failed: %v", err)
	}

	if !strings.HasSuffix(path, "test.csv") {
		t.Errorf("Path should end with test.csv: %s", path)
	}

	// Verify file exists and content is correct
	savedContent, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	if string(savedContent) != content {
		t.Errorf("Content mismatch: expected %s, got %s", content, string(savedContent))
	}
}

func TestFileStore_SaveDocument_DuplicateFilename(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	// Save first file
	path1, err := store.SaveDocument("test.csv", strings.NewReader("content1"))
	if err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// Save second file with same name
	path2, err := store.SaveDocument("test.csv", strings.NewReader("content2"))
	if err != nil {
		t.Fatalf("Second save failed: %v", err)
	}

	// Paths should be different (timestamped)
	if path1 == path2 {
		t.Error("Duplicate filenames should result in different paths")
	}

	// Both files should exist
	if _, err := os.Stat(path1); os.IsNotExist(err) {
		t.Error("First file should exist")
	}

	if _, err := os.Stat(path2); os.IsNotExist(err) {
		t.Error("Second file should exist")
	}
}

func TestFileStore_SaveAndLoadParsedDocument(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	parsed := &model.ParsedDocument{
		DocumentID: "doc_1",
		DocType:    model.DocTypePnL,
		Period: model.Period{
			Start: "2024-01-01",
			End:   "2024-12-31",
		},
		Data: map[string]float64{
			"revenue":  1000000,
			"expenses": 800000,
			"cash":     500000,
		},
		RawText:  "Sample text",
		Metadata: map[string]interface{}{"source": "test"},
		ParsedAt: time.Now(),
	}

	// Save
	err := store.SaveParsedDocument(parsed)
	if err != nil {
		t.Fatalf("SaveParsedDocument failed: %v", err)
	}

	// Load
	loaded, err := store.LoadParsedDocument("doc_1")
	if err != nil {
		t.Fatalf("LoadParsedDocument failed: %v", err)
	}

	if loaded == nil {
		t.Fatal("Loaded document is nil")
	}

	if loaded.DocumentID != "doc_1" {
		t.Errorf("DocumentID mismatch: expected doc_1, got %s", loaded.DocumentID)
	}

	if loaded.Data["revenue"] != 1000000 {
		t.Errorf("Revenue mismatch: expected 1000000, got %f", loaded.Data["revenue"])
	}
}

func TestFileStore_LoadParsedDocument_NotExists(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	loaded, err := store.LoadParsedDocument("nonexistent")
	if err != nil {
		t.Fatalf("Should not error for non-existent: %v", err)
	}

	if loaded != nil {
		t.Error("Should return nil for non-existent document")
	}
}

func TestFileStore_LoadAllParsedDocuments(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	// Save multiple documents
	for i := 1; i <= 3; i++ {
		parsed := &model.ParsedDocument{
			DocumentID: "doc_" + string(rune('0'+i)),
			DocType:    model.DocTypePnL,
			Data:       map[string]float64{"revenue": float64(i * 1000000)},
			ParsedAt:   time.Now(),
		}
		store.SaveParsedDocument(parsed)
	}

	// Load all
	docs, err := store.LoadAllParsedDocuments()
	if err != nil {
		t.Fatalf("LoadAllParsedDocuments failed: %v", err)
	}

	if len(docs) != 3 {
		t.Errorf("Expected 3 documents, got %d", len(docs))
	}
}

func TestFileStore_LoadAllParsedDocuments_Empty(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)

	docs, err := store.LoadAllParsedDocuments()
	if err != nil {
		t.Fatalf("LoadAllParsedDocuments should not error: %v", err)
	}

	if len(docs) != 0 {
		t.Errorf("Expected 0 documents, got %d", len(docs))
	}
}

func TestFileStore_GetPaths(t *testing.T) {
	tmpDir, cleanup := setupTempDir(t)
	defer cleanup()

	store := NewFileStore(tmpDir)
	paths := store.GetPaths()

	if paths == nil {
		t.Fatal("GetPaths returned nil")
	}

	if paths.DataDir != tmpDir {
		t.Errorf("DataDir mismatch: expected %s, got %s", tmpDir, paths.DataDir)
	}

	if !strings.Contains(paths.DocumentsDir, "documents") {
		t.Errorf("DocumentsDir should contain 'documents': %s", paths.DocumentsDir)
	}
}

// Helper function
func setupTempDir(t *testing.T) (string, func()) {
	tmpDir, err := os.MkdirTemp("", "cfo_fs_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	err = InitDirectories(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init directories: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return tmpDir, cleanup
}

func TestPaths_AllPathMethods(t *testing.T) {
	paths := NewPaths("/data")

	if paths.CompanyFilePath() != filepath.Join("/data", "state", "company.json") {
		t.Errorf("CompanyFilePath incorrect: %s", paths.CompanyFilePath())
	}

	if paths.DocumentsFilePath() != filepath.Join("/data", "state", "documents.json") {
		t.Errorf("DocumentsFilePath incorrect: %s", paths.DocumentsFilePath())
	}

	if paths.DocumentFilePath("test.csv") != filepath.Join("/data", "documents", "test.csv") {
		t.Errorf("DocumentFilePath incorrect: %s", paths.DocumentFilePath("test.csv"))
	}

	if paths.ParsedFilePath("doc_1") != filepath.Join("/data", "parsed", "doc_1.json") {
		t.Errorf("ParsedFilePath incorrect: %s", paths.ParsedFilePath("doc_1"))
	}
}

