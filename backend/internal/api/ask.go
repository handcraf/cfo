// Package api — /ask HTTP handler.
//
// The production ask flow is:
//
//	1. period_parser   resolves "Q1 2024" etc. deterministically
//	2. financial_logic computes SQL-backed numbers for that period
//	3. HybridRetriever retrieves evidence (semantic + keyword, RRF-fused)
//	                  with filters applied BEFORE retrieval
//	4. Reranker        deterministic rerank → top-K (default 5)
//	5. ConflictDetect  checks for disagreeing numeric claims
//	6. Confidence      deterministic High/Medium/Low/Unknown
//	7. LLM             explains the numbers; receives no freedom to compute
//	8. Audit           append-only trail (stage 3)
//
// Invariants (from AGENTS.md):
//   - Numbers come from financial_logic, never from the LLM.
//   - Retrieval filters run before search, not after.
//   - Conflicts cap confidence at Low.
//   - LLM receives pre-computed facts + evidence + confidence; it narrates.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cfo/backend/internal/config"
	"github.com/cfo/backend/internal/industry"
	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/service"
	"github.com/cfo/backend/internal/storage"
)

// llmFallbackMessage is surfaced when the local llama.cpp runtime fails.
// The copy is deliberately short and actionable — users should not see
// stack traces or "model unreachable" jargon.
const llmFallbackMessage = "Unable to generate explanation. Please try again."

// AskHandler handles the ask endpoint.
type AskHandler struct {
	store        *storage.FileStore
	cfg          *config.Config
	rag          *service.RAGService
	llm          *service.LLMService
	finLogic     *service.FinancialLogic
	periodParser *service.PeriodParser
	// intentClf is the deterministic question-intent classifier added in
	// the "smarter CFO" round. It runs BEFORE retrieval so we can route
	// out-of-scope questions to a refusal and shape evidence retrieval
	// by question category.
	intentClf *service.IntentClassifier

	// Stage 2 additions. Any may be nil; the flow degrades gracefully.
	vectorStore  service.Store
	hybrid       *service.HybridRetriever
	reranker     *service.Reranker
	conflictDetr *service.ConflictDetector

	// Stage 3 (optional): audit log sink. Nil = no audit.
	// TODO: wire when sqlstore lands.
	audit AskAuditSink
}

// AskAuditSink is the minimal interface for append-only ask audit logging.
// Implemented by storage/sqlstore.AskAudit in Stage 3.
type AskAuditSink interface {
	Record(ctx context.Context, event AskAuditEvent) error
}

// AskAuditEvent is the row shape we append for every /ask call.
// Kept narrow on purpose: this is a forensic log, not a telemetry stream.
type AskAuditEvent struct {
	Question    string
	Period      string
	NumbersUsed []string
	EvidenceIDs []string
	Confidence  string
	Conflicts   int
	Error       string
}

// NewAskHandler creates a new AskHandler with the stage-1/2 pipeline wired up.
// Callers that want a degraded handler (no vector store) may pass a nil Store.
func NewAskHandler(store *storage.FileStore, cfg *config.Config) *AskHandler {
	h := &AskHandler{
		store:        store,
		cfg:          cfg,
		rag:          service.NewRAGService(store),
		llm: service.NewLLMService(service.LLMConfig{
			Binary:      cfg.LlamaCppBinary,
			ModelPath:   cfg.ModelPath,
			MaxTokens:   cfg.LLMMaxTokens,
			Temperature: cfg.LLMTemperature,
			TopP:        cfg.LLMTopP,
			Seed:        cfg.LLMSeed,
			ContextSize: cfg.LLMContextSize,
			Timeout:     time.Duration(cfg.LLMTimeoutSec) * time.Second,
			Threads:     cfg.LLMThreads,
		}),
		finLogic:     service.NewFinancialLogic(store),
		periodParser: service.NewPeriodParser(),
		intentClf:    service.NewIntentClassifier(),
		conflictDetr: service.NewConflictDetector(),
		reranker:     service.NewReranker(service.DefaultRerankWeights()),
	}
	// Hybrid works with either side nil; wire it up.
	h.hybrid = service.NewHybridRetriever(nil, h.rag, store)
	return h
}

