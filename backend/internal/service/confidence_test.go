package service

import "testing"

func TestScore_UnknownWhenNothing(t *testing.T) {
	c := Score(ConfidenceInputs{PeriodMatched: true})
	if c.Level != ConfidenceUnknown {
		t.Fatalf("expected unknown, got %s", c.Level)
	}
	if c.Score != 0 {
		t.Fatalf("expected 0 score, got %f", c.Score)
	}
}

func TestScore_ConflictsForceLow(t *testing.T) {
	c := Score(ConfidenceInputs{
		HasSQLMetrics:  true,
		SQLMetricCount: 5,
		EvidenceCount:  10,
		TopScore:       0.9,
		AgreementRatio: 1.0,
		ConflictCount:  1,
		PeriodMatched:  true,
	})
	if c.Level != ConfidenceLow {
		t.Fatalf("conflicts must force Low, got %s", c.Level)
	}
}

func TestScore_HighRequiresAllSignals(t *testing.T) {
	c := Score(ConfidenceInputs{
		HasSQLMetrics:  true,
		SQLMetricCount: 3,
		EvidenceCount:  3,
		TopScore:       0.35,
		AgreementRatio: 0.8,
		PeriodMatched:  true,
	})
	if c.Level != ConfidenceHigh {
		t.Fatalf("expected high, got %s (%v)", c.Level, c.Reasons)
	}
	if c.Score <= 0 || c.Score > 1 {
		t.Fatalf("score out of range: %f", c.Score)
	}
}

func TestScore_MediumWhenNoPeriodMatch(t *testing.T) {
	c := Score(ConfidenceInputs{
		HasSQLMetrics:  true,
		SQLMetricCount: 2,
		EvidenceCount:  3,
		TopScore:       0.5,
		PeriodMatched:  false,
	})
	if c.Level != ConfidenceMedium {
		t.Fatalf("expected medium (no period), got %s", c.Level)
	}
}

func TestScore_LowWhenEvidenceOnlyAndSparse(t *testing.T) {
	c := Score(ConfidenceInputs{
		HasSQLMetrics: false,
		EvidenceCount: 1,
		TopScore:      0.1,
		PeriodMatched: true,
	})
	if c.Level != ConfidenceLow {
		t.Fatalf("expected low, got %s", c.Level)
	}
}

func TestScore_ReasonsAreNonEmpty(t *testing.T) {
	c := Score(ConfidenceInputs{
		HasSQLMetrics: true,
		EvidenceCount: 2,
		TopScore:      0.2,
		PeriodMatched: true,
	})
	if len(c.Reasons) == 0 {
		t.Fatalf("expected reasons to be populated")
	}
}
