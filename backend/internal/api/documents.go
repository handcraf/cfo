package api

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cfo/backend/internal/config"
	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/service"
	"github.com/cfo/backend/internal/storage"
)

// DocumentsHandler handles document endpoints
type DocumentsHandler struct {
	store  *storage.FileStore
	parser *service.DocumentParser
}

// NewDocumentsHandler creates a new DocumentsHandler
func NewDocumentsHandler(store *storage.FileStore, cfg *config.Config) *DocumentsHandler {
	return &DocumentsHandler{
		store:  store,
		parser: service.NewDocumentParser(),
	}
}

// Upload handles POST /documents/upload
func (h *DocumentsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 32MB)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, "Failed to parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".pdf" && ext != ".csv" && ext != ".xlsx" {
		writeError(w, "Invalid file type. Accepted: PDF, CSV, XLSX", http.StatusBadRequest)
		return
	}

	// Get document type from form
	docTypeStr := r.FormValue("doc_type")
	docType := model.DocType(docTypeStr)
	if docType == "" {
		docType = model.DocTypeUnknown
	}

	// Get period from form
	periodStart := r.FormValue("period_start")
	periodEnd := r.FormValue("period_end")

	// Generate document ID
	docID := fmt.Sprintf("doc_%d", time.Now().UnixNano())

	// Save the raw file
	filePath, err := h.store.SaveDocument(header.Filename, file)
	if err != nil {
		writeError(w, "Failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create document metadata
	doc := model.Document{
		ID:          docID,
		Filename:    filepath.Base(filePath),
		DocType:     docType,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		FilePath:    filePath,
		ParsedPath:  h.store.GetPaths().ParsedFilePath(docID),
		UploadedAt:  time.Now(),
		FileSize:    header.Size,
		MimeType:    header.Header.Get("Content-Type"),
	}

	// Parse the document
	savedFile, err := os.Open(filePath)
	if err == nil {
		defer savedFile.Close()
		parsed, parseErr := h.parser.Parse(doc, savedFile)
		if parseErr == nil && parsed != nil {
			h.store.SaveParsedDocument(parsed)
		}
		// TODO: Log parsing errors but don't fail the upload
	}

	// Update document list
	docList, err := h.store.LoadDocumentList()
	if err != nil {
		writeError(w, "Failed to load document list", http.StatusInternalServerError)
		return
	}

	docList.Documents = append(docList.Documents, doc)
	if err := h.store.SaveDocumentList(docList); err != nil {
		writeError(w, "Failed to save document list", http.StatusInternalServerError)
		return
	}

	writeJSON(w, doc)
}

// List handles GET /documents
func (h *DocumentsHandler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	docList, err := h.store.LoadDocumentList()
	if err != nil {
		writeError(w, "Failed to load documents", http.StatusInternalServerError)
		return
	}

	writeJSON(w, docList)
}

// ResetDocuments handles DELETE /documents/reset - removes all documents
func (h *DocumentsHandler) ResetDocuments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := h.store.ResetDocuments(); err != nil {
		writeError(w, "Failed to reset documents: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "All documents have been deleted.",
	})
}