// WithVectorStore attaches an optional vector backend. Call this after
// constructing the handler if a Store is available (in-memory or Qdrant).
// When no Store is attached, retrieval falls back to keyword-only.
func (h *AskHandler) WithVectorStore(vs service.Store) *AskHandler {
	h.vectorStore = vs
	h.hybrid = service.NewHybridRetriever(vs, h.rag, h.store)
	return h
}

// WithAudit attaches the ask audit sink (Stage 3).
func (h *AskHandler) WithAudit(a AskAuditSink) *AskHandler {
	h.audit = a
	return h
}

// getCompanyIndustryType retrieves the company's industry type for specialized handling.
func (h *AskHandler) getCompanyIndustryType() model.IndustryType {
	company, err := h.store.LoadCompany()
	if err != nil || company == nil {
		return model.IndustryGeneric
	}
	if company.IndustryType == "" {
		return model.IndustryGeneric
	}
	return company.IndustryType
}

// industryContext holds context data from industry-specific handlers.
type industryContext struct {
	industryType   model.IndustryType
	industryIntent string
	chunks         []industry.Chunk
	vocabulary     []string
	engaged        bool
}

// Ask handles POST /ask.
func (h *AskHandler) Ask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req model.AskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Question == "" {
		writeError(w, "Question is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// -----------------------------------------------------------------------
	// Step 0: Classify intent. Two short-circuits live here:
	//
	//   a) GREETING — "hi", "hello", "thanks", "bye" → fixed friendly
	//      reply. NO retrieval, NO SQL, NO LLM. Treating these as real
	//      questions causes the pipeline to summarize the entire company,
	//      which is both expensive and surprising to the user.
	//
	//   b) OUT_OF_SCOPE — weather/jokes/code/self-identity → polite
	//      refusal that steers the user back to CFO topics.
	//
	// Both paths skip Gemma entirely (sub-millisecond response).
	// -----------------------------------------------------------------------
	intent := h.intentClf.Classify(req.Question)

	if intent.IsGreeting() {
		log.Printf("[Ask] greeting handled: keyword=%v", intent.Keywords)
		response := model.AskResponse{
			Question:    req.Question,
			Summary:     service.GreetingMessage,
			Explanation: "",
			NumbersUsed: []string{},
			Sources:     []string{},
			Confidence: &model.AskConfidence{
				Level:   "unknown",
				Score:   0,
				Reasons: []string{"greeting — no question asked", "intent=greeting"},
			},
		}
		h.recordAudit(ctx, req.Question, service.ParsedPeriod{}, nil, nil,
			service.Confidence{Level: service.ConfidenceUnknown, Score: 0,
				Reasons: []string{"greeting"}}, nil, "")
		writeJSON(w, response)
		return
	}

	if intent.IsRefusable() {
		log.Printf("[Ask] out-of-scope question rejected: keywords=%v", intent.Keywords)
		response := model.AskResponse{
			Question:    req.Question,
			Summary:     service.OutOfScopeMessage,
			Explanation: "",
			// Use empty slices, not nil, so JSON renders as [] not null —
			// downstream consumers (e2e tests, frontend) expect arrays.
			NumbersUsed: []string{},
			Sources:     []string{},
			Confidence: &model.AskConfidence{
				Level:   "unknown",
				Score:   0,
				Reasons: []string{"question is out of CFO scope", "intent=" + string(intent.Primary)},
			},
		}
		h.recordAudit(ctx, req.Question, service.ParsedPeriod{}, nil, nil,
			service.Confidence{Level: service.ConfidenceUnknown, Score: 0,
				Reasons: []string{"out_of_scope"}}, nil, "")
		writeJSON(w, response)
		return
	}

	// -----------------------------------------------------------------------
	// Step 1: Parse period
	// -----------------------------------------------------------------------
	period := h.periodParser.Parse(req.Question)

	// -----------------------------------------------------------------------
	// Step 2: Compute SQL-backed metrics (the "facts")
	// -----------------------------------------------------------------------
	metrics := h.computeMetrics(period)

	// -----------------------------------------------------------------------
	// Step 3: Industry resolution (provides filter signals + vocabulary)
	// -----------------------------------------------------------------------
	industryType := h.getCompanyIndustryType()
	indCtx := h.resolveIndustryContext(req.Question, period, industryType)

	// -----------------------------------------------------------------------
	// Step 4: Hybrid retrieval — first pass with strict filter, then a
	// broadened fallback pass if recall was empty. The LLM still sees the
	// user's original wording; only the RETRIEVAL query is expanded with
	// finance-synonyms so we improve recall without changing semantics.
	// -----------------------------------------------------------------------
	expandedQuery := service.ExpandQuery(req.Question)
	// Retrieval period: prefer the user's explicit period; fall back to
	// the period the metrics ended up being computed for (so we don't
	// retrieve Q3 chunks when the metrics describe Q1, which the conflict
	// detector would then misread as disagreement).
	periodStart, periodEnd := period.Start, period.End
	if !period.Detected && metrics != nil {
		periodStart, periodEnd = metrics.PeriodStart, metrics.PeriodEnd
	}
	strictFilter := service.VectorFilter{
		Industry:    industryType,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	}
	candidates, err := h.hybrid.Retrieve(ctx, service.HybridQuery{
		Text:   expandedQuery,
		TopN:   service.DefaultHybridTopN,
		Filter: strictFilter,
	})
	if err != nil {
		log.Printf("[Ask] hybrid retrieval (pass 1) error: %v", err)
	}

	// Two-pass fallback: if the strict pass returned nothing, broaden by
	// dropping the period filter (industry stays — wrong-industry chunks
	// are worse than no chunks). We accept that broadened retrieval may
	// not match the requested period; the reranker will down-score those.
	if len(candidates) == 0 && (strictFilter.PeriodStart != "" || strictFilter.PeriodEnd != "") {
		broad := service.VectorFilter{Industry: industryType}
		fallback, ferr := h.hybrid.Retrieve(ctx, service.HybridQuery{
			Text:   expandedQuery,
			TopN:   service.DefaultHybridTopN,
			Filter: broad,
		})
		if ferr != nil {
			log.Printf("[Ask] hybrid retrieval (pass 2/broadened) error: %v", ferr)
		}
		if len(fallback) > 0 {
			log.Printf("[Ask] used broadened-filter fallback: recovered %d chunks", len(fallback))
			candidates = fallback
		}
	}

	// -----------------------------------------------------------------------
	// Step 5: Deterministic rerank → top-5
	// -----------------------------------------------------------------------
	signals := service.RerankSignals{
		Industry:         industryType,
		PeriodStart:      periodStart,
		PeriodEnd:        periodEnd,
		PreferredSources: metricsDataSources(metrics),
	}
	topChunks := h.reranker.Rerank(candidates, signals, 5)

	// -----------------------------------------------------------------------
	// Step 6: Conflict detection (numeric only; temporal is TODO)
	//
	// Only run the detector on chunks whose period actually overlaps the
	// metrics period. Otherwise the detector compares unrelated-period
	// numbers (e.g., 2024 revenue vs 2026 metrics) and reports false
	// "conflicts" that aren't real disagreements — they're just different
	// time periods being measured.
	// -----------------------------------------------------------------------
	conflictChunks := chunksInPeriod(topChunks, periodStart, periodEnd)
	conflicts := h.conflictDetr.Detect(conflictChunks)

	// -----------------------------------------------------------------------
	// Step 7: Confidence score
	// -----------------------------------------------------------------------
	conf := service.Score(service.ConfidenceInputs{
		HasSQLMetrics:  hasAnyMetric(metrics),
		SQLMetricCount: metricCount(metrics),
		EvidenceCount:  len(topChunks),
		TopScore:       topScore(topChunks),
		AgreementRatio: agreementRatio(topChunks, metrics.DataSources),
		ConflictCount:  len(conflicts),
		PeriodMatched:  periodStart == "" || allChunksMatchPeriod(topChunks, periodStart, periodEnd),
	})

	numbersUsed := h.finLogic.FormatMetricsForPrompt(metrics)

	// -----------------------------------------------------------------------
	// Step 8: LLM explains (never computes)
	// -----------------------------------------------------------------------
	enrichedContext := h.buildEnrichedContext(period, topChunks, indCtx, conf)

	response := model.AskResponse{
		Question:    req.Question,
		NumbersUsed: numbersUsed,
		Sources:     h.collectSourceNames(topChunks),
		Confidence:  toAskConfidence(conf),
		Conflicts:   toAskConflicts(conflicts),
		Evidence:    toAskEvidence(topChunks),
	}

	// If confidence is "unknown" (no SQL + no evidence) OR we have open
	// conflicts, short-circuit the LLM: we have nothing solid to explain.
	if conf.Level == service.ConfidenceUnknown || len(conflicts) > 0 {
		response.Summary = uncertainSummary(conf, conflicts)
		response.Explanation = "The backend does not have enough consistent data to answer confidently. See evidence and conflicts."
		h.recordAudit(ctx, req.Question, period, numbersUsed, topChunks, conf, conflicts, "")
		writeJSON(w, response)
		return
	}

	explanation, err := h.llm.ExplainMetrics(req.Question, numbersUsed, enrichedContext)
	if err != nil {
		log.Printf("[ASK] llama.cpp runtime failed: %v", err)
		response.Summary = llmFallbackMessage
		response.Explanation = ""
		response.Error = err.Error()
		h.recordAudit(ctx, req.Question, period, numbersUsed, topChunks, conf, conflicts, err.Error())
		writeJSON(w, response)
		return
	}

	response.Summary = explanation.Summary
	response.Explanation = explanation.Detail

	h.recordAudit(ctx, req.Question, period, numbersUsed, topChunks, conf, conflicts, "")
	writeJSON(w, response)
}

// computeMetrics is unchanged from the pre-Stage-2 flow except that it's
// factored out of Ask to keep the pipeline readable.
func (h *AskHandler) computeMetrics(period service.ParsedPeriod) *model.FinancialMetrics {
	var metrics *model.FinancialMetrics
	var err error
	if period.Detected {
		metrics, err = h.finLogic.CalculateMetricsForPeriod(period.Start, period.End)
		if err != nil || !hasAnyMetric(metrics) {
			metrics, err = h.finLogic.CalculateCurrentMetrics()
		}
	} else {
		metrics, err = h.finLogic.CalculateCurrentMetrics()
	}
	if err != nil {
		metrics = &model.FinancialMetrics{Errors: []string{err.Error()}}
	}
	if period.Detected {
		metrics.PeriodStart = period.Start
		metrics.PeriodEnd = period.End
	}
	return metrics
}

// resolveIndustryContext is unchanged behaviorally. It runs AFTER we know
// the industry type to gather vocabulary and domain hints for the prompt.
func (h *AskHandler) resolveIndustryContext(question string, period service.ParsedPeriod, industryType model.IndustryType) industryContext {
	ctx := industryContext{industryType: industryType}

	if industryType == model.IndustryGeneric || industryType == "" {
		return ctx
	}

	handler := industry.GetIndustryHandler(industryType)
	if handler == nil {
		return ctx
	}
	ctx.engaged = true
	ctx.vocabulary = handler.GetIndustryVocabulary()

	intent, ok := handler.ResolveIndustryIntent(question)
	if !ok {
		return ctx
	}
	ctx.industryIntent = intent

	modelPeriod := model.Period{Start: period.Start, End: period.End}
	chunks, err := handler.FetchIndustryData(intent, modelPeriod)
	if err == nil {
		ctx.chunks = chunks
	}
	return ctx
}

// buildEnrichedContext renders the evidence chunks + industry hints into
// the blob the LLM receives. It is a *narration* input, not a reasoning
// input — the numbers still come from numbersUsed, passed separately.
func (h *AskHandler) buildEnrichedContext(
	period service.ParsedPeriod,
	chunks []service.EvidenceChunk,
	indCtx industryContext,
	conf service.Confidence,
) string {
	var b strings.Builder

	if period.Detected {
		fmt.Fprintf(&b, "Time Period: %s (from %s to %s)\n\n", period.Label, period.Start, period.End)
	}

	fmt.Fprintf(&b, "Backend Confidence: %s (score %.2f)\n", conf.Level, conf.Score)
	if len(conf.Reasons) > 0 {
		fmt.Fprintf(&b, "Reasons: %s\n\n", strings.Join(conf.Reasons, "; "))
	}

	if len(chunks) > 0 {
		b.WriteString("Evidence Chunks (pre-retrieved; do not extrapolate):\n")
		for i, c := range chunks {
			fmt.Fprintf(&b, "[E%d | src=%s | score=%.3f]\n%s\n---\n",
				i+1, c.Source, c.Score, c.Text)
		}
	}

	if indCtx.engaged {
		fmt.Fprintf(&b, "\n[Industry Context: %s]\n", indCtx.industryType)
		if len(indCtx.vocabulary) > 0 {
			b.WriteString("Industry Terminology:\n")
			max := 5
			if len(indCtx.vocabulary) < max {
				max = len(indCtx.vocabulary)
			}
			for i := 0; i < max; i++ {
				fmt.Fprintf(&b, "  - %s\n", indCtx.vocabulary[i])
			}
		}
		if indCtx.industryIntent != "" {
			fmt.Fprintf(&b, "Detected Industry Intent: %s\n", indCtx.industryIntent)
		}
	}

	return b.String()
}

// recordAudit is best-effort. Audit failures must NEVER block the response.
func (h *AskHandler) recordAudit(
	ctx context.Context,
	question string,
	period service.ParsedPeriod,
	numbers []string,
	chunks []service.EvidenceChunk,
	conf service.Confidence,
	conflicts []service.Conflict,
	errStr string,
) {
	if h.audit == nil {
		return
	}
	ids := make([]string, 0, len(chunks))
	for _, c := range chunks {
		ids = append(ids, c.ID)
	}
	evt := AskAuditEvent{
		Question:    question,
		Period:      period.Label,
		NumbersUsed: numbers,
		EvidenceIDs: ids,
		Confidence:  string(conf.Level),
		Conflicts:   len(conflicts),
		Error:       errStr,
	}
	if err := h.audit.Record(ctx, evt); err != nil {
		log.Printf("[Ask][audit] failed to record event: %v", err)
	}
}

// -----------------------------------------------------------------------
// small helpers kept local to this file (no public API)
// -----------------------------------------------------------------------

func hasAnyMetric(m *model.FinancialMetrics) bool {
	if m == nil {
		return false
	}
	return m.Cash != nil || m.Revenue != nil || m.Expenses != nil ||
		m.NetIncome != nil || m.TotalAssets != nil || m.TotalLiab != nil ||
		m.Equity != nil || m.MonthlyBurn != nil || m.RunwayMonths != nil
}

func metricCount(m *model.FinancialMetrics) int {
	if m == nil {
		return 0
	}
	n := 0
	ptrs := []*float64{m.Cash, m.Revenue, m.Expenses, m.NetIncome,
		m.TotalAssets, m.TotalLiab, m.Equity, m.MonthlyBurn, m.RunwayMonths}
	for _, p := range ptrs {
		if p != nil {
			n++
		}
	}
	return n
}

func metricsDataSources(m *model.FinancialMetrics) []string {
	if m == nil {
		return nil
	}
	return m.DataSources
}

func topScore(chunks []service.EvidenceChunk) float32 {
	if len(chunks) == 0 {
		return 0
	}
	return chunks[0].Score
}

// agreementRatio returns the fraction of evidence chunks whose DocumentID
// overlaps the set of SQL data sources. It's bounded [0,1].
func agreementRatio(chunks []service.EvidenceChunk, sqlSources []string) float32 {
	if len(chunks) == 0 || len(sqlSources) == 0 {
		return 0
	}
	sqlSet := make(map[string]struct{}, len(sqlSources))
	for _, s := range sqlSources {
		sqlSet[s] = struct{}{}
	}
	hits := 0
	for _, c := range chunks {
		if _, ok := sqlSet[c.DocumentID]; ok {
			hits++
		}
	}
	return float32(hits) / float32(len(chunks))
}

// chunksInPeriod returns the subset of chunks whose declared period
// overlaps [start,end]. Chunks with NO period information are passed
// through (we can't disprove overlap). When start or end is empty,
// returns the input unchanged.
func chunksInPeriod(chunks []service.EvidenceChunk, start, end string) []service.EvidenceChunk {
	if start == "" || end == "" {
		return chunks
	}
	out := make([]service.EvidenceChunk, 0, len(chunks))
	for _, c := range chunks {
		if c.PeriodStart == "" || c.PeriodEnd == "" {
			out = append(out, c)
			continue
		}
		// Standard interval overlap: NOT(A ends before B starts OR B ends before A starts).
		if c.PeriodEnd < start || end < c.PeriodStart {
			continue
		}
		out = append(out, c)
	}
	return out
}

func allChunksMatchPeriod(chunks []service.EvidenceChunk, start, end string) bool {
	if start == "" || end == "" {
		return true
	}
	for _, c := range chunks {
		if c.PeriodStart == "" || c.PeriodEnd == "" {
			continue // unknown period: don't penalize, don't confirm either
		}
		// Use same overlap logic as the filter for consistency.
		if c.PeriodEnd < start || end < c.PeriodStart {
			return false
		}
	}
	return true
}

// collectSourceNames returns a deduped list of human-readable source names
// for the top chunks. We prefer the Source field (typically the original
// filename) and fall back to DocumentID only if Source is empty. This is
// what users see in the UI as "Sources: [P&L_Q1.xlsx, BalanceSheet.pdf]".
func (h *AskHandler) collectSourceNames(chunks []service.EvidenceChunk) []string {
	seen := make(map[string]struct{}, len(chunks))
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		name := c.Source
		if name == "" {
			name = c.DocumentID
		}
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

const maxEvidenceTextWire = 1200

func toAskEvidence(chunks []service.EvidenceChunk) []model.AskEvidence {
	out := make([]model.AskEvidence, 0, len(chunks))
	for _, c := range chunks {
		text := c.Text
		if len(text) > maxEvidenceTextWire {
			text = text[:maxEvidenceTextWire] + "..."
		}
		out = append(out, model.AskEvidence{
			ID:          c.ID,
			DocumentID:  c.DocumentID,
			Source:      c.Source,
			DocType:     string(c.DocType),
			PeriodStart: c.PeriodStart,
			PeriodEnd:   c.PeriodEnd,
			Score:       c.Score,
			Text:        text,
		})
	}
	return out
}

func toAskConfidence(c service.Confidence) *model.AskConfidence {
	return &model.AskConfidence{
		Level:   string(c.Level),
		Score:   c.Score,
		Reasons: c.Reasons,
	}
}

func toAskConflicts(conflicts []service.Conflict) []model.AskConflict {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]model.AskConflict, 0, len(conflicts))
	for _, c := range conflicts {
		vs := make([]model.AskConflictValue, 0, len(c.Values))
		for _, v := range c.Values {
			vs = append(vs, model.AskConflictValue{
				Value:   v.Value,
				ChunkID: v.ChunkID,
				Source:  v.Source,
			})
		}
		out = append(out, model.AskConflict{
			Metric:    c.Metric,
			SpreadPct: c.SpreadPct,
			Values:    vs,
		})
	}
	return out
}

func uncertainSummary(conf service.Confidence, conflicts []service.Conflict) string {
	if len(conflicts) > 0 {
		names := make([]string, 0, len(conflicts))
		for _, c := range conflicts {
			names = append(names, c.Metric)
		}
		return fmt.Sprintf("Uncertain — conflicting values detected for: %s.", strings.Join(names, ", "))
	}
	return "Uncertain — insufficient data to answer confidently."
}
