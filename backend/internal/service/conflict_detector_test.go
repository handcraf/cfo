package service

import "testing"

func TestConflictDetector_DetectsRevenueMismatch(t *testing.T) {
	d := NewConflictDetector()
	chunks := []EvidenceChunk{
		{ID: "c1", Source: "pnl.xlsx", Text: "Total revenue for Q1 was $1,200,000."},
		{ID: "c2", Source: "deck.pdf", Text: "Revenue: $1.45M across the quarter."},
	}
	conflicts := d.Detect(chunks)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict, got %d: %+v", len(conflicts), conflicts)
	}
	if conflicts[0].Metric != "revenue" {
		t.Errorf("expected metric=revenue, got %s", conflicts[0].Metric)
	}
	if conflicts[0].SpreadPct < 10 {
		t.Errorf("expected non-trivial spread, got %.2f", conflicts[0].SpreadPct)
	}
	if len(conflicts[0].Values) != 2 {
		t.Errorf("expected 2 distinct values, got %d", len(conflicts[0].Values))
	}
}

func TestConflictDetector_IgnoresRoundingNoise(t *testing.T) {
	d := NewConflictDetector()
	chunks := []EvidenceChunk{
		{ID: "c1", Text: "Cash balance $1,234,567"},
		{ID: "c2", Text: "Cash on hand was 1,234,568"},
	}
	conflicts := d.Detect(chunks)
	if len(conflicts) != 0 {
		t.Fatalf("tiny rounding noise should not be a conflict, got %+v", conflicts)
	}
}

func TestConflictDetector_MatchesMagnitudeSuffixes(t *testing.T) {
	d := NewConflictDetector()
	chunks := []EvidenceChunk{
		{ID: "a", Text: "Revenue was $2M"},
		{ID: "b", Text: "Revenue: 2,000,000"},
		{ID: "c", Text: "Revenue: $3M"},
	}
	conflicts := d.Detect(chunks)
	if len(conflicts) != 1 {
		t.Fatalf("expected 1 conflict (2M vs 3M), got %d", len(conflicts))
	}
}

func TestConflictDetector_NoFalsePositiveSingle(t *testing.T) {
	d := NewConflictDetector()
	chunks := []EvidenceChunk{
		{ID: "a", Text: "Revenue: $1.2M"},
	}
	if conflicts := d.Detect(chunks); len(conflicts) != 0 {
		t.Fatalf("singleton must not conflict, got %+v", conflicts)
	}
}

func TestConflictDetector_RespectsThreshold(t *testing.T) {
	d := &ConflictDetector{MinSpreadPct: 50} // absurd threshold
	chunks := []EvidenceChunk{
		{ID: "a", Text: "Revenue was $1,000,000"},
		{ID: "b", Text: "Revenue was $1,100,000"},
	}
	if conflicts := d.Detect(chunks); len(conflicts) != 0 {
		t.Fatalf("10%% spread should be below 50%% threshold, got %+v", conflicts)
	}
}
