package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cfo/backend/internal/model"
)

// Storage limits for security and performance
const (
	// MaxUploadSize is the maximum allowed file upload size (50MB)
	MaxUploadSize = 50 * 1024 * 1024
	// MaxJSONSize is the maximum allowed JSON file size for reading (10MB)
	MaxJSONSize = 10 * 1024 * 1024
	// MaxChunksPerDocument is the maximum number of chunks per document
	MaxChunksPerDocument = 1000
)

// InitDirectories creates the required directory structure
func InitDirectories(dataDir string) error {
	paths := NewPaths(dataDir)

	dirs := []string{
		paths.DocumentsDir,
		paths.ParsedDir,
		paths.StateDir,
	}

	// Add RAG directories for all industry types
	// These directories hold chunked extracted data for industry-specific RAG
	dirs = append(dirs, paths.GetAllRAGPaths()...)

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// FileStore handles all file-based storage operations.
// It provides thread-safe access to file operations with proper error handling.
type FileStore struct {
	paths *Paths
	mu    sync.RWMutex // Mutex for thread-safe file operations
}

// NewFileStore creates a new FileStore
func NewFileStore(dataDir string) *FileStore {
	return &FileStore{
		paths: NewPaths(dataDir),
	}
}

// GetPaths returns the paths configuration
func (fs *FileStore) GetPaths() *Paths {
	return fs.paths
}

// RAGChunk represents a chunk of text for RAG retrieval
type RAGChunk struct {
	ChunkID     string                 `json:"chunk_id"`
	DocumentID  string                 `json:"document_id"`
	Text        string                 `json:"text"`
	ChunkType   string                 `json:"chunk_type,omitempty"`
	Source      string                 `json:"source,omitempty"`
	Keywords    []string               `json:"relevance_keywords,omitempty"`
	Metrics     map[string]float64     `json:"metrics,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
}

// RAGDocument represents a collection of chunks for a document
type RAGDocument struct {
	DocumentID   string                 `json:"document_id"`
	IndustryType model.IndustryType     `json:"industry_type"`
	SourceFile   string                 `json:"source_file"`
	Chunks       []RAGChunk             `json:"chunks"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
	ParsedAt     time.Time              `json:"parsed_at"`
}

// SaveCompany saves company data to file with thread-safe locking.
func (fs *FileStore) SaveCompany(company *model.Company) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	company.UpdatedAt = time.Now()
	return fs.writeJSONAtomic(fs.paths.CompanyFilePath(), company)
}

// LoadCompany loads company data from file with thread-safe locking.
func (fs *FileStore) LoadCompany() (*model.Company, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	path := fs.paths.CompanyFilePath()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil // No company setup yet
	}

	var company model.Company
	if err := fs.readJSON(path, &company); err != nil {
		return nil, err
	}

	return &company, nil
}

// SaveDocumentList saves the document list to file with thread-safe locking.
func (fs *FileStore) SaveDocumentList(docs *model.DocumentList) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	docs.UpdatedAt = time.Now()
	return fs.writeJSONAtomic(fs.paths.DocumentsFilePath(), docs)
}

// LoadDocumentList loads the document list from file with thread-safe locking.
func (fs *FileStore) LoadDocumentList() (*model.DocumentList, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	path := fs.paths.DocumentsFilePath()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &model.DocumentList{Documents: []model.Document{}}, nil
	}

	var docs model.DocumentList
	if err := fs.readJSON(path, &docs); err != nil {
		return nil, err
	}

	return &docs, nil
}

// SaveDocument saves an uploaded document file with size limits and validation.
func (fs *FileStore) SaveDocument(filename string, reader io.Reader) (string, error) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Sanitize the filename to prevent path traversal
	safeFilename := SanitizeFilename(filename)
	destPath := fs.paths.DocumentFilePath(safeFilename)

	// Validate the path is within the documents directory
	if !fs.paths.ValidateDocumentPath(destPath) {
		return "", fmt.Errorf("invalid document path: attempted path traversal")
	}

	// Create unique filename if file exists
	if _, err := os.Stat(destPath); err == nil {
		ext := filepath.Ext(safeFilename)
		base := safeFilename[:len(safeFilename)-len(ext)]
		timestamp := time.Now().Format("20060102_150405")
		safeFilename = fmt.Sprintf("%s_%s%s", base, timestamp, ext)
		destPath = fs.paths.DocumentFilePath(safeFilename)
	}

	// Create file with limited reader to prevent DoS
	file, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Limit the size of uploaded files
	limitedReader := io.LimitReader(reader, MaxUploadSize+1)
	written, err := io.Copy(file, limitedReader)
	if err != nil {
		// Clean up partial file on error
		os.Remove(destPath)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	if written > MaxUploadSize {
		// File too large, clean up and return error
		os.Remove(destPath)
		return "", fmt.Errorf("file too large: maximum size is %d bytes", MaxUploadSize)
	}

	log.Printf("[FileStore] Saved document: %s (%d bytes)", safeFilename, written)
	return destPath, nil
}

