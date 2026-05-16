// Package service — confidence scoring.
//
// The ask flow ends up with three classes of truthiness:
//
//	High   — SQL-backed numbers, strong evidence, no conflicts.
//	Medium — partial coverage: some SQL numbers OR weak evidence.
//	Low    — no SQL numbers, weak or zero evidence, or an open conflict.
//
// This is a DETERMINISTIC function of observable inputs. No ML, no LLM.
// A user can read the score breakdown and trace exactly why we called
// it Medium instead of High.
package service

// ConfidenceLevel is an enum string that's safe to put into API responses.
type ConfidenceLevel string

const (
	ConfidenceHigh    ConfidenceLevel = "high"
	ConfidenceMedium  ConfidenceLevel = "medium"
	ConfidenceLow     ConfidenceLevel = "low"
	ConfidenceUnknown ConfidenceLevel = "unknown"
)

// ConfidenceInputs is every observable signal the scorer considers.
// Keep this struct narrow — every field here must be something the ask
// flow already computes deterministically.
type ConfidenceInputs struct {
	// HasSQLMetrics is true when financial_logic returned at least one
	// non-nil metric for the asked period. This is our ground truth.
	HasSQLMetrics bool

	// SQLMetricCount is how many deterministic metrics we actually have
	// (cash, revenue, burn, runway, ...). More is better.
	SQLMetricCount int

	// EvidenceCount is the post-rerank top-K size.
	EvidenceCount int

	// TopScore is the score of the highest-ranked evidence chunk.
	// For RRF-fused+reranked, meaningful range is roughly 0.01..0.6.
	TopScore float32

	// AgreementRatio: fraction of evidence chunks that reference the same
	// document(s) as the SQL ground truth. High agreement = high confidence.
	// Range [0.0, 1.0]; 0 means "no overlap".
	AgreementRatio float32

	// ConflictCount is the number of numeric conflicts detected.
	ConflictCount int

	// PeriodMatched is true when the user asked about a period and every
	// evidence chunk's period overlaps it. If the user didn't ask a
	// period-scoped question, set this to true.
	PeriodMatched bool
}

// Confidence is the scored result with an auditable reason list.
type Confidence struct {
	Level  ConfidenceLevel `json:"level"`
	Score  float32         `json:"score"`   // 0..1 for UIs that want a number
	Reasons []string       `json:"reasons"` // human-readable audit trail
}

// Score evaluates the inputs and returns a Confidence. The rules are:
//
//	1. Any open conflict caps us at Low.
//	2. No SQL metrics AND no evidence       → Unknown.
//	3. SQL metrics + ≥3 evidence + top≥0.3 + period matched → High.
//	4. SQL metrics OR strong evidence       → Medium.
//	5. Otherwise                             → Low.
//
// The numeric score is a weighted sum for UIs; the Level is what callers
// should key their UX on.
func Score(in ConfidenceInputs) Confidence {
	reasons := make([]string, 0, 4)

	// Rule 1: conflicts always cap confidence.
	if in.ConflictCount > 0 {
		reasons = append(reasons, "conflicting numeric evidence detected")
		return Confidence{
			Level:   ConfidenceLow,
			Score:   0.2,
			Reasons: reasons,
		}
	}

	// Rule 2: nothing to go on at all.
	if !in.HasSQLMetrics && in.EvidenceCount == 0 {
		reasons = append(reasons, "no SQL metrics and no retrieval evidence")
		return Confidence{
			Level:   ConfidenceUnknown,
			Score:   0.0,
			Reasons: reasons,
		}
	}

	// Compute a weighted score for UX.
	var score float32
	if in.HasSQLMetrics {
		score += 0.40
		reasons = append(reasons, "SQL metrics present")
		if in.SQLMetricCount >= 3 {
			score += 0.10
			reasons = append(reasons, "multiple SQL metrics")
		}
	}
	if in.EvidenceCount >= 3 {
		score += 0.15
		reasons = append(reasons, "≥3 evidence chunks")
	} else if in.EvidenceCount > 0 {
		score += 0.05
		reasons = append(reasons, "sparse evidence")
	}
	if in.TopScore >= 0.3 {
		score += 0.15
		reasons = append(reasons, "strong top-chunk score")
	} else if in.TopScore > 0 {
		score += 0.05
		reasons = append(reasons, "weak top-chunk score")
	}
	if in.AgreementRatio >= 0.5 {
		score += 0.10
		reasons = append(reasons, "evidence agrees with SQL sources")
	}
	if in.PeriodMatched {
		score += 0.10
		reasons = append(reasons, "period scope matched")
	} else {
		reasons = append(reasons, "period scope not confirmed")
	}

	if score > 1.0 {
		score = 1.0
	}

	// Rule 3 / 4 / 5: bucketing.
	level := ConfidenceLow
	switch {
	case in.HasSQLMetrics && in.EvidenceCount >= 3 && in.TopScore >= 0.3 && in.PeriodMatched:
		level = ConfidenceHigh
	case in.HasSQLMetrics || (in.EvidenceCount >= 3 && in.TopScore >= 0.3):
		level = ConfidenceMedium
	default:
		level = ConfidenceLow
	}

	return Confidence{Level: level, Score: score, Reasons: reasons}
}
