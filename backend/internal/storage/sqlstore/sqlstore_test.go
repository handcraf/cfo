package sqlstore

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTempStore(t *testing.T) *Store {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestOpen_CreatesSchemaIdempotently(t *testing.T) {
	s := openTempStore(t)
	// Re-applying the schema should not error.
	if err := s.applySchema(context.Background()); err != nil {
		t.Fatalf("re-apply schema: %v", err)
	}
}

func TestCompany_UpsertAndGet(t *testing.T) {
	s := openTempStore(t)
	ctx := context.Background()

	if got, _ := s.GetCompany(ctx); got != nil {
		t.Fatalf("expected no company row initially, got %+v", got)
	}

	c := CompanyRow{
		Name:           "Acme",
		Industry:       "Tech",
		IndustryType:   "generic",
		FiscalYearEnd:  "12-31",
		Currency:       "USD",
		SetupCompleted: true,
	}
	if err := s.UpsertCompany(ctx, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := s.GetCompany(ctx)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.Name != "Acme" || !got.SetupCompleted {
		t.Fatalf("unexpected company: %+v", got)
	}

	// Upsert-update.
	c.Name = "Acme2"
	c.SetupCompleted = false
	if err := s.UpsertCompany(ctx, c); err != nil {
		t.Fatalf("upsert 2: %v", err)
	}
	got, _ = s.GetCompany(ctx)
	if got.Name != "Acme2" || got.SetupCompleted {
		t.Fatalf("upsert did not update: %+v", got)
	}
}

func TestDocumentsAndLineItems(t *testing.T) {
	s := openTempStore(t)
	ctx := context.Background()

	d := DocumentRow{
		ID:          "doc_q1",
		Filename:    "q1.xlsx",
		DocType:     "P&L",
		PeriodStart: "2024-01-01",
		PeriodEnd:   "2024-03-31",
		FilePath:    "/tmp/q1.xlsx",
		ParsedPath:  "/tmp/q1.json",
		FileSize:    123,
		MimeType:    "application/vnd.ms-excel",
		UploadedAt:  time.Now().UTC(),
	}
	if err := s.UpsertDocument(ctx, d); err != nil {
		t.Fatalf("upsert doc: %v", err)
	}

	items := []LineItemRow{
		{DocumentID: "doc_q1", Key: "revenue", Value: 1_000_000},
		{DocumentID: "doc_q1", Key: "cash", Value: 500_000},
	}
	if err := s.ReplaceLineItemsForDocument(ctx, "doc_q1", items); err != nil {
		t.Fatalf("replace items: %v", err)
	}

	// Query metric, in-period.
	hits, err := s.QueryMetric(ctx, "revenue", "2024-01-01", "2024-03-31")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(hits) != 1 || hits[0].Value != 1_000_000 {
		t.Fatalf("unexpected hits: %+v", hits)
	}

	// Query metric, out-of-period.
	hits, err = s.QueryMetric(ctx, "revenue", "2025-01-01", "2025-03-31")
	if err != nil {
		t.Fatalf("query OOP: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected 0 out-of-period hits, got %+v", hits)
	}

	// Replace wipes previous values atomically.
	replaced := []LineItemRow{
		{DocumentID: "doc_q1", Key: "revenue", Value: 2_000_000},
	}
	if err := s.ReplaceLineItemsForDocument(ctx, "doc_q1", replaced); err != nil {
		t.Fatalf("replace again: %v", err)
	}
	hits, _ = s.QueryMetric(ctx, "revenue", "", "")
	if len(hits) != 1 || hits[0].Value != 2_000_000 {
		t.Fatalf("replace didn't take: %+v", hits)
	}
	hits, _ = s.QueryMetric(ctx, "cash", "", "")
	if len(hits) != 0 {
		t.Fatalf("cash should have been deleted in replace, got %+v", hits)
	}

	// Cascade delete: deleting the doc removes its line_items.
	if err := s.DeleteDocument(ctx, "doc_q1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	hits, _ = s.QueryMetric(ctx, "revenue", "", "")
	if len(hits) != 0 {
		t.Fatalf("cascade delete failed: %+v", hits)
	}
}

func TestAskAudit_AppendAndRead(t *testing.T) {
	s := openTempStore(t)
	ctx := context.Background()

	rows := []AskAuditRow{
		{Question: "What was revenue in Q1?", Period: "Q1 2024", Confidence: "high"},
		{Question: "Cash position today?", Confidence: "medium", NumbersUsed: []string{"Cash: 100k"}},
		{Question: "What's burn?", Confidence: "low", Conflicts: 1, ErrorMsg: "something"},
	}
	for _, r := range rows {
		if err := s.RecordAskEvent(ctx, r); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	got, err := s.RecentAsks(ctx, 10)
	if err != nil {
		t.Fatalf("recent: %v", err)
	}
	if len(got) != len(rows) {
		t.Fatalf("expected %d rows, got %d", len(rows), len(got))
	}
	// Newest-first ordering.
	if got[0].Question != "What's burn?" {
		t.Fatalf("expected newest-first ordering; got %q first", got[0].Question)
	}
	if got[0].ErrorMsg != "something" || got[0].Conflicts != 1 {
		t.Fatalf("fields lost: %+v", got[0])
	}
}

func TestListDocuments_OrdersNewestFirst(t *testing.T) {
	s := openTempStore(t)
	ctx := context.Background()

	older := time.Now().UTC().Add(-2 * time.Hour)
	newer := time.Now().UTC().Add(-1 * time.Hour)

	for _, d := range []DocumentRow{
		{ID: "old", Filename: "o.xlsx", DocType: "P&L", PeriodStart: "2024-01-01", PeriodEnd: "2024-03-31", UploadedAt: older},
		{ID: "new", Filename: "n.xlsx", DocType: "P&L", PeriodStart: "2024-04-01", PeriodEnd: "2024-06-30", UploadedAt: newer},
	} {
		if err := s.UpsertDocument(ctx, d); err != nil {
			t.Fatalf("upsert: %v", err)
		}
	}
	got, err := s.ListDocuments(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 || got[0].ID != "new" {
		t.Fatalf("expected newest-first, got %+v", got)
	}
}
