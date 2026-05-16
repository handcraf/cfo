package service

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cfo/backend/internal/model"
	"github.com/xuri/excelize/v2"
)

// Constants for context limits
const (
	MaxChunkSize     = 500   // Max characters per chunk
	MaxTotalTextSize = 24000 // ~6K tokens for llama3 (leaving room for prompt)
	MaxChunksPerDoc  = 50    // Max chunks per document
)

// DocumentParser handles parsing of uploaded financial documents
type DocumentParser struct{}

// NewDocumentParser creates a new DocumentParser
func NewDocumentParser() *DocumentParser {
	return &DocumentParser{}
}

// Parse parses a document and extracts financial data
func (p *DocumentParser) Parse(doc model.Document, reader io.Reader) (*model.ParsedDocument, error) {
	parsed := &model.ParsedDocument{
		DocumentID: doc.ID,
		DocType:    doc.DocType,
		Filename:   doc.Filename,
		Period: model.Period{
			Start: doc.PeriodStart,
			End:   doc.PeriodEnd,
		},
		Data:     make(map[string]float64),
		Chunks:   []model.TextChunk{},
		Metadata: make(map[string]interface{}),
		ParsedAt: time.Now(),
	}

	// Determine parser based on file extension
	filename := strings.ToLower(doc.Filename)

	switch {
	case strings.HasSuffix(filename, ".csv"):
		return p.parseCSV(parsed, reader)
	case strings.HasSuffix(filename, ".xlsx"), strings.HasSuffix(filename, ".xls"):
		return p.parseXLSX(parsed, reader, doc.FilePath)
	case strings.HasSuffix(filename, ".pdf"):
		return p.parsePDF(parsed, reader, doc.FilePath)
	default:
		return nil, fmt.Errorf("unsupported file format: %s", filename)
	}
}

// parseCSV parses a CSV file for financial data
func (p *DocumentParser) parseCSV(parsed *model.ParsedDocument, reader io.Reader) (*model.ParsedDocument, error) {
	csvReader := csv.NewReader(reader)

	var rawTextBuilder strings.Builder
	var allRows []string

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // Skip malformed rows
		}

		// Build raw text for RAG
		line := strings.Join(record, " | ")
		rawTextBuilder.WriteString(line + "\n")
		allRows = append(allRows, line)

		// Try to extract key-value pairs
		if len(record) >= 2 {
			label := strings.TrimSpace(record[0])
			valueStr := strings.TrimSpace(record[1])

			valueStr = cleanNumericString(valueStr)

			if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
				normalizedLabel := normalizeFinancialLabel(label)
				if normalizedLabel != "" {
					parsed.Data[normalizedLabel] = value
				}
			}
		}
	}

	parsed.RawText = rawTextBuilder.String()
	parsed.PageCount = 1

	// Create chunks from rows
	parsed.Chunks = p.createChunksFromLines(allRows, "csv", parsed.Filename)

	return parsed, nil
}

// parseXLSX parses an XLSX file using excelize library
func (p *DocumentParser) parseXLSX(parsed *model.ParsedDocument, reader io.Reader, filePath string) (*model.ParsedDocument, error) {
	// Read file content
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, reader); err != nil {
		return nil, fmt.Errorf("failed to read XLSX: %w", err)
	}

	// Open with excelize
	f, err := excelize.OpenReader(&buf)
	if err != nil {
		// If excelize fails, try reading from file path directly
		if filePath != "" {
			f, err = excelize.OpenFile(filePath)
			if err != nil {
				parsed.RawText = "Failed to parse XLSX file"
				parsed.Metadata["parse_error"] = err.Error()
				return parsed, nil
			}
		} else {
			parsed.RawText = "Failed to parse XLSX file"
			parsed.Metadata["parse_error"] = err.Error()
			return parsed, nil
		}
	}
	defer f.Close()

	var rawTextBuilder strings.Builder
	var allLines []string

	// Get all sheet names
	sheets := f.GetSheetList()
	parsed.Metadata["sheets"] = sheets

	for _, sheet := range sheets {
		rawTextBuilder.WriteString(fmt.Sprintf("\n=== Sheet: %s ===\n", sheet))

		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}

		for _, row := range rows {
			if len(row) == 0 {
				continue
			}

			line := strings.Join(row, " | ")
			rawTextBuilder.WriteString(line + "\n")
			allLines = append(allLines, line)

			// Try to extract key-value pairs from first two columns
			if len(row) >= 2 {
				label := strings.TrimSpace(row[0])
				valueStr := strings.TrimSpace(row[1])

				valueStr = cleanNumericString(valueStr)

				if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
					normalizedLabel := normalizeFinancialLabel(label)
					if normalizedLabel != "" {
						parsed.Data[normalizedLabel] = value
					}
				}
			}
		}

		// Create chunks for this sheet
		sheetChunks := p.createChunksFromLines(allLines, sheet, parsed.Filename)
		parsed.Chunks = append(parsed.Chunks, sheetChunks...)
		allLines = []string{} // Reset for next sheet
	}

	parsed.RawText = rawTextBuilder.String()
	parsed.PageCount = len(sheets)
	parsed.Metadata["parse_status"] = "success"

	return parsed, nil
}

