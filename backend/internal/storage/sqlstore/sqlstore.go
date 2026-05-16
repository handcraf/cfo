// Package sqlstore — SQLite source of truth for the AI CFO platform.
//
// Why SQLite (not Postgres)?
//   - Single-file database aligns with the "on-prem single-binary" posture.
//   - modernc.org/sqlite is pure Go — no CGO — so Docker builds stay simple.
//   - Concurrent writers are not a current requirement; readers scale fine.
//   - The data volume (thousands of documents per tenant, maybe millions
//     of line items) comfortably fits SQLite's sweet spot.
//
// Scope for this package:
//   - companies        — one row per company (singleton today, extensible)
//   - documents        — document metadata + period + type + paths
//   - line_items       — parsed key/value financials per document
//   - ask_audit        — append-only audit log for every /ask
//
// Vectors do NOT live here. See service/qdrant_store.go.
//
// Every exported method takes a context. All writes are wrapped in a
// transaction so crashes never leave half-written rows.
package sqlstore

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, registered as "sqlite"
)

//go:embed schema.sql
var schemaFS embed.FS

// Store is the SQLite-backed source of truth.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (or creates) the SQLite database at dbPath and applies the
// schema. The directory must exist. Safe to call multiple times.
func Open(ctx context.Context, dbPath string) (*Store, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("sqlstore: empty dbPath")
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", filepath.Clean(dbPath))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlstore: open: %w", err)
	}
	// Single writer, many readers: keep it conservative.
	db.SetMaxOpenConns(1)

	s := &Store{db: db, path: dbPath}
	if err := s.applySchema(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	log.Printf("[sqlstore] opened %s", dbPath)
	return s, nil
}

// Close releases the database connection. Idempotent.
func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

// applySchema runs the embedded schema.sql. The schema uses
// `CREATE TABLE IF NOT EXISTS` everywhere, so re-running is safe.
//
// TODO: When we need a second migration, introduce a schema_version table
// and a proper migration runner (e.g., goose or a bespoke slice of .sql
// files executed in order). For now the schema is authoritative.
func (s *Store) applySchema(ctx context.Context) error {
	data, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("sqlstore: read embedded schema: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, string(data)); err != nil {
		return fmt.Errorf("sqlstore: apply schema: %w", err)
	}
	return nil
}

// ============================================================================
// Companies
// ============================================================================

// CompanyRow is the SQL projection of model.Company. We keep it local so
// sqlstore does not import model (and model does not import sqlstore).
type CompanyRow struct {
	Name           string
	Industry       string
	IndustryType   string
	FiscalYearEnd  string
	Currency       string
	SetupCompleted bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UpsertCompany inserts-or-updates the single company row.
//
// Today the app is single-tenant, so we use a fixed id=1. When multi-tenant
// lands, add a tenant_id column and switch to upsert-by-key.
func (s *Store) UpsertCompany(ctx context.Context, c CompanyRow) error {
	const q = `
INSERT INTO companies (id, name, industry, industry_type, fiscal_year_end, currency, setup_completed, created_at, updated_at)
VALUES (1, ?, ?, ?, ?, ?, ?, COALESCE((SELECT created_at FROM companies WHERE id=1), ?), ?)
ON CONFLICT(id) DO UPDATE SET
  name=excluded.name,
  industry=excluded.industry,
  industry_type=excluded.industry_type,
  fiscal_year_end=excluded.fiscal_year_end,
  currency=excluded.currency,
  setup_completed=excluded.setup_completed,
  updated_at=excluded.updated_at;`
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, q,
		c.Name, c.Industry, c.IndustryType, c.FiscalYearEnd, c.Currency,
		boolToInt(c.SetupCompleted), now, now)
	return err
}

// GetCompany returns the company row, or (nil, nil) if none exists.
func (s *Store) GetCompany(ctx context.Context) (*CompanyRow, error) {
	const q = `SELECT name, industry, industry_type, fiscal_year_end, currency, setup_completed, created_at, updated_at FROM companies WHERE id=1`
	var c CompanyRow
	var completed int
	err := s.db.QueryRowContext(ctx, q).Scan(
		&c.Name, &c.Industry, &c.IndustryType, &c.FiscalYearEnd, &c.Currency,
		&completed, &c.CreatedAt, &c.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c.SetupCompleted = completed != 0
	return &c, nil
}

// ============================================================================
// Documents
// ============================================================================

// DocumentRow is the SQL projection of model.Document (plus derived fields).
type DocumentRow struct {
	ID          string
	Filename    string
	DocType     string
	PeriodStart string // YYYY-MM-DD
	PeriodEnd   string // YYYY-MM-DD
	FilePath    string
	ParsedPath  string
	FileSize    int64
	MimeType    string
	UploadedAt  time.Time
}

// UpsertDocument writes (or replaces) a document metadata row.
func (s *Store) UpsertDocument(ctx context.Context, d DocumentRow) error {
	const q = `
INSERT INTO documents (id, filename, doc_type, period_start, period_end, file_path, parsed_path, file_size, mime_type, uploaded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  filename=excluded.filename,
  doc_type=excluded.doc_type,
  period_start=excluded.period_start,
  period_end=excluded.period_end,
  file_path=excluded.file_path,
  parsed_path=excluded.parsed_path,
  file_size=excluded.file_size,
  mime_type=excluded.mime_type;`
	_, err := s.db.ExecContext(ctx, q,
		d.ID, d.Filename, d.DocType, d.PeriodStart, d.PeriodEnd,
		d.FilePath, d.ParsedPath, d.FileSize, d.MimeType, d.UploadedAt)
	return err
}

// ListDocuments returns all document rows, ordered newest first.
func (s *Store) ListDocuments(ctx context.Context) ([]DocumentRow, error) {
	const q = `SELECT id, filename, doc_type, period_start, period_end, file_path, parsed_path, file_size, mime_type, uploaded_at FROM documents ORDER BY uploaded_at DESC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DocumentRow
	for rows.Next() {
		var d DocumentRow
		if err := rows.Scan(&d.ID, &d.Filename, &d.DocType, &d.PeriodStart, &d.PeriodEnd,
			&d.FilePath, &d.ParsedPath, &d.FileSize, &d.MimeType, &d.UploadedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteDocument removes a document and all its line_items (FK cascade).
func (s *Store) DeleteDocument(ctx context.Context, documentID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM documents WHERE id=?`, documentID)
	return err
}

