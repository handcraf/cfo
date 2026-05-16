package sqlstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeFile is a small helper that creates the parent dir and writes data.
func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestMigrateFromJSON_HappyPath(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	s := openTempStore(t)

	// Company file.
	writeFile(t, filepath.Join(dataDir, "state", "company.json"), `{
		"name": "Acme",
		"industry": "SaaS",
		"industry_type": "generic",
		"fiscal_year_end": "12-31",
		"currency": "USD",
		"setup_completed": true
	}`)

	// Documents index.
	writeFile(t, filepath.Join(dataDir, "state", "documents.json"), `{
		"documents": [
			{
				"id": "doc_q1",
				"filename": "q1.xlsx",
				"doc_type": "P&L",
				"period_start": "2024-01-01",
				"period_end": "2024-03-31",
				"file_path": "/tmp/q1.xlsx",
				"parsed_path": "/tmp/q1.json",
				"uploaded_at": "2024-04-01T00:00:00Z",
				"file_size": 10,
				"mime_type": "x"
			}
		]
	}`)

	// Parsed data for doc_q1.
	writeFile(t, filepath.Join(dataDir, "parsed", "doc_q1.json"), `{
		"document_id": "doc_q1",
		"data": {"revenue": 1234567.89, "cash": 500000}
	}`)

	res, err := s.MigrateFromJSON(ctx, dataDir)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !res.CompanyImported {
		t.Errorf("company not imported")
	}
	if res.DocumentsImported != 1 {
		t.Errorf("documents imported = %d, want 1", res.DocumentsImported)
	}
	if res.LineItemsImported != 2 {
		t.Errorf("line items imported = %d, want 2", res.LineItemsImported)
	}

	// Verify queryable state.
	if c, _ := s.GetCompany(ctx); c == nil || c.Name != "Acme" {
		t.Errorf("company not readable after migrate: %+v", c)
	}
	hits, _ := s.QueryMetric(ctx, "revenue", "2024-01-01", "2024-03-31")
	if len(hits) != 1 {
		t.Errorf("expected 1 revenue row, got %d", len(hits))
	}

	// Idempotency: run again with no changes, no errors.
	if _, err := s.MigrateFromJSON(ctx, dataDir); err != nil {
		t.Errorf("re-run failed: %v", err)
	}
}

func TestMigrateFromJSON_MissingFiles(t *testing.T) {
	ctx := context.Background()
	s := openTempStore(t)
	res, err := s.MigrateFromJSON(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("empty migrate should not error: %v", err)
	}
	if res.CompanyImported || res.DocumentsImported != 0 || res.LineItemsImported != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}
}
