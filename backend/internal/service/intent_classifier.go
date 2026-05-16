// Package service — deterministic question intent classifier.
//
// This module is the SECOND stage of question parsing (after period_parser).
// It tells the pipeline what KIND of question the user asked, so downstream
// stages can route appropriately.
//
// Why deterministic? Because the architecture contract says:
//   "Backend decides. LLM narrates."
// Routing decisions made by an LLM are non-reproducible. A regex-and-keyword
// classifier is dumb but auditable, and that's the point.
//
// TODO: When we have a labeled corpus of real user questions, train a tiny
// fastText / FastBERT classifier and ship it as a Go-callable ONNX model.
// Until then, this hand-crafted rule set covers the 95th percentile.
package service

import (
	"sort"
	"strings"
)

// Intent is the high-level category of a user question.
type Intent string

const (
	// IntentGreeting — "hi", "hello", "good morning", "thanks", "bye".
	// These should NEVER trigger retrieval or LLM — we reply with a
	// short canned greeting and stop. Treating "hi" as a real question
	// causes the pipeline to summarize the entire company's data, which
	// is both expensive (full Gemma call) and confusing (the user did
	// not ask for that).
	IntentGreeting Intent = "greeting"
	// IntentLookup — "what is X", "show me Y", "what was profit last quarter"
	IntentLookup Intent = "lookup"
	// IntentExplain — "why did revenue drop", "how come margins compressed"
	IntentExplain Intent = "explain"
	// IntentCompare — "compare Q1 vs Q2 revenue", "difference between X and Y"
	IntentCompare Intent = "compare"
	// IntentTrend — "revenue trend over the last year", "growth in expenses"
	IntentTrend Intent = "trend"
	// IntentForecast — "projected runway", "next quarter's burn"
	IntentForecast Intent = "forecast"
	// IntentSummary — "summarize last quarter", "give me an overview"
	IntentSummary Intent = "summary"
	// IntentOutOfScope — "what's the weather", "write me code" — refuse politely.
	IntentOutOfScope Intent = "out_of_scope"
	// IntentUnknown — couldn't classify; treat as lookup but flag low confidence.
	IntentUnknown Intent = "unknown"
)

// ClassifiedIntent is the structured result of classification.
type ClassifiedIntent struct {
	Primary   Intent   // The strongest match
	Score     float32  // 0..1, how confident the classifier is
	Keywords  []string // What triggered the classification (auditable)
	Secondary Intent   // Optional second-strongest, "" if none meaningful
}

// IntentClassifier is a deterministic question categorizer.
// Safe for concurrent use; no state.
type IntentClassifier struct{}

// NewIntentClassifier returns a ready classifier.
func NewIntentClassifier() *IntentClassifier { return &IntentClassifier{} }

// keyword groups are scored cumulatively. A question matching 3 lookup
// keywords beats a question matching 1 trend keyword, hence the weighted
// approach. Weights are deliberately small integers; tweak by feel.
type kwGroup struct {
	intent   Intent
	weight   float32
	patterns []string
}

// greetingExact is a whole-utterance match list: if the user's trimmed,
// lowercased, punctuation-stripped input is one of these strings (or one of
// them followed by very little), it's a greeting. We do exact-match instead
// of substring because "hi" appears inside many real questions ("highest
// revenue") and would otherwise hijack them.
var greetingExact = map[string]bool{
	"hi":             true,
	"hii":            true,
	"hiii":           true,
	"hello":          true,
	"hey":            true,
	"heya":           true,
	"hola":           true,
	"namaste":        true,
	"yo":             true,
	"sup":            true,
	"good morning":   true,
	"good afternoon": true,
	"good evening":   true,
	"morning":        true,
	"thanks":         true,
	"thank you":      true,
	"thank u":        true,
	"thx":            true,
	"ty":             true,
	"ok":             true,
	"okay":           true,
	"k":              true,
	"cool":           true,
	"got it":         true,
	"alright":        true,
	"bye":            true,
	"goodbye":        true,
	"see ya":         true,
	"see you":        true,
	"cya":            true,
	"adios":          true,
}

// We keep these lowercase, single-word or short-phrase patterns. The
// classifier lowercases the question once at entry. Order does not matter.
var intentRules = []kwGroup{
	// OUT OF SCOPE — strongest signal. If matched, we short-circuit.
	{IntentOutOfScope, 2.0, []string{
		"weather", "joke", "tell me a joke", "write code", "code for", "python script",
		"who won", "election", "sports", "movie", "song", "recipe", "what is your name",
		"who are you", "are you ai", "are you human", "are you chatgpt", "are you gemma",
		"what model", "what llm", "what is your model",
	}},

	// COMPARE
	{IntentCompare, 1.5, []string{
		" vs ", " versus ", " vs. ", "compare", "comparison", "difference between",
		"how does", "relative to", "versus", "against",
	}},

	// TREND
	{IntentTrend, 1.4, []string{
		"trend", "over time", "growth", "decline", "change over", "year over year",
		"yoy", "qoq", "month over month", "evolution", "trajectory", "slope",
	}},

	// FORECAST
	{IntentForecast, 1.4, []string{
		"forecast", "predict", "projection", "projected", "expected", "will be",
		"next quarter", "next year", "next month", "going forward", "outlook",
		"future", "estimate for", "what will",
	}},

	// SUMMARY
	{IntentSummary, 1.3, []string{
		"summarize", "summary", "overview", "give me a recap", "tldr", "tl;dr",
		"high level", "bird's eye", "executive summary",
	}},

	// EXPLAIN
	{IntentExplain, 1.2, []string{
		"why", "how come", "explain", "what does it mean", "what does that mean",
		"reason for", "driver of", "what caused", "root cause", "behind",
	}},

	// LOOKUP — broad and low weight so it doesn't dominate.
	{IntentLookup, 1.0, []string{
		"what is", "what was", "what are", "what were", "show me", "show the",
		"tell me", "how much", "how many", "current", "latest", "list",
		"give me the", "report", "value of", "amount of",
	}},
}

