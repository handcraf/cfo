package model

// FinancialMetrics represents calculated financial metrics
type FinancialMetrics struct {
	Cash           *float64         `json:"cash"`
	MonthlyBurn    *float64         `json:"monthly_burn"`
	RunwayMonths   *float64         `json:"runway_months"`
	Revenue        *float64         `json:"revenue"`
	Expenses       *float64         `json:"expenses"`
	NetIncome      *float64         `json:"net_income"`
	TotalAssets    *float64         `json:"total_assets"`
	TotalLiab      *float64         `json:"total_liabilities"`
	Equity         *float64         `json:"equity"`
	PeriodStart    string           `json:"period_start"`
	PeriodEnd      string           `json:"period_end"`
	Trends         *TrendData       `json:"trends,omitempty"`
	Errors         []string         `json:"errors,omitempty"` // Any calculation errors
	DataSources    []string         `json:"data_sources"`     // Document IDs used
}

// TrendData represents period-over-period comparison
type TrendData struct {
	RevenueChange  *float64 `json:"revenue_change_pct,omitempty"`
	ExpenseChange  *float64 `json:"expense_change_pct,omitempty"`
	CashChange     *float64 `json:"cash_change_pct,omitempty"`
	BurnChange     *float64 `json:"burn_change_pct,omitempty"`
}

// AskRequest represents a question to the CFO
type AskRequest struct {
	Question string `json:"question"`
}

// AskResponse represents the CFO's answer.
//
// Production-grade fields (Confidence, Conflicts, Evidence) let the UI
// render trustworthy answers:
//   - render "Uncertain" when Conflicts is non-empty
//   - badge the answer with Confidence.Level
//   - show Evidence chunks with their source+score for audit
//
// Numbers still come from SQL/deterministic calc only. The LLM only
// produces Summary and Explanation (see llm-boundary.mdc rule).
type AskResponse struct {
	Question    string   `json:"question"`
	Summary     string   `json:"summary"`
	NumbersUsed []string `json:"numbers_used"`
	Explanation string   `json:"explanation"`
	Sources     []string `json:"sources"` // Document IDs referenced
	Error       string   `json:"error,omitempty"`

	// Confidence is the backend's self-assessment of answer reliability.
	// Always populated; read Confidence.Level for bucketing, Confidence.Score
	// for a numeric UI, Confidence.Reasons for the audit trail.
	Confidence *AskConfidence `json:"confidence,omitempty"`

	// Conflicts is non-empty when the retrieved evidence contains
	// disagreeing numeric claims. A responsible UI renders the answer as
	// "Uncertain" and surfaces the conflict to the user.
	Conflicts []AskConflict `json:"conflicts,omitempty"`

	// Evidence is the final top-K chunks shown to the LLM, retained so
	// the UI can cite them. Text is truncated; full text is in the store.
	Evidence []AskEvidence `json:"evidence,omitempty"`
}

// AskConfidence mirrors service.Confidence for the API shape.
type AskConfidence struct {
	Level   string   `json:"level"`  // "high" | "medium" | "low" | "unknown"
	Score   float32  `json:"score"`  // 0..1
	Reasons []string `json:"reasons"`
}

// AskConflict mirrors service.Conflict for the API shape.
type AskConflict struct {
	Metric    string             `json:"metric"`
	SpreadPct float64            `json:"spread_pct"`
	Values    []AskConflictValue `json:"values"`
}

type AskConflictValue struct {
	Value   float64 `json:"value"`
	ChunkID string  `json:"chunk_id"`
	Source  string  `json:"source"`
}

// AskEvidence is a trimmed-for-API view of service.EvidenceChunk.
type AskEvidence struct {
	ID          string  `json:"id"`
	DocumentID  string  `json:"document_id"`
	Source      string  `json:"source"`
	DocType     string  `json:"doc_type,omitempty"`
	PeriodStart string  `json:"period_start,omitempty"`
	PeriodEnd   string  `json:"period_end,omitempty"`
	Score       float32 `json:"score"`
	Text        string  `json:"text"` // may be truncated for wire size
}