// SaveParsedDocument saves parsed document data with thread-safe locking.
func (fs *FileStore) SaveParsedDocument(parsed *model.ParsedDocument) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Sanitize document ID
	safeDocID := SanitizeDocumentID(parsed.DocumentID)
	path := fs.paths.ParsedFilePath(safeDocID)

	// Validate the path
	if !fs.paths.ValidateParsedPath(path) {
		return fmt.Errorf("invalid parsed document path: attempted path traversal")
	}

	return fs.writeJSONAtomic(path, parsed)
}

// LoadParsedDocument loads parsed document data with thread-safe locking.
func (fs *FileStore) LoadParsedDocument(documentID string) (*model.ParsedDocument, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// Sanitize document ID
	safeDocID := SanitizeDocumentID(documentID)
	path := fs.paths.ParsedFilePath(safeDocID)

	// Validate the path
	if !fs.paths.ValidateParsedPath(path) {
		return nil, fmt.Errorf("invalid parsed document path: attempted path traversal")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	var parsed model.ParsedDocument
	if err := fs.readJSON(path, &parsed); err != nil {
		return nil, err
	}

	return &parsed, nil
}

// LoadAllParsedDocuments loads all parsed documents with thread-safe locking.
func (fs *FileStore) LoadAllParsedDocuments() ([]*model.ParsedDocument, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	files, err := os.ReadDir(fs.paths.ParsedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*model.ParsedDocument{}, nil
		}
		return nil, err
	}

	var docs []*model.ParsedDocument
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		// Get document ID from filename (remove .json extension)
		docID := file.Name()[:len(file.Name())-5]

		// Use the internal read (already has lock)
		path := fs.paths.ParsedFilePath(docID)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}

		var parsed model.ParsedDocument
		if err := fs.readJSON(path, &parsed); err != nil {
			log.Printf("[FileStore] Warning: failed to load parsed document %s: %v", docID, err)
			continue // Skip invalid files
		}
		docs = append(docs, &parsed)
	}

	return docs, nil
}

// ============================================================================
// RAG CHUNK STORAGE METHODS
// ============================================================================

// SaveRAGDocument saves RAG chunks for a document to the appropriate industry directory.
func (fs *FileStore) SaveRAGDocument(ragDoc *RAGDocument) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Validate industry type
	if !model.IsValidIndustryType(ragDoc.IndustryType) {
		ragDoc.IndustryType = model.IndustryGeneric
	}

	// Validate chunk count
	if len(ragDoc.Chunks) > MaxChunksPerDocument {
		return fmt.Errorf("too many chunks: maximum is %d, got %d", MaxChunksPerDocument, len(ragDoc.Chunks))
	}

	// Get the chunk file path
	path := fs.paths.GetRAGChunkPath(ragDoc.IndustryType, ragDoc.DocumentID)

	// Validate path is within RAG directory
	if !fs.paths.ValidateRAGPath(path) {
		return fmt.Errorf("invalid RAG path: attempted path traversal")
	}

	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create RAG directory: %w", err)
	}

	ragDoc.ParsedAt = time.Now()
	return fs.writeJSONAtomic(path, ragDoc)
}

// LoadRAGDocument loads RAG chunks for a document from the appropriate industry directory.
func (fs *FileStore) LoadRAGDocument(industryType model.IndustryType, documentID string) (*RAGDocument, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// Validate industry type
	if !model.IsValidIndustryType(industryType) {
		industryType = model.IndustryGeneric
	}

	path := fs.paths.GetRAGChunkPath(industryType, documentID)

	// Validate path
	if !fs.paths.ValidateRAGPath(path) {
		return nil, fmt.Errorf("invalid RAG path: attempted path traversal")
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}

	var ragDoc RAGDocument
	if err := fs.readJSON(path, &ragDoc); err != nil {
		return nil, err
	}

	return &ragDoc, nil
}

// LoadAllRAGDocuments loads all RAG documents for a specific industry type.
func (fs *FileStore) LoadAllRAGDocuments(industryType model.IndustryType) ([]*RAGDocument, error) {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	// Validate industry type
	if !model.IsValidIndustryType(industryType) {
		industryType = model.IndustryGeneric
	}

	ragDir := fs.paths.GetRAGPath(industryType)

	files, err := os.ReadDir(ragDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*RAGDocument{}, nil
		}
		return nil, err
	}

	var docs []*RAGDocument
	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		path := filepath.Join(ragDir, file.Name())
		var ragDoc RAGDocument
		if err := fs.readJSON(path, &ragDoc); err != nil {
			log.Printf("[FileStore] Warning: failed to load RAG document %s: %v", file.Name(), err)
			continue
		}
		docs = append(docs, &ragDoc)
	}

	return docs, nil
}

// DeleteRAGDocument removes RAG chunks for a specific document.
func (fs *FileStore) DeleteRAGDocument(industryType model.IndustryType, documentID string) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if !model.IsValidIndustryType(industryType) {
		industryType = model.IndustryGeneric
	}

	path := fs.paths.GetRAGChunkPath(industryType, documentID)

	// Validate path
	if !fs.paths.ValidateRAGPath(path) {
		return fmt.Errorf("invalid RAG path: attempted path traversal")
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete RAG document: %w", err)
	}

	log.Printf("[FileStore] Deleted RAG document: %s/%s", industryType, documentID)
	return nil
}

