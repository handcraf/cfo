package service

import (
	"strings"
	"testing"

	"github.com/cfo/backend/internal/model"
)

func TestDocumentParser_ParseCSV_BasicFinancials(t *testing.T) {
	parser := NewDocumentParser()

	csvContent := `Metric,Value
Revenue,1000000
Expenses,800000
Net Income,200000
Cash,500000
Total Assets,2000000
Total Liabilities,1000000
Equity,1000000`

	doc := model.Document{
		ID:          "test_doc_1",
		Filename:    "test.csv",
		DocType:     model.DocTypePnL,
		PeriodStart: "2024-01-01",
		PeriodEnd:   "2024-12-31",
	}

	reader := strings.NewReader(csvContent)
	parsed, err := parser.Parse(doc, reader)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed == nil {
		t.Fatal("Parsed document is nil")
	}

	// Verify parsed data
	tests := []struct {
		key      string
		expected float64
	}{
		{"revenue", 1000000},
		{"expenses", 800000},
		{"net_income", 200000},
		{"cash", 500000},
		{"total_assets", 2000000},
		{"total_liabilities", 1000000},
		{"equity", 1000000},
	}

	for _, tt := range tests {
		if val, ok := parsed.Data[tt.key]; !ok {
			t.Errorf("Missing key %s in parsed data", tt.key)
		} else if val != tt.expected {
			t.Errorf("Key %s: expected %f, got %f", tt.key, tt.expected, val)
		}
	}
}

func TestDocumentParser_ParseCSV_WithCurrencyFormatting(t *testing.T) {
	parser := NewDocumentParser()

	csvContent := `Metric,Value
Revenue,"$1,500,000"
Expenses,"$1,200,000"
Cash,"$300,000"`

	doc := model.Document{
		ID:       "test_doc_2",
		Filename: "formatted.csv",
		DocType:  model.DocTypePnL,
	}

	reader := strings.NewReader(csvContent)
	parsed, err := parser.Parse(doc, reader)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if val := parsed.Data["revenue"]; val != 1500000 {
		t.Errorf("Revenue: expected 1500000, got %f", val)
	}

	if val := parsed.Data["expenses"]; val != 1200000 {
		t.Errorf("Expenses: expected 1200000, got %f", val)
	}

	if val := parsed.Data["cash"]; val != 300000 {
		t.Errorf("Cash: expected 300000, got %f", val)
	}
}

func TestDocumentParser_ParseCSV_WithNegativeValues(t *testing.T) {
	parser := NewDocumentParser()

	csvContent := `Metric,Value
Net Income,(50000)
Cash,100000`

	doc := model.Document{
		ID:       "test_doc_3",
		Filename: "negative.csv",
		DocType:  model.DocTypePnL,
	}

	reader := strings.NewReader(csvContent)
	parsed, err := parser.Parse(doc, reader)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if val := parsed.Data["net_income"]; val != -50000 {
		t.Errorf("Net Income: expected -50000, got %f", val)
	}
}

func TestDocumentParser_ParseCSV_VariousLabelFormats(t *testing.T) {
	parser := NewDocumentParser()

	csvContent := `Metric,Value
Total Revenue,1000000
Total Expenses,800000
Net Profit,200000
Cash and Cash Equivalents,500000
Stockholders Equity,1000000
Cost of Goods Sold,400000
EBITDA,300000`

	doc := model.Document{
		ID:       "test_doc_4",
		Filename: "variations.csv",
		DocType:  model.DocTypePnL,
	}

	reader := strings.NewReader(csvContent)
	parsed, err := parser.Parse(doc, reader)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Check that various label formats are normalized
	expectedMappings := map[string]float64{
		"revenue":    1000000,
		"expenses":   800000,
		"net_income": 200000,
		"cash":       500000,
		"equity":     1000000,
		"cogs":       400000,
		"ebitda":     300000,
	}

	for key, expected := range expectedMappings {
		if val, ok := parsed.Data[key]; !ok {
			t.Errorf("Missing normalized key %s", key)
		} else if val != expected {
			t.Errorf("Key %s: expected %f, got %f", key, expected, val)
		}
	}
}

func TestDocumentParser_ParseCSV_EmptyFile(t *testing.T) {
	parser := NewDocumentParser()

	doc := model.Document{
		ID:       "test_doc_empty",
		Filename: "empty.csv",
		DocType:  model.DocTypePnL,
	}

	reader := strings.NewReader("")
	parsed, err := parser.Parse(doc, reader)

	if err != nil {
		t.Fatalf("Parse should not fail on empty file: %v", err)
	}

	if len(parsed.Data) != 0 {
		t.Errorf("Expected empty data map, got %d entries", len(parsed.Data))
	}
}

