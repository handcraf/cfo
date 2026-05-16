package service

import (
	"strings"
	"testing"
)

func TestExpandSynonyms_Profit(t *testing.T) {
	got := ExpandSynonyms("How much profit did we make?")
	if !containsString(got, "net income") {
		t.Errorf("expected 'net income' in expansion of 'profit', got %v", got)
	}
}

func TestExpandSynonyms_BurnAndRunway(t *testing.T) {
	got := ExpandSynonyms("How long is our cash burn runway?")
	for _, want := range []string{"cash", "monthly burn", "runway"} {
		if !containsString(got, want) {
			t.Errorf("expected %q in expansion, got %v", want, got)
		}
	}
}

func TestExpandSynonyms_NoMatch(t *testing.T) {
	got := ExpandSynonyms("xyzzy quantum spaghetti")
	if len(got) != 0 {
		t.Errorf("expected empty expansion for nonsense input, got %v", got)
	}
}

func TestExpandSynonyms_Deterministic(t *testing.T) {
	q := "What was our revenue, profit, and cash position?"
	a := ExpandSynonyms(q)
	b := ExpandSynonyms(q)
	if !equalSlices(a, b) {
		t.Errorf("non-deterministic expansion: a=%v b=%v", a, b)
	}
}

func TestExpandQuery_AppendsCanonicalTerms(t *testing.T) {
	q := "How is our burn?"
	out := ExpandQuery(q)
	if !strings.Contains(out, "How is our burn?") {
		t.Errorf("ExpandQuery dropped the original question: %q", out)
	}
	if !strings.Contains(out, "monthly burn") {
		t.Errorf("ExpandQuery did not add canonical term 'monthly burn': %q", out)
	}
}

func TestExpandQuery_NoMatchReturnsOriginal(t *testing.T) {
	q := "Some random sentence"
	out := ExpandQuery(q)
	if out != q {
		t.Errorf("ExpandQuery should be a no-op for no-match input, got %q", out)
	}
}

// helpers

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