// Classify returns the structured intent of a question. It NEVER returns
// nil; in the worst case Primary == IntentUnknown.
func (c *IntentClassifier) Classify(question string) ClassifiedIntent {
	q := strings.ToLower(strings.TrimSpace(question))
	if q == "" {
		return ClassifiedIntent{Primary: IntentUnknown}
	}

	// FIRST: greeting check (whole-utterance match on a short input).
	// Strips trailing punctuation like "hi!" / "hello." / "thanks,"
	// and accepts inputs up to ~24 chars so "hello there" / "hi cfo"
	// also count. Anything longer is presumed to be a real question
	// and falls through to the keyword classifier.
	if g, ok := greetingMatch(q); ok {
		return ClassifiedIntent{
			Primary:  IntentGreeting,
			Score:    1.0,
			Keywords: []string{g},
		}
	}

	scores := make(map[Intent]float32)
	matched := make(map[Intent][]string)

	for _, g := range intentRules {
		for _, p := range g.patterns {
			if strings.Contains(q, p) {
				scores[g.intent] += g.weight
				matched[g.intent] = append(matched[g.intent], p)
			}
		}
	}

	if len(scores) == 0 {
		return ClassifiedIntent{Primary: IntentUnknown, Keywords: []string{}}
	}

	// Pick top two intents by score, deterministic on ties via intent name.
	type pair struct {
		intent Intent
		score  float32
	}
	ranked := make([]pair, 0, len(scores))
	for k, v := range scores {
		ranked = append(ranked, pair{k, v})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return string(ranked[i].intent) < string(ranked[j].intent)
	})

	primary := ranked[0].intent
	secondary := Intent("")
	if len(ranked) > 1 && ranked[1].score >= ranked[0].score*0.6 {
		secondary = ranked[1].intent
	}

	// Normalize score into [0,1]. The denominator caps at 5.0 because in
	// practice the strongest matches are 3-5 weighted keywords.
	norm := ranked[0].score / 5.0
	if norm > 1.0 {
		norm = 1.0
	}

	return ClassifiedIntent{
		Primary:   primary,
		Secondary: secondary,
		Score:     norm,
		Keywords:  matched[primary],
	}
}

// IsRefusable reports whether the LLM should be skipped entirely for this
// intent and a polite refusal returned. We don't want Gemma writing poetry
// when a user asks about the weather.
func (i ClassifiedIntent) IsRefusable() bool {
	return i.Primary == IntentOutOfScope
}

// IsGreeting is a convenience for the ask handler to short-circuit
// chitchat. Greetings are NOT refusals — we reply, just with a fixed
// friendly message, no RAG / LLM / SQL.
func (i ClassifiedIntent) IsGreeting() bool {
	return i.Primary == IntentGreeting
}

// OutOfScopeMessage is the user-facing refusal for out-of-scope questions.
// Kept short and on-brand.
const OutOfScopeMessage = "I'm an AI CFO — I can only answer questions about your company's financial documents and metrics. Please ask something like \"what was our revenue last quarter\" or \"explain the cash burn trend\"."

// GreetingMessage is the canned reply for hi/hello/thanks/bye. Kept brief
// and steers the user toward a useful question.
const GreetingMessage = "Hi! I'm your AI CFO. Ask me about your company's financials — for example: \"What was our revenue last quarter?\", \"How is our cash runway?\", or \"Compare Q1 vs Q2 expenses.\""

// punctTrim is the set of trailing characters we strip before greeting
// matching so "hi!", "hello.", "thanks," all normalize.
const punctTrim = "!?.,;: \t"

// greetingMatch reports whether the (already-lowercased, trimmed) input
// is recognizably a greeting. We accept:
//   - any exact match from greetingExact (after punctuation strip)
//   - a greeting prefix followed by a short addressee ("hi there",
//     "hello cfo", "thanks man") — capped at 24 chars total so a real
//     question that happens to start with "hi" doesn't get misrouted.
func greetingMatch(q string) (string, bool) {
	stripped := strings.TrimRight(q, punctTrim)
	if greetingExact[stripped] {
		return stripped, true
	}
	// Short two-token greetings: first token must be in greetingExact,
	// total length capped so this can't swallow real questions.
	if len(stripped) > 24 {
		return "", false
	}
	parts := strings.Fields(stripped)
	if len(parts) >= 1 && greetingExact[parts[0]] {
		// Only accept up to 3 tokens total and the trailing tokens must
		// be short (no number, no metric name).
		if len(parts) <= 3 {
			joined := strings.Join(parts[1:], " ")
			if joined == "" || isShortAddressee(joined) {
				return parts[0], true
			}
		}
	}
	return "", false
}

// isShortAddressee accepts common conversational tails like "there",
// "cfo", "man", "bro", "team" — i.e., things people append to a
// greeting but that don't change the intent.
func isShortAddressee(s string) bool {
	short := map[string]bool{
		"there": true, "cfo": true, "man": true, "bro": true,
		"sir": true, "team": true, "ai": true, "friend": true,
		"buddy": true, "u": true, "you": true,
	}
	return short[s]
}