func TestDocumentParser_ParseCSV_MalformedRows(t *testing.T) {
	parser := NewDocumentParser()

	csvContent := `Metric,Value
Revenue,1000000
This is a malformed row without comma
Expenses,800000
Another bad row
Cash,500000`

	doc := model.Document{
		ID:       "test_doc_malformed",
		Filename: "malformed.csv",
		DocType:  model.DocTypePnL,
	}

	reader := strings.NewReader(csvContent)
	parsed, err := parser.Parse(doc, reader)

	if err != nil {
		t.Fatalf("Parse should handle malformed rows gracefully: %v", err)
	}

	// Should still parse the valid rows
	if val := parsed.Data["revenue"]; val != 1000000 {
		t.Errorf("Revenue should be parsed: expected 1000000, got %f", val)
	}

	if val := parsed.Data["expenses"]; val != 800000 {
		t.Errorf("Expenses should be parsed: expected 800000, got %f", val)
	}
}

func TestDocumentParser_ParseCSV_RawTextExtraction(t *testing.T) {
	parser := NewDocumentParser()

	csvContent := `Metric,Value
Revenue,1000000
Expenses,800000`

	doc := model.Document{
		ID:       "test_doc_rawtext",
		Filename: "rawtext.csv",
		DocType:  model.DocTypePnL,
	}

	reader := strings.NewReader(csvContent)
	parsed, err := parser.Parse(doc, reader)

	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.RawText == "" {
		t.Error("RawText should not be empty")
	}

	if !strings.Contains(parsed.RawText, "Revenue") {
		t.Error("RawText should contain 'Revenue'")
	}
}

func TestDocumentParser_ParsePDF(t *testing.T) {
	parser := NewDocumentParser()

	doc := model.Document{
		ID:       "test_doc_pdf",
		Filename: "test.pdf",
		DocType:  model.DocTypePnL,
	}

	reader := strings.NewReader("PDF binary content placeholder")
	parsed, err := parser.Parse(doc, reader)

	if err != nil {
		t.Fatalf("Parse should not fail for PDF: %v", err)
	}

	// PDF parsing should have some status (success or failed depending on pdftotext availability)
	status, hasStatus := parsed.Metadata["parse_status"]
	if !hasStatus {
		t.Log("PDF parse_status not set - parser handled gracefully")
	} else {
		t.Logf("PDF parse_status: %v", status)
	}

	// Should have RawText (even if it's an error message)
	if parsed.RawText == "" {
		t.Error("PDF should have some RawText content")
	}
}

func TestDocumentParser_ParseXLSX(t *testing.T) {
	parser := NewDocumentParser()

	doc := model.Document{
		ID:       "test_doc_xlsx",
		Filename: "test.xlsx",
		DocType:  model.DocTypePnL,
	}

	// Invalid XLSX content - parser should handle gracefully
	reader := strings.NewReader("XLSX binary content placeholder")
	parsed, err := parser.Parse(doc, reader)

	if err != nil {
		t.Fatalf("Parse should not fail for XLSX: %v", err)
	}

	// Should have some parse status or error info
	if parseErr, hasErr := parsed.Metadata["parse_error"]; hasErr {
		t.Logf("XLSX parse_error (expected for invalid content): %v", parseErr)
	}

	// Should have RawText (even if it's an error message)
	if parsed.RawText == "" {
		t.Error("XLSX should have some RawText content")
	}
}

func TestCleanNumericString(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"$1,000,000", "1000000"},
		{"1000000", "1000000"},
		{"(50000)", "-50000"},
		{"$100.50", "100.50"},
		{"  $500  ", "500"},
	}

	for _, tt := range tests {
		result := cleanNumericString(tt.input)
		if result != tt.expected {
			t.Errorf("cleanNumericString(%s): expected %s, got %s", tt.input, tt.expected, result)
		}
	}
}

func TestNormalizeFinancialLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Revenue", "revenue"},
		{"Total Revenue", "revenue"},
		{"Net Sales", "revenue"},
		{"Expenses", "expenses"},
		{"Total Expenses", "expenses"},
		{"Net Income", "net_income"},
		{"Net Profit", "net_income"},
		{"Cash", "cash"},
		{"Cash and Cash Equivalents", "cash"},
		{"Total Assets", "total_assets"},
		{"Total Liabilities", "total_liabilities"},
		{"Equity", "equity"},
		{"Stockholders Equity", "equity"},
		{"Unknown Label", ""},
		{"Random Text", ""},
	}

	for _, tt := range tests {
		result := normalizeFinancialLabel(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeFinancialLabel(%s): expected %s, got %s", tt.input, tt.expected, result)
		}
	}
}