// parsePDF parses a PDF file using pdftotext CLI
func (p *DocumentParser) parsePDF(parsed *model.ParsedDocument, reader io.Reader, filePath string) (*model.ParsedDocument, error) {
	// First, save the reader content to a temp file if filePath is not available
	var pdfPath string

	if filePath != "" && fileExists(filePath) {
		pdfPath = filePath
	} else {
		// Create temp file
		tmpFile, err := os.CreateTemp("", "pdf-*.pdf")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		if _, err := io.Copy(tmpFile, reader); err != nil {
			return nil, fmt.Errorf("failed to write temp file: %w", err)
		}
		pdfPath = tmpFile.Name()
	}

	// Try pdftotext first (poppler-utils)
	text, pageCount, err := extractTextWithPdftotext(pdfPath)
	if err != nil {
		// Fallback: try to read as text (for text-based PDFs)
		text, err = extractTextFallback(pdfPath)
		if err != nil {
			parsed.RawText = "PDF parsing failed - please ensure poppler-utils is installed"
			parsed.Metadata["parse_error"] = err.Error()
			parsed.Metadata["parse_status"] = "failed"
			return parsed, nil
		}
		pageCount = 1
	}

	// Truncate if too large
	if len(text) > MaxTotalTextSize {
		text = text[:MaxTotalTextSize] + "\n... [Content truncated for LLM context limit]"
		parsed.Metadata["truncated"] = true
	}

	parsed.RawText = text
	parsed.PageCount = pageCount
	parsed.Metadata["parse_status"] = "success"

	// Create chunks from the text (by pages or paragraphs)
	parsed.Chunks = p.createChunksFromText(text, parsed.Filename)

	// Try to extract financial metrics from the text
	p.extractMetricsFromText(parsed, text)

	return parsed, nil
}

// extractTextWithPdftotext uses poppler's pdftotext to extract text
func extractTextWithPdftotext(pdfPath string) (string, int, error) {
	// Check if pdftotext is available
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", 0, fmt.Errorf("pdftotext not found: %w", err)
	}

	// Get page count using pdfinfo
	pageCount := 1
	if _, err := exec.LookPath("pdfinfo"); err == nil {
		cmd := exec.Command("pdfinfo", pdfPath)
		output, err := cmd.Output()
		if err == nil {
			for _, line := range strings.Split(string(output), "\n") {
				if strings.HasPrefix(line, "Pages:") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						if n, err := strconv.Atoi(parts[1]); err == nil {
							pageCount = n
						}
					}
				}
			}
		}
	}

	// Extract text with layout preservation
	cmd := exec.Command("pdftotext", "-layout", pdfPath, "-")
	output, err := cmd.Output()
	if err != nil {
		// Try without layout flag
		cmd = exec.Command("pdftotext", pdfPath, "-")
		output, err = cmd.Output()
		if err != nil {
			return "", 0, fmt.Errorf("pdftotext failed: %w", err)
		}
	}

	return string(output), pageCount, nil
}

// extractTextFallback tries to read text from a PDF file directly
func extractTextFallback(pdfPath string) (string, error) {
	file, err := os.Open(pdfPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	var content strings.Builder
	scanner := bufio.NewScanner(file)

	// Increase buffer size for large files
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		// Filter out binary content
		if isPrintable(line) {
			content.WriteString(line + "\n")
		}
	}

	if content.Len() == 0 {
		return "", fmt.Errorf("no readable text found in PDF")
	}

	return content.String(), nil
}

// isPrintable checks if a string contains mostly printable characters
func isPrintable(s string) bool {
	if len(s) == 0 {
		return false
	}
	printable := 0
	for _, r := range s {
		if r >= 32 && r < 127 {
			printable++
		}
	}
	return float64(printable)/float64(len(s)) > 0.8
}

