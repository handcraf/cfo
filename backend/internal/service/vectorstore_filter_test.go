package service

import (
	"testing"

	"github.com/cfo/backend/internal/model"
)

func TestVectorFilter_IsZero(t *testing.T) {
	if !(VectorFilter{}).IsZero() {
		t.Fatal("empty VectorFilter must be zero")
	}
	if (VectorFilter{Industry: model.IndustryEducation}).IsZero() {
		t.Fatal("industry alone must not be zero")
	}
	if (VectorFilter{PeriodStart: "2024-01-01", PeriodEnd: "2024-03-31"}).IsZero() {
		t.Fatal("period range must not be zero")
	}
}

func TestMatchesFilter_IndustryGate(t *testing.T) {
	doc := VectorDocument{IndustryType: model.IndustryPharma}
	if matchesFilter(doc, VectorFilter{Industry: model.IndustryEducation}) {
		t.Fatal("industry mismatch must reject")
	}
	if !matchesFilter(doc, VectorFilter{Industry: model.IndustryPharma}) {
		t.Fatal("industry match must accept")
	}
	if !matchesFilter(doc, VectorFilter{}) {
		t.Fatal("empty filter must accept all")
	}
}

func TestMatchesFilter_PeriodOverlap(t *testing.T) {
	doc := VectorDocument{
		PeriodStart: "2024-01-01",
		PeriodEnd:   "2024-03-31",
	}
	cases := []struct {
		name   string
		f      VectorFilter
		expect bool
	}{
		{"exact", VectorFilter{PeriodStart: "2024-01-01", PeriodEnd: "2024-03-31"}, true},
		{"partial-left", VectorFilter{PeriodStart: "2023-12-01", PeriodEnd: "2024-01-15"}, true},
		{"partial-right", VectorFilter{PeriodStart: "2024-03-15", PeriodEnd: "2024-04-30"}, true},
		{"disjoint-before", VectorFilter{PeriodStart: "2023-10-01", PeriodEnd: "2023-12-31"}, false},
		{"disjoint-after", VectorFilter{PeriodStart: "2024-04-01", PeriodEnd: "2024-06-30"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := matchesFilter(doc, c.f)
			if got != c.expect {
				t.Errorf("want %v got %v", c.expect, got)
			}
		})
	}
}

func TestMatchesFilter_ChunkWithoutPeriodRejectedWhenPeriodAsked(t *testing.T) {
	doc := VectorDocument{IndustryType: model.IndustryEducation}
	f := VectorFilter{PeriodStart: "2024-01-01", PeriodEnd: "2024-03-31"}
	if matchesFilter(doc, f) {
		t.Fatal("chunk with no period must not match a period-scoped filter")
	}
}

func TestMatchesFilter_DocumentIDAllowList(t *testing.T) {
	doc := VectorDocument{DocumentID: "doc_42"}
	if matchesFilter(doc, VectorFilter{DocumentIDs: []string{"other"}}) {
		t.Fatal("document id not in allow list must be rejected")
	}
	if !matchesFilter(doc, VectorFilter{DocumentIDs: []string{"doc_42"}}) {
		t.Fatal("document id in allow list must be accepted")
	}
}
