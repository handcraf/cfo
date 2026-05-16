package storage

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cfo/backend/internal/model"
)

// File and directory constants for security limits
const (
	// MaxFilenameLength is the maximum allowed filename length
	MaxFilenameLength = 255
	// MaxDocumentIDLength is the maximum allowed document ID length
	MaxDocumentIDLength = 128
)

// Paths holds all the directory paths for data storage
type Paths struct {
	DataDir      string
	DocumentsDir string
	ParsedDir    string
	StateDir     string
	RAGDir       string // Root directory for RAG chunked data
}

// NewPaths creates a new Paths instance from the base data directory
func NewPaths(dataDir string) *Paths {
	// Clean the data directory path to prevent issues
	cleanDataDir := filepath.Clean(dataDir)
	return &Paths{
		DataDir:      cleanDataDir,
		DocumentsDir: filepath.Join(cleanDataDir, "documents"),
		ParsedDir:    filepath.Join(cleanDataDir, "parsed"),
		StateDir:     filepath.Join(cleanDataDir, "state"),
		RAGDir:       filepath.Join(cleanDataDir, "rag"),
	}
}

// SanitizeFilename cleans a filename to prevent path traversal attacks.
// It removes directory components and dangerous characters.
func SanitizeFilename(filename string) string {
	// Handle empty input first
	if filename == "" {
		return "unnamed_file"
	}

	// Get only the base name (removes any directory components)
	filename = filepath.Base(filename)

	// Remove null bytes and control characters
	filename = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, filename)

	// Remove path separators that might have been encoded
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")
	// Replace consecutive dots (path traversal) with single underscore
	for strings.Contains(filename, "..") {
		filename = strings.ReplaceAll(filename, "..", "_")
	}

	// Truncate if too long
	if len(filename) > MaxFilenameLength {
		ext := filepath.Ext(filename)
		base := filename[:len(filename)-len(ext)]
		maxBase := MaxFilenameLength - len(ext)
		if maxBase > 0 && len(base) > maxBase {
			base = base[:maxBase]
		}
		filename = base + ext
	}

	// Default filename if empty or just dots after sanitization
	if filename == "" || filename == "." || filename == "_" {
		filename = "unnamed_file"
	}

	return filename
}

// SanitizeDocumentID cleans a document ID to prevent path traversal attacks.
// Document IDs should only contain alphanumeric characters, underscores, and hyphens.
func SanitizeDocumentID(documentID string) string {
	// Handle empty input first
	if documentID == "" {
		return "unnamed_document"
	}

	// Remove any path separators
	documentID = filepath.Base(documentID)

	// Only allow alphanumeric, underscore, hyphen
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	documentID = reg.ReplaceAllString(documentID, "_")

	// Remove leading/trailing underscores that may have been created
	documentID = strings.Trim(documentID, "_")

	// Truncate if too long
	if len(documentID) > MaxDocumentIDLength {
		documentID = documentID[:MaxDocumentIDLength]
	}

	// Default ID if empty after sanitization
	if documentID == "" {
		documentID = "unnamed_document"
	}

	return documentID
}

// CompanyFilePath returns the path to the company.json file
func (p *Paths) CompanyFilePath() string {
	return filepath.Join(p.StateDir, "company.json")
}

// DocumentsFilePath returns the path to the documents.json file
func (p *Paths) DocumentsFilePath() string {
	return filepath.Join(p.StateDir, "documents.json")
}

// DocumentFilePath returns the path to store an uploaded document.
// The filename is sanitized to prevent path traversal attacks.
func (p *Paths) DocumentFilePath(filename string) string {
	safeFilename := SanitizeFilename(filename)
	return filepath.Join(p.DocumentsDir, safeFilename)
}

// ParsedFilePath returns the path to store parsed document data.
// The document ID is sanitized to prevent path traversal attacks.
func (p *Paths) ParsedFilePath(documentID string) string {
	safeDocID := SanitizeDocumentID(documentID)
	return filepath.Join(p.ParsedDir, safeDocID+".json")
}

// IsPathWithinDirectory checks if a path is safely within the expected directory.
// This prevents path traversal attacks by ensuring the resolved path
// stays within the base directory.
func IsPathWithinDirectory(basePath, targetPath string) bool {
	// Clean both paths
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return false
	}

	// Ensure target is within base
	relPath, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}

	// Check if the relative path goes outside the base directory
	return !strings.HasPrefix(relPath, "..") && !filepath.IsAbs(relPath)
}

// GetRAGPath returns the path to the RAG directory for a specific industry type.
// Each industry has its own subdirectory for chunked extracted data.
//
// Directory structure:
//
//	backend/data/rag/
//	├── generic/     - Default financial data chunks
//	├── education/   - Education sector specific chunks
//	├── ecommerce/   - E-commerce specific chunks
//	└── pharma/      - Pharmaceutical specific chunks
//
// The industry type is validated to prevent path traversal attacks.
// Invalid industry types default to "generic".
//
// TODO: Implement document chunking and storage into these directories
// when documents are uploaded and parsed.
func (p *Paths) GetRAGPath(industryType model.IndustryType) string {
	// Default to generic if industry type is empty or invalid
	// This also prevents path traversal via malicious industry type values
	if industryType == "" || !model.IsValidIndustryType(industryType) {
		industryType = model.IndustryGeneric
	}
	return filepath.Join(p.RAGDir, string(industryType))
}

// GetRAGChunkPath returns the path to store a specific chunk file for an industry.
// The chunk files contain JSON-formatted document chunks for RAG retrieval.
//
// Both industry type and document ID are validated/sanitized to prevent
// path traversal attacks.
//
// TODO: Implement actual chunk storage when document parsing is extended
// to support industry-specific chunking strategies.
func (p *Paths) GetRAGChunkPath(industryType model.IndustryType, documentID string) string {
	// Sanitize document ID to prevent path traversal
	safeDocID := SanitizeDocumentID(documentID)
	return filepath.Join(p.GetRAGPath(industryType), safeDocID+"_chunks.json")
}

// GetAllRAGPaths returns paths for all supported industry RAG directories.
// Useful for initialization and cleanup operations.
func (p *Paths) GetAllRAGPaths() []string {
	industryTypes := model.ValidIndustryTypes()
	paths := make([]string, len(industryTypes))
	for i, it := range industryTypes {
		paths[i] = p.GetRAGPath(it)
	}
	return paths
}

// ValidateRAGPath checks if a given path is a valid RAG storage path
// within the expected RAG directory structure.
func (p *Paths) ValidateRAGPath(path string) bool {
	return IsPathWithinDirectory(p.RAGDir, path)
}

// ValidateDocumentPath checks if a given path is a valid document storage path.
func (p *Paths) ValidateDocumentPath(path string) bool {
	return IsPathWithinDirectory(p.DocumentsDir, path)
}

// ValidateParsedPath checks if a given path is a valid parsed document storage path.
func (p *Paths) ValidateParsedPath(path string) bool {
	return IsPathWithinDirectory(p.ParsedDir, path)
}