// createChunksFromLines creates text chunks from lines
func (p *DocumentParser) createChunksFromLines(lines []string, source, filename string) []model.TextChunk {
	var chunks []model.TextChunk
	var currentChunk strings.Builder
	chunkNum := 0

	for _, line := range lines {
		if currentChunk.Len()+len(line) > MaxChunkSize {
			if currentChunk.Len() > 0 {
				chunks = append(chunks, model.TextChunk{
					Text:   currentChunk.String(),
					Source: fmt.Sprintf("%s (chunk %d)", filename, chunkNum+1),
					Sheet:  source,
				})
				chunkNum++
				currentChunk.Reset()
			}
		}
		currentChunk.WriteString(line + "\n")

		if len(chunks) >= MaxChunksPerDoc {
			break
		}
	}

	// Add remaining content
	if currentChunk.Len() > 0 && len(chunks) < MaxChunksPerDoc {
		chunks = append(chunks, model.TextChunk{
			Text:   currentChunk.String(),
			Source: fmt.Sprintf("%s (chunk %d)", filename, chunkNum+1),
			Sheet:  source,
		})
	}

	return chunks
}

// createChunksFromText creates chunks from a text block
func (p *DocumentParser) createChunksFromText(text, filename string) []model.TextChunk {
	lines := strings.Split(text, "\n")
	return p.createChunksFromLines(lines, "page", filename)
}

// extractMetricsFromText tries to extract financial metrics from text
func (p *DocumentParser) extractMetricsFromText(parsed *model.ParsedDocument, text string) {
	lines := strings.Split(text, "\n")

	for _, line := range lines {
		// Look for patterns like "Revenue: $1,000,000" or "Revenue $1,000,000"
		parts := strings.FieldsFunc(line, func(r rune) bool {
			return r == ':' || r == '\t'
		})

		if len(parts) >= 2 {
			label := strings.TrimSpace(parts[0])
			valueStr := strings.TrimSpace(parts[1])

			// Extract first number-like value
			valueStr = extractFirstNumber(valueStr)
			valueStr = cleanNumericString(valueStr)

			if value, err := strconv.ParseFloat(valueStr, 64); err == nil {
				normalizedLabel := normalizeFinancialLabel(label)
				if normalizedLabel != "" {
					parsed.Data[normalizedLabel] = value
				}
			}
		}
	}
}

// extractFirstNumber extracts the first number from a string
func extractFirstNumber(s string) string {
	var result strings.Builder
	started := false

	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' || r == '-' || r == '(' || r == ')' {
			result.WriteRune(r)
			started = true
		} else if r == '$' {
			continue
		} else if started {
			break
		}
	}

	return result.String()
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// cleanNumericString removes common formatting from numeric strings
func cleanNumericString(s string) string {
	s = strings.ReplaceAll(s, "$", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, "(", "-")
	s = strings.ReplaceAll(s, ")", "")
	s = strings.TrimSpace(s)
	return s
}

// normalizeFinancialLabel maps common financial terms to standard keys
func normalizeFinancialLabel(label string) string {
	label = strings.ToLower(strings.TrimSpace(label))

	mappings := map[string][]string{
		"revenue": {
			"revenue", "total revenue", "net revenue", "sales", "total sales",
			"income", "net sales", "gross revenue",
		},
		"expenses": {
			"expenses", "total expenses", "operating expenses", "opex",
			"total operating expenses", "cost", "total cost",
		},
		"net_income": {
			"net income", "net profit", "profit", "net earnings",
			"earnings", "bottom line", "net income (loss)",
		},
		"cash": {
			"cash", "cash and cash equivalents", "cash & equivalents",
			"total cash", "cash balance", "cash on hand",
		},
		"total_assets": {
			"total assets", "assets", "total asset",
		},
		"total_liabilities": {
			"total liabilities", "liabilities", "total liability",
			"total liab", "debt",
		},
		"equity": {
			"equity", "total equity", "shareholders equity",
			"stockholders equity", "net worth", "total stockholders equity",
		},
		"accounts_receivable": {
			"accounts receivable", "receivables", "ar", "a/r",
		},
		"accounts_payable": {
			"accounts payable", "payables", "ap", "a/p",
		},
		"cogs": {
			"cogs", "cost of goods sold", "cost of sales", "cost of revenue",
		},
		"gross_profit": {
			"gross profit", "gross margin", "gross income",
		},
		"operating_income": {
			"operating income", "operating profit", "ebit", "op income",
		},
		"ebitda": {
			"ebitda", "adjusted ebitda",
		},
	}

	for normalized, variations := range mappings {
		for _, v := range variations {
			if label == v {
				return normalized
			}
		}
	}

	return ""
}

// GetFileExtension returns the file extension
func GetFileExtension(filename string) string {
	return strings.ToLower(filepath.Ext(filename))
}
