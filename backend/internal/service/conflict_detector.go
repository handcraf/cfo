// Package service — conflict detection over retrieved evidence.
//
// A conflict is two pieces of evidence that purport to describe the same
// thing but disagree. In a financial context, the canonical case is:
//
//	Chunk A (source: "Q1_2024_PnL.xlsx") says "Revenue: $1,200,000"
//	Chunk B (source: "Q1_2024_investor_deck.pdf") says "Revenue: $1,450,000"
//
// A CFO answer backed by disagreeing evidence must not be reported with
// high confidence. The detector surfaces these so the ask flow can:
//   - return "uncertain" instead of a confident number
//   - include the conflicting chunks in the response for audit
//
// This is deliberately a simple heuristic. It does NOT try to understand
// semantics — it looks for structurally similar labels with materially
// different numbers. Precision over recall: we'd rather miss a subtle
// conflict than invent fake ones.
//
// TODO (temporal): Detect when the same document is cited under conflicting
// periods (e.g., a chunk labeled "Q1 2024" but with period metadata Q2 2024).
// That requires a richer chunk-parse than we have today.
package service

import (
	"fmt"
	"regexp"
	"strings"
)

// Conflict describes two (or more) pieces of evidence that disagree.
type Conflict struct {
	// Metric is the normalized label the values all claimed to describe
	// (e.g., "revenue", "cash", "total_assets").
	Metric string `json:"metric"`

	// Values are the distinct numeric values found for this metric.
	Values []ConflictValue `json:"values"`

	// SpreadPct is |max-min| / max * 100 — how far apart the values are.
	// Large spreads (>5%) are the ones the detector flags.
	SpreadPct float64 `json:"spread_pct"`
}

// ConflictValue ties a numeric claim back to its source chunk for audit.
type ConflictValue struct {
	Value   float64 `json:"value"`
	ChunkID string  `json:"chunk_id"`
	Source  string  `json:"source"`
}

// ConflictDetector configuration. Zero value is not usable; use
// NewConflictDetector.
type ConflictDetector struct {
	// MinSpreadPct is the minimum relative spread (percent) between the
	// smallest and largest value that triggers a conflict. 5% is a good
	// default for financial numbers — tighter risks flagging rounding
	// differences as conflicts.
	MinSpreadPct float64
}

// NewConflictDetector returns a detector with sensible defaults.
func NewConflictDetector() *ConflictDetector {
	return &ConflictDetector{MinSpreadPct: 5.0}
}

// metricPatterns maps regex patterns that identify a metric mention to
// the canonical metric label. Order matters: first match wins. Patterns
// are case-insensitive and allow common separators between label and value.
//
// Intentionally conservative — we only try to catch metrics we also
// compute in financial_logic.go. Unknown metrics get ignored.
var metricPatterns = []struct {
	metric string
	re     *regexp.Regexp
}{
	// cash / cash balance / cash position
	{"cash", regexp.MustCompile(`(?i)\b(?:cash(?:\s+(?:balance|position|on\s+hand))?)\b`)},
	// revenue / sales / turnover
	{"revenue", regexp.MustCompile(`(?i)\b(?:revenue|total\s+revenue|net\s+sales|sales|turnover)\b`)},
	// expenses / operating expenses
	{"expenses", regexp.MustCompile(`(?i)\b(?:operating\s+expenses|opex|total\s+expenses|expenses)\b`)},
	// net income / net profit / loss
	{"net_income", regexp.MustCompile(`(?i)\b(?:net\s+(?:income|profit|earnings|loss))\b`)},
	// total assets
	{"total_assets", regexp.MustCompile(`(?i)\btotal\s+assets\b`)},
	// total liabilities
	{"total_liabilities", regexp.MustCompile(`(?i)\btotal\s+liabilities\b`)},
	// equity / shareholders equity
	{"equity", regexp.MustCompile(`(?i)\b(?:shareholders?'?\s+)?equity\b`)},
}

// numberRe matches numbers like 1,234,567.89 / $1.2M / 1.5 million / 2B.
// We accept magnitude suffixes but normalize in parseNumber.
var numberRe = regexp.MustCompile(`\$?([0-9]{1,3}(?:,[0-9]{3})+|[0-9]+(?:\.[0-9]+)?)(?:\s*(k|K|m|M|b|B|thousand|million|billion))?`)

