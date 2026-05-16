// Package service — financial-term synonym expansion for retrieval recall.
//
// Why: users phrase questions in many ways ("burn", "monthly spend",
// "operating expenses") but the source documents typically use a single
// canonical term. Without expansion, a perfectly good question can miss
// every chunk because the keyword retriever doesn't match.
//
// This module DOES NOT touch the LLM prompt — the LLM still sees the
// original question. Expansion is ONLY used to widen the retrieval recall.
// That preserves the determinism contract: the user's words are what the
// model narrates.
//
// TODO: Pull this list from an industry module so terminology can vary
// (e.g., healthcare "claims" vs SaaS "ARR"). For now, generic finance.
package service

import (
	"sort"
	"strings"
)

// Synonyms is the canonical map: each entry says "if the question mentions
// any of the alias keys, also try the canonical term(s) during retrieval."
// Both sides are lowercase, single tokens or short multi-word phrases.
var synonymGroups = []struct {
	canonical string
	aliases   []string
}{
	{"revenue", []string{"sales", "top line", "topline", "turnover", "billings", "income"}},
	{"net income", []string{"profit", "net profit", "earnings", "bottom line", "bottomline", "income after tax", "pat"}},
	{"gross profit", []string{"gross margin dollar", "gp", "gross income"}},
	{"gross margin", []string{"gp%", "gross profit %", "gross profit percent", "gm%"}},
	{"operating income", []string{"ebit", "operating profit", "op income"}},
	{"ebitda", []string{"operating cash earnings"}},
	{"expenses", []string{"costs", "opex", "operating expenses", "spend", "outflows"}},
	{"cost of goods sold", []string{"cogs", "cost of sales", "direct costs"}},
	{"monthly burn", []string{"burn", "burn rate", "cash burn", "monthly spend"}},
	{"runway", []string{"cash runway", "months remaining", "how long", "survival"}},
	{"cash", []string{"cash position", "cash on hand", "cash balance", "liquidity"}},
	{"total assets", []string{"assets"}},
	{"total liabilities", []string{"liabilities", "debts owed", "what we owe"}},
	{"equity", []string{"shareholders equity", "stockholders equity", "net worth"}},
	{"receivables", []string{"ar", "accounts receivable", "money owed to us"}},
	{"payables", []string{"ap", "accounts payable", "money we owe"}},
	{"deferred revenue", []string{"unearned revenue", "advance billings"}},
	{"churn", []string{"customer churn", "logo churn", "attrition"}},
	{"arr", []string{"annual recurring revenue", "annualized recurring revenue"}},
	{"mrr", []string{"monthly recurring revenue"}},
	{"cac", []string{"customer acquisition cost"}},
	{"ltv", []string{"lifetime value", "customer lifetime value", "clv"}},
}

// Expand returns a list of canonical terms relevant to the question.
// The returned slice is deduplicated, lowercased, deterministic order
// (sorted alphabetically). Empty if no aliases matched.
//
// The original question is NOT included — callers append it separately
// if they want.
func ExpandSynonyms(question string) []string {
	q := strings.ToLower(question)
	hits := make(map[string]struct{})

	for _, g := range synonymGroups {
		if matchesAny(q, g.aliases) || strings.Contains(q, g.canonical) {
			hits[g.canonical] = struct{}{}
		}
	}

	out := make([]string, 0, len(hits))
	for k := range hits {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func matchesAny(q string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(q, n) {
			return true
		}
	}
	return false
}

// ExpandQuery returns an augmented retrieval string of the form:
//
//	<original question>  <canonical1> <canonical2> ...
//
// This is what feeds the BM25/vector retriever. The LLM still only sees
// the user's original wording.
func ExpandQuery(question string) string {
	exp := ExpandSynonyms(question)
	if len(exp) == 0 {
		return question
	}
	return strings.TrimSpace(question) + " " + strings.Join(exp, " ")
}
