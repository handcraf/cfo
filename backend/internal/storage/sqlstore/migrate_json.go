// Package sqlstore — one-shot migration from file-based JSON to SQLite.
//
// This is intentionally idempotent and best-effort:
//   - Re-running MigrateFromJSON is safe: UPSERTs and ON CONFLICT everywhere.
//   - A missing JSON file is not an error — it just means nothing to import.
//   - Parse errors on individual documents are logged, not fatal, so one
//     bad file doesn't block migration of the other 99.
//
// Call this from main() after Open() when SQLStoreEnabled. Do NOT delete
// the JSON files after migration — they are the fallback until we're
// confident the SQL path is stable.
//
// TODO: Add a `--dry-run` CLI flag that counts rows without writing.
// TODO: Add a checksum/version marker so we can detect double-migrations.
package sqlstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MigrateFromJSON imports company, documents, and parsed line items from
// the legacy file-based layout into this SQLite store.
//
// dataDir is the same path the FileStore uses (backend/data by default).
// Layout expected:
//
//	<dataDir>/state/company.json
//	<dataDir>/state/documents.json
//	<dataDir>/parsed/<doc_id>.json
//
// Rows already present in SQLite are upserted, so this is safe to run
// every boot. The migration takes out no locks of its own — callers
// must ensure no concurrent writers (i.e., run before http.ListenAndServe).
func (s *Store) MigrateFromJSON(ctx context.Context, dataDir string) (MigrationResult, error) {
	var result MigrationResult

	if err := s.migrateCompany(ctx, dataDir, &result); err != nil {
		return result, err
	}
	if err := s.migrateDocuments(ctx, dataDir, &result); err != nil {
		return result, err
	}
	if err := s.migrateParsedDocs(ctx, dataDir, &result); err != nil {
		return result, err
	}

	log.Printf("[sqlstore][migrate] done — company=%v docs=%d line_items=%d errors=%d",
		result.CompanyImported, result.DocumentsImported, result.LineItemsImported, len(result.Errors))
	return result, nil
}

// MigrationResult is a tally returned from MigrateFromJSON.
type MigrationResult struct {
	CompanyImported   bool
	DocumentsImported int
	LineItemsImported int
	Errors            []string
}

func (s *Store) migrateCompany(ctx context.Context, dataDir string, r *MigrationResult) error {
	path := filepath.Join(dataDir, "state", "company.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read company.json: %w", err)
	}
	var c legacyCompany
	if err := json.Unmarshal(data, &c); err != nil {
		r.Errors = append(r.Errors, fmt.Sprintf("company.json unmarshal: %v", err))
		return nil
	}
	if err := s.UpsertCompany(ctx, CompanyRow{
		Name:           c.Name,
		Industry:       c.Industry,
		IndustryType:   c.IndustryType,
		FiscalYearEnd:  c.FiscalYearEnd,
		Currency:       c.Currency,
		SetupCompleted: c.SetupCompleted,
	}); err != nil {
		return fmt.Errorf("upsert company: %w", err)
	}
	r.CompanyImported = true
	return nil
}

func (s *Store) migrateDocuments(ctx context.Context, dataDir string, r *MigrationResult) error {
	path := filepath.Join(dataDir, "state", "documents.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read documents.json: %w", err)
	}
	var list legacyDocumentList
	if err := json.Unmarshal(data, &list); err != nil {
		r.Errors = append(r.Errors, fmt.Sprintf("documents.json unmarshal: %v", err))
		return nil
	}
	for _, d := range list.Documents {
		if err := s.UpsertDocument(ctx, DocumentRow{
			ID:          d.ID,
			Filename:    d.Filename,
			DocType:     d.DocType,
			PeriodStart: d.PeriodStart,
			PeriodEnd:   d.PeriodEnd,
			FilePath:    d.FilePath,
			ParsedPath:  d.ParsedPath,
			FileSize:    d.FileSize,
			MimeType:    d.MimeType,
			UploadedAt:  d.UploadedAt,
		}); err != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("upsert doc %s: %v", d.ID, err))
			continue
		}
		r.DocumentsImported++
	}
	return nil
}

func (s *Store) migrateParsedDocs(ctx context.Context, dataDir string, r *MigrationResult) error {
	parsedDir := filepath.Join(dataDir, "parsed")
	entries, err := os.ReadDir(parsedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read parsed dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		docID := strings.TrimSuffix(e.Name(), ".json")
		if err := s.migrateOneParsedDoc(ctx, parsedDir, e, docID, r); err != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("parsed %s: %v", docID, err))
			continue
		}
	}
	return nil
}

func (s *Store) migrateOneParsedDoc(
	ctx context.Context,
	parsedDir string,
	e fs.DirEntry,
	docID string,
	r *MigrationResult,
) error {
	path := filepath.Join(parsedDir, e.Name())
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var p legacyParsedDoc
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	items := make([]LineItemRow, 0, len(p.Data))
	for k, v := range p.Data {
		items = append(items, LineItemRow{DocumentID: docID, Key: k, Value: v})
	}
	if err := s.ReplaceLineItemsForDocument(ctx, docID, items); err != nil {
		return err
	}
	r.LineItemsImported += len(items)
	return nil
}

// ----- legacy JSON shapes (copies of model package to avoid import cycle) -----

type legacyCompany struct {
	Name           string `json:"name"`
	Industry       string `json:"industry"`
	IndustryType   string `json:"industry_type"`
	FiscalYearEnd  string `json:"fiscal_year_end"`
	Currency       string `json:"currency"`
	SetupCompleted bool   `json:"setup_completed"`
}

type legacyDocument struct {
	ID          string    `json:"id"`
	Filename    string    `json:"filename"`
	DocType     string    `json:"doc_type"`
	PeriodStart string    `json:"period_start"`
	PeriodEnd   string    `json:"period_end"`
	FilePath    string    `json:"file_path"`
	ParsedPath  string    `json:"parsed_path"`
	UploadedAt  time.Time `json:"uploaded_at"`
	FileSize    int64     `json:"file_size"`
	MimeType    string    `json:"mime_type"`
}

type legacyDocumentList struct {
	Documents []legacyDocument `json:"documents"`
}

type legacyParsedDoc struct {
	DocumentID string             `json:"document_id"`
	Data       map[string]float64 `json:"data"`
}
