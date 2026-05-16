package model

import "time"

// DocType represents the type of financial document
type DocType string

const (
	DocTypePnL          DocType = "P&L"
	DocTypeBalanceSheet DocType = "BalanceSheet"
	DocTypeCashFlow     DocType = "CashFlow"
	DocTypeUnknown      DocType = "Unknown"
)

// Document represents an uploaded financial document
type Document struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	DocType     DocType   `json:"doc_type"`
	PeriodStart string    `json:"period_start"` // YYYY-MM-DD
	PeriodEnd   string    `json:"period_end"`   // YYYY-MM-DD
	FilePath    string    `json:"file_path"`    // Path to raw file
	ParsedPath  string    `json:"parsed_path"`  // Path to parsed JSON
	UploadedAt  time.Time `json:"uploaded_at"`
	FileSize    int64     `json:"file_size"`
	MimeType    string    `json:"mime_type"`
}

// DocumentList holds all documents metadata
type DocumentList struct {
	Documents []Document `json:"documents"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ParsedDocument represents the parsed data from a financial document
type ParsedDocument struct {
	DocumentID string                 `json:"document_id"`
	DocType    DocType                `json:"doc_type"`
	Filename   string                 `json:"filename"` // Original filename
	Period     Period                 `json:"period"`
	Data       map[string]float64     `json:"data"`       // Key-value pairs of financial metrics
	RawText    string                 `json:"raw_text"`   // Full extracted text for RAG
	Chunks     []TextChunk            `json:"chunks"`     // Broken into searchable chunks
	PageCount  int                    `json:"page_count"` // Number of pages (for PDF)
	Metadata   map[string]interface{} `json:"metadata"`   // Additional metadata
	ParsedAt   time.Time              `json:"parsed_at"`
}

// TextChunk represents a chunk of text from a document
type TextChunk struct {
	Text   string `json:"text"`
	Page   int    `json:"page,omitempty"`   // Page number (1-indexed, for PDFs)
	Sheet  string `json:"sheet,omitempty"`  // Sheet name (for XLSX)
	Source string `json:"source,omitempty"` // Source identifier
}

// Period represents a financial period
type Period struct {
	Start string `json:"start"`
	End   string `json:"end"`
}