// ============================================================================
// Line items (parsed financial key/values)
// ============================================================================

// LineItemRow is a single (document, metric_key) → numeric value row.
// Storing line items as (documentID, key, value) tuples rather than a
// wide table keeps the schema stable as we add new metric keys.
type LineItemRow struct {
	DocumentID string
	Key        string
	Value      float64
}

// ReplaceLineItemsForDocument is the atomic write pattern for parsed data:
// wipe and rewrite inside a single transaction. This is the safest way to
// handle re-parses of the same document.
func (s *Store) ReplaceLineItemsForDocument(ctx context.Context, documentID string, items []LineItemRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Rollback is a no-op after Commit.

	if _, err := tx.ExecContext(ctx, `DELETE FROM line_items WHERE document_id=?`, documentID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO line_items (document_id, metric_key, metric_value) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, li := range items {
		if li.DocumentID != documentID {
			return fmt.Errorf("sqlstore: line item documentID mismatch: %s vs %s", li.DocumentID, documentID)
		}
		if _, err := stmt.ExecContext(ctx, li.DocumentID, li.Key, li.Value); err != nil {
			return fmt.Errorf("sqlstore: insert line item %s=%v: %w", li.Key, li.Value, err)
		}
	}
	return tx.Commit()
}

// QueryMetric returns all (documentID, value) pairs for a metric key,
// optionally restricted to documents whose period overlaps [startDate, endDate].
//
// This is the method financial_logic.go will eventually call instead of
// loading full ParsedDocuments. Keeping it narrow: the caller must still
// aggregate across multiple documents — sqlstore doesn't do finance.
//
// TODO: wire FinancialLogic to prefer this when the SQL store is present.
func (s *Store) QueryMetric(ctx context.Context, metricKey, startDate, endDate string) ([]LineItemHit, error) {
	var b strings.Builder
	b.WriteString(`
SELECT li.document_id, li.metric_value, d.period_start, d.period_end
FROM line_items li
JOIN documents d ON d.id = li.document_id
WHERE li.metric_key = ?`)
	args := []any{metricKey}

	if startDate != "" && endDate != "" {
		// Overlap predicate (lexically sortable YYYY-MM-DD):
		//   NOT (docEnd < queryStart OR queryEnd < docStart)
		b.WriteString(` AND NOT (d.period_end < ? OR ? < d.period_start)`)
		args = append(args, startDate, endDate)
	}
	b.WriteString(` ORDER BY d.period_end DESC`)

	rows, err := s.db.QueryContext(ctx, b.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LineItemHit
	for rows.Next() {
		var h LineItemHit
		if err := rows.Scan(&h.DocumentID, &h.Value, &h.PeriodStart, &h.PeriodEnd); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// LineItemHit is a single result from QueryMetric.
type LineItemHit struct {
	DocumentID  string
	Value       float64
	PeriodStart string
	PeriodEnd   string
}

// ============================================================================
// Ask audit log
// ============================================================================

// AskAuditRow is one ask event. Append-only.
type AskAuditRow struct {
	Question    string
	Period      string
	NumbersUsed []string
	EvidenceIDs []string
	Confidence  string
	Conflicts   int
	ErrorMsg    string
}

// RecordAskEvent appends a row. Best-effort: this is a forensic log, not
// a blocking path. Callers should log errors but not surface them to users.
func (s *Store) RecordAskEvent(ctx context.Context, r AskAuditRow) error {
	const q = `
INSERT INTO ask_audit (at, question, period, numbers_used, evidence_ids, confidence, conflicts, error_msg)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, q,
		time.Now().UTC(),
		r.Question,
		r.Period,
		strings.Join(r.NumbersUsed, "\n"),
		strings.Join(r.EvidenceIDs, ","),
		r.Confidence,
		r.Conflicts,
		r.ErrorMsg,
	)
	return err
}

// RecentAsks returns the N most recent audit rows. Useful for a /admin/audit
// endpoint (not wired in this PR).
func (s *Store) RecentAsks(ctx context.Context, limit int) ([]AskAuditRow, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT question, period, numbers_used, evidence_ids, confidence, conflicts, error_msg
FROM ask_audit ORDER BY at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AskAuditRow
	for rows.Next() {
		var r AskAuditRow
		var numbers, evidence string
		if err := rows.Scan(&r.Question, &r.Period, &numbers, &evidence, &r.Confidence, &r.Conflicts, &r.ErrorMsg); err != nil {
			return nil, err
		}
		if numbers != "" {
			r.NumbersUsed = strings.Split(numbers, "\n")
		}
		if evidence != "" {
			r.EvidenceIDs = strings.Split(evidence, ",")
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ============================================================================
// helpers
// ============================================================================

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
