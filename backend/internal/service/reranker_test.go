package service

import (
	"testing"

	"github.com/cfo/backend/internal/model"
)

func TestRerank_PeriodBoostWins(t *testing.T) {
	r := NewReranker(DefaultRerankWeights())

	// Two chunks with equal base score. One has a matching period.
	in := []EvidenceChunk{
		{ID: "no-period", Score: 0.1, Text: "a"},
		{
			ID:          "in-period",
			Score:       0.1,
			Text:        "a",
			PeriodStart: "2024-01-01",
			PeriodEnd:   "2024-03-31",
		},
	}
	out := r.Rerank(in, RerankSignals{
		PeriodStart: "2024-01-01",
		PeriodEnd:   "2024-03-31",
	}, 5)

	if out[0].ID != "in-period" {
		t.Fatalf("expected in-period to win, got %v", idsOf(out))
	}
}

func TestRerank_IndustryAndDocTypeStack(t *testing.T) {
	r := NewReranker(DefaultRerankWeights())

	in := []EvidenceChunk{
		{ID: "neither", Score: 0.1, Text: "a"},
		{ID: "industry-only", Score: 0.1, Text: "a", Industry: model.IndustryEducation},
		{ID: "doctype-only", Score: 0.1, Text: "a", DocType: model.DocType("P&L")},
		{ID: "both", Score: 0.1, Text: "a", Industry: model.IndustryEducation, DocType: model.DocType("P&L")},
	}
	out := r.Rerank(in, RerankSignals{
		Industry: model.IndustryEducation,
		DocType:  model.DocType("P&L"),
	}, 5)

	if out[0].ID != "both" {
		t.Fatalf("expected 'both' first, got %v", idsOf(out))
	}
	if out[len(out)-1].ID != "neither" {
		t.Fatalf("expected 'neither' last, got %v", idsOf(out))
	}
}

func TestRerank_PrefersKnownSources(t *testing.T) {
	r := NewReranker(DefaultRerankWeights())
	in := []EvidenceChunk{
		{ID: "a", Score: 0.1, Source: "random.pdf", Text: "a"},
		{ID: "b", Score: 0.1, Source: "pnl_q1.xlsx", Text: "a"},
	}
	out := r.Rerank(in, RerankSignals{PreferredSources: []string{"pnl_q1.xlsx"}}, 5)
	if out[0].ID != "b" {
		t.Fatalf("expected preferred-source chunk first, got %v", idsOf(out))
	}
}

func TestRerank_LengthPenalty(t *testing.T) {
	r := NewReranker(DefaultRerankWeights())
	long := make([]byte, LengthPenaltyChars+1)
	for i := range long {
		long[i] = 'a'
	}
	in := []EvidenceChunk{
		{ID: "short", Score: 0.1, Text: "hi"},
		{ID: "long", Score: 0.1, Text: string(long)},
	}
	out := r.Rerank(in, RerankSignals{}, 5)
	if out[0].ID != "short" {
		t.Fatalf("expected short chunk first after length penalty, got %v", idsOf(out))
	}
}

func TestRerank_RespectsTopK(t *testing.T) {
	r := NewReranker(DefaultRerankWeights())
	in := make([]EvidenceChunk, 10)
	for i := range in {
		in[i] = EvidenceChunk{ID: string(rune('a' + i)), Score: float32(i) * 0.01, Text: "x"}
	}
	out := r.Rerank(in, RerankSignals{}, 3)
	if len(out) != 3 {
		t.Fatalf("expected 3, got %d", len(out))
	}
}
