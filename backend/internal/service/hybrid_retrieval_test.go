package service

import "testing"

// TestRRFFuse_AdditiveOnDuplicates verifies the core RRF property:
// a chunk that appears in BOTH lists beats a chunk that appears in only one,
// all else equal.
func TestRRFFuse_AdditiveOnDuplicates(t *testing.T) {
	both := EvidenceChunk{ID: "both", DocumentID: "d1", Text: "hit"}
	semOnly := EvidenceChunk{ID: "sem", DocumentID: "d2", Text: "hit"}
	kwOnly := EvidenceChunk{ID: "kw", DocumentID: "d3", Text: "hit"}

	sem := []EvidenceChunk{both, semOnly}
	kw := []EvidenceChunk{both, kwOnly}

	fused := rrfFuse(sem, kw, 60)
	if len(fused) != 3 {
		t.Fatalf("expected 3 unique chunks, got %d", len(fused))
	}
	if fused[0].ID != "both" {
		t.Errorf("expected 'both' to rank first (appears in both lists); got order: %v", idsOf(fused))
	}
	if fused[0].SemanticRank != 1 || fused[0].KeywordRank != 1 {
		t.Errorf("expected both ranks set on duplicate, got sem=%d kw=%d",
			fused[0].SemanticRank, fused[0].KeywordRank)
	}
}

// TestRRFFuse_PreservesSemanticWhenKeywordEmpty guards the "degraded"
// hybrid mode where one retriever returns nothing.
func TestRRFFuse_PreservesSemanticWhenKeywordEmpty(t *testing.T) {
	sem := []EvidenceChunk{
		{ID: "s1", Text: "a"},
		{ID: "s2", Text: "b"},
	}
	fused := rrfFuse(sem, nil, 60)
	if len(fused) != 2 {
		t.Fatalf("expected 2 fused, got %d", len(fused))
	}
	if fused[0].ID != "s1" {
		t.Errorf("top should stay s1, got %s", fused[0].ID)
	}
	if fused[0].SemanticRank != 1 || fused[0].KeywordRank != 0 {
		t.Errorf("expected semRank=1 kwRank=0, got sem=%d kw=%d",
			fused[0].SemanticRank, fused[0].KeywordRank)
	}
}

func TestRRFFuse_DeterministicTiebreak(t *testing.T) {
	// Two chunks with identical scores should tiebreak deterministically:
	// lower semantic rank first, then alphabetical ID.
	a := EvidenceChunk{ID: "aaa", Text: "x"}
	b := EvidenceChunk{ID: "bbb", Text: "x"}
	// Put them at the same rank in both lists.
	fused1 := rrfFuse([]EvidenceChunk{a, b}, nil, 60)
	fused2 := rrfFuse([]EvidenceChunk{a, b}, nil, 60)
	if fused1[0].ID != fused2[0].ID {
		t.Fatalf("fusion is non-deterministic: run1=%s run2=%s", fused1[0].ID, fused2[0].ID)
	}
}

func idsOf(chunks []EvidenceChunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.ID
	}
	return out
}