// Detect scans the chunks for numeric claims on known metrics and reports
// every metric that has materially different values across chunks.
//
// Time complexity: O(N * P) where N = chunks, P = patterns. Fine at the
// top-K=5 to top-K=20 range we operate at.
func (d *ConflictDetector) Detect(chunks []EvidenceChunk) []Conflict {
	// metric → list of claimed values (with source)
	claims := make(map[string][]ConflictValue)

	for _, ch := range chunks {
		text := ch.Text
		for _, p := range metricPatterns {
			locs := p.re.FindAllStringIndex(text, -1)
			if len(locs) == 0 {
				continue
			}
			for _, loc := range locs {
				// Look for a number in the ±80 chars window around the label.
				start := loc[1]
				end := loc[1] + 80
				if end > len(text) {
					end = len(text)
				}
				window := text[start:end]
				val, ok := extractNumber(window)
				if !ok {
					// Also check backwards a bit — sometimes the number
					// precedes the label ("$1.2M in revenue").
					bstart := loc[0] - 80
					if bstart < 0 {
						bstart = 0
					}
					val, ok = extractNumber(text[bstart:loc[0]])
					if !ok {
						continue
					}
				}
				claims[p.metric] = append(claims[p.metric], ConflictValue{
					Value:   val,
					ChunkID: ch.ID,
					Source:  ch.Source,
				})
			}
		}
	}

	var conflicts []Conflict
	for metric, values := range claims {
		// Per-source dedup: a single CSV/PDF often lists sub-categories
		// ("Recurring Revenue", "Service Revenue", "Total Revenue") and
		// our regex catches them all. Treating those as conflicts is a
		// false positive — they're hierarchical sub-totals. We keep the
		// LARGEST value per (metric, source) pair, which approximates
		// "the totals row" without parsing line-item semantics.
		bySource := make(map[string]ConflictValue)
		for _, v := range values {
			key := v.Source
			if key == "" {
				key = v.ChunkID
			}
			cur, ok := bySource[key]
			if !ok || v.Value > cur.Value {
				bySource[key] = v
			}
		}
		if len(bySource) < 2 {
			// Only one source claimed this metric — not a cross-source
			// disagreement, can't be a conflict.
			continue
		}
		// Now apply the legacy dedupe + spread check across sources.
		perSource := make([]ConflictValue, 0, len(bySource))
		for _, v := range bySource {
			perSource = append(perSource, v)
		}
		distinct := dedupeValues(perSource)
		if len(distinct) < 2 {
			continue
		}
		spread := spreadPercent(distinct)
		if spread < d.MinSpreadPct {
			continue
		}
		conflicts = append(conflicts, Conflict{
			Metric:    metric,
			Values:    distinct,
			SpreadPct: spread,
		})
	}
	return conflicts
}

// extractNumber parses the first number mention out of a text window and
// normalizes magnitude suffixes (K/M/B). Returns false when nothing found.
func extractNumber(s string) (float64, bool) {
	m := numberRe.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	raw := strings.ReplaceAll(m[1], ",", "")
	val, err := parseFloat(raw)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(m[2]) {
	case "k", "thousand":
		val *= 1e3
	case "m", "million":
		val *= 1e6
	case "b", "billion":
		val *= 1e9
	}
	return val, true
}

// parseFloat wraps strconv.ParseFloat to keep the import local to this file.
func parseFloat(s string) (float64, error) {
	var v float64
	_, err := fmt.Sscanf(s, "%f", &v)
	return v, err
}

// dedupeValues folds values that agree within 0.5% of each other. This
// tolerates rounding noise (e.g., $1,234,567 vs $1,234,568) without hiding
// material disagreement.
func dedupeValues(vals []ConflictValue) []ConflictValue {
	const tolerance = 0.005
	out := make([]ConflictValue, 0, len(vals))
	for _, v := range vals {
		dup := false
		for _, existing := range out {
			if existing.Value == 0 {
				continue
			}
			diff := (v.Value - existing.Value) / existing.Value
			if diff < 0 {
				diff = -diff
			}
			if diff < tolerance {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, v)
		}
	}
	return out
}

// spreadPercent returns |max-min|/|max| * 100 over distinct values.
// Returns 0 on singleton or all-zero inputs.
func spreadPercent(vals []ConflictValue) float64 {
	if len(vals) < 2 {
		return 0
	}
	min, max := vals[0].Value, vals[0].Value
	for _, v := range vals[1:] {
		if v.Value < min {
			min = v.Value
		}
		if v.Value > max {
			max = v.Value
		}
	}
	if max == 0 {
		return 0
	}
	diff := max - min
	if diff < 0 {
		diff = -diff
	}
	ref := max
	if ref < 0 {
		ref = -ref
	}
	return (diff / ref) * 100
}