// ClearRAGDirectory removes all RAG chunks for a specific industry type.
func (fs *FileStore) ClearRAGDirectory(industryType model.IndustryType) error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if !model.IsValidIndustryType(industryType) {
		return fmt.Errorf("invalid industry type: %s", industryType)
	}

	ragDir := fs.paths.GetRAGPath(industryType)
	return fs.clearDirectory(ragDir)
}

// ResetAllData removes all data and reinitializes directories (switch company).
// This includes company data, documents, parsed documents, and all RAG chunks.
func (fs *FileStore) ResetAllData() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	log.Printf("[FileStore] Resetting all data...")

	// Remove company.json
	companyPath := fs.paths.CompanyFilePath()
	if err := os.Remove(companyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove company file: %w", err)
	}

	// Clear documents list
	docsPath := fs.paths.DocumentsFilePath()
	if err := os.Remove(docsPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove documents list: %w", err)
	}

	// Clear all uploaded documents
	if err := fs.clearDirectory(fs.paths.DocumentsDir); err != nil {
		return fmt.Errorf("failed to clear documents directory: %w", err)
	}

	// Clear all parsed documents
	if err := fs.clearDirectory(fs.paths.ParsedDir); err != nil {
		return fmt.Errorf("failed to clear parsed directory: %w", err)
	}

	// Clear all RAG directories
	for _, ragPath := range fs.paths.GetAllRAGPaths() {
		if err := fs.clearDirectory(ragPath); err != nil {
			log.Printf("[FileStore] Warning: failed to clear RAG directory %s: %v", ragPath, err)
		}
	}

	log.Printf("[FileStore] All data reset successfully")
	return nil
}

// ResetDocuments removes all documents but keeps company data.
// This includes documents, parsed documents, and RAG chunks.
func (fs *FileStore) ResetDocuments() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	log.Printf("[FileStore] Resetting documents...")

	// Clear documents list
	docsPath := fs.paths.DocumentsFilePath()
	if err := os.Remove(docsPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove documents list: %w", err)
	}

	// Clear all uploaded documents
	if err := fs.clearDirectory(fs.paths.DocumentsDir); err != nil {
		return fmt.Errorf("failed to clear documents directory: %w", err)
	}

	// Clear all parsed documents
	if err := fs.clearDirectory(fs.paths.ParsedDir); err != nil {
		return fmt.Errorf("failed to clear parsed directory: %w", err)
	}

	// Clear all RAG directories
	for _, ragPath := range fs.paths.GetAllRAGPaths() {
		if err := fs.clearDirectory(ragPath); err != nil {
			log.Printf("[FileStore] Warning: failed to clear RAG directory %s: %v", ragPath, err)
		}
	}

	log.Printf("[FileStore] Documents reset successfully")
	return nil
}

// clearDirectory removes all files in a directory but keeps the directory
func (fs *FileStore) clearDirectory(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("failed to remove %s: %w", path, err)
		}
	}

	return nil
}

// writeJSON writes data as JSON to a file (non-atomic, for internal use).
func (fs *FileStore) writeJSON(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

// writeJSONAtomic writes data as JSON to a file atomically.
// It writes to a temporary file first, then renames it to the target path.
// This prevents data corruption if the write is interrupted.
func (fs *FileStore) writeJSONAtomic(path string, data interface{}) error {
	// Create temp file in the same directory as target
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, ".tmp_*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on error
	success := false
	defer func() {
		if !success {
			os.Remove(tmpPath)
		}
	}()

	// Write JSON to temp file
	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	// Sync to ensure data is written to disk
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return fmt.Errorf("failed to sync file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	success = true
	return nil
}

// readJSON reads JSON data from a file with size limits.
func (fs *FileStore) readJSON(path string, data interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Check file size before reading
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	if info.Size() > MaxJSONSize {
		return fmt.Errorf("file too large: maximum size is %d bytes, got %d", MaxJSONSize, info.Size())
	}

	// Use limited reader for additional safety
	limitedReader := io.LimitReader(file, MaxJSONSize)
	if err := json.NewDecoder(limitedReader).Decode(data); err != nil {
		return fmt.Errorf("failed to decode JSON: %w", err)
	}

	return nil
}

// ============================================================================
// FILE VALIDATION UTILITIES
// ============================================================================

// ValidateUploadedFile performs security checks on uploaded file content.
// Returns an error if the file appears to be malicious.
func ValidateUploadedFile(filename string, content []byte) error {
	// Check for null bytes (potential security issue)
	for _, b := range content[:min(1024, len(content))] {
		if b == 0 {
			return fmt.Errorf("file contains null bytes")
		}
	}

	// Validate filename characters
	if SanitizeFilename(filename) != filename {
		log.Printf("[FileStore] Warning: filename was sanitized from %q", filename)
	}

	return nil
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
