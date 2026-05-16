package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRunner is a deterministic stand-in for llama.cpp. It records the
// last prompt it saw and returns whatever the author configured.
type fakeRunner struct {
	response   string
	err        error
	lastPrompt string
	delay      time.Duration
}

func (f *fakeRunner) Run(ctx context.Context, prompt string) (string, error) {
	f.lastPrompt = prompt
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

// ----------------------------------------------------------------------
// Prompt construction
// ----------------------------------------------------------------------

func TestLLM_BuildPrompt_ContainsCoreElements(t *testing.T) {
	l := NewLLMService(LLMConfig{})
	question := "What is our cash position?"
	numbers := []string{"Cash: $500,000", "Runway: 10 months"}
	context := "Revenue: $1,000,000"

	p := l.buildPrompt(question, numbers, context)

	for _, must := range []string{question, "Cash: $500,000", "Runway: 10 months", "Revenue: $1,000,000", "SUMMARY", "EXPLANATION", "NEVER"} {
		if !strings.Contains(p, must) {
			t.Errorf("prompt missing %q", must)
		}
	}
}

func TestLLM_BuildPrompt_TruncatesOversizedContext(t *testing.T) {
	l := NewLLMService(LLMConfig{ContextSize: 512})
	huge := strings.Repeat("x", 100_000)
	p := l.buildPrompt("q", []string{"Cash: $1"}, huge)
	if len(p) >= 100_000 {
		t.Fatalf("prompt should be truncated; got len=%d", len(p))
	}
	if !strings.Contains(p, "truncated") {
		t.Errorf("expected truncation marker in prompt")
	}
}

func TestLLM_BuildPrompt_NoNumbersStillSafe(t *testing.T) {
	l := NewLLMService(LLMConfig{})
	p := l.buildPrompt("hi", nil, "ctx")
	if !strings.Contains(p, "no numbers available") {
		t.Errorf("expected placeholder when numbers are empty; got: %s", p)
	}
}

// ----------------------------------------------------------------------
// Explanation parsing
// ----------------------------------------------------------------------

func TestLLM_ParseExplanation_Structured(t *testing.T) {
	l := NewLLMService(LLMConfig{})
	out := l.parseExplanation(`SUMMARY: Cash position is healthy at $500,000.

EXPLANATION: With a monthly burn of $50,000, runway is roughly 10 months.`)
	if !strings.Contains(out.Summary, "healthy") {
		t.Errorf("summary missing key phrase: %q", out.Summary)
	}
	if !strings.Contains(out.Detail, "runway") {
		t.Errorf("detail missing key phrase: %q", out.Detail)
	}
}

func TestLLM_ParseExplanation_UnstructuredFallback(t *testing.T) {
	l := NewLLMService(LLMConfig{})
	out := l.parseExplanation("Cash is $500K. Runway is 10 months.")
	if out.Summary == "" && out.Detail == "" {
		t.Fatal("expected fallback summary/detail to be populated")
	}
}

func TestLLM_ParseExplanation_Empty(t *testing.T) {
	l := NewLLMService(LLMConfig{})
	out := l.parseExplanation("")
	if out == nil {
		t.Fatal("nil result on empty input")
	}
}

func TestLLM_ParseExplanation_CaseInsensitiveHeaders(t *testing.T) {
	l := NewLLMService(LLMConfig{})
	out := l.parseExplanation(`summary: lower case.
explanation: also lower case.`)
	if out.Summary == "" || out.Detail == "" {
		t.Errorf("failed to parse lowercase headers: %+v", out)
	}
}

// ----------------------------------------------------------------------
// End-to-end with the fake runner (Steps 1-3 of the user acceptance set)
// ----------------------------------------------------------------------

func TestLLM_ExplainMetrics_ProfitQuestion(t *testing.T) {
	fr := &fakeRunner{response: `SUMMARY: Profit in Q1 was $150,000 — margins are improving.

EXPLANATION: Revenue minus expenses landed at $150K, up from the prior quarter; the pre-computed numbers show a 12% margin consistent with the evidence.`}
	l := newLLMServiceWithRunner(LLMConfig{}, fr)
	out, err := l.ExplainMetrics("What was the profit in Q1?",
		[]string{"Revenue: $1,250,000", "Expenses: $1,100,000", "Net income: $150,000"},
		"Q1 P&L excerpt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.Summary, "$150,000") {
		t.Errorf("summary missing key number: %q", out.Summary)
	}
	if !strings.Contains(fr.lastPrompt, "Net income: $150,000") {
		t.Errorf("runner did not receive pre-computed facts; got prompt: %s", fr.lastPrompt)
	}
}

func TestLLM_ExplainMetrics_MissingDataRefusal(t *testing.T) {
	// When upstream has no numbers, we still call the LLM; the contract
	// says it must refuse gracefully rather than fabricate.
	fr := &fakeRunner{response: `SUMMARY: Data not available.

EXPLANATION: The requested metric is not present in the uploaded documents; I cannot provide a number without verified SQL input.`}
	l := newLLMServiceWithRunner(LLMConfig{}, fr)
	out, err := l.ExplainMetrics("What was R&D spend last year?", nil, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(strings.ToLower(out.Summary+out.Detail), "not available") {
		t.Errorf("refusal not surfaced; got %+v", out)
	}
	// And the prompt must have told the model the facts are empty.
	if !strings.Contains(fr.lastPrompt, "no numbers available") {
		t.Errorf("empty-numbers placeholder missing from prompt: %s", fr.lastPrompt)
	}
}

func TestLLM_ExplainMetrics_LargeContextStable(t *testing.T) {
	fr := &fakeRunner{response: "SUMMARY: Stable.\n\nEXPLANATION: ok."}
	l := newLLMServiceWithRunner(LLMConfig{ContextSize: 2048}, fr)
	big := strings.Repeat("The quarterly report shows steady performance. ", 5000)
	out, err := l.ExplainMetrics("Summarize Q1", []string{"Revenue: $1M"}, big)
	if err != nil {
		t.Fatalf("unexpected error on large context: %v", err)
	}
	if out.Summary == "" {
		t.Errorf("expected non-empty summary on large context")
	}
	if len(fr.lastPrompt) > 30_000 {
		t.Errorf("prompt not truncated; length %d", len(fr.lastPrompt))
	}
}

func TestLLM_ExplainMetrics_RunnerErrorPropagates(t *testing.T) {
	fr := &fakeRunner{err: errors.New("boom")}
	l := newLLMServiceWithRunner(LLMConfig{}, fr)
	_, err := l.ExplainMetrics("q", []string{"x"}, "ctx")
	if err == nil {
		t.Fatal("expected error from failing runner")
	}
}

func TestLLM_ExplainMetrics_RunnerTimesOut(t *testing.T) {
	fr := &fakeRunner{response: "unused", delay: 50 * time.Millisecond}
	l := newLLMServiceWithRunner(LLMConfig{Timeout: 5 * time.Millisecond}, fr)
	_, err := l.ExplainMetrics("q", nil, "")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// ----------------------------------------------------------------------
// Command construction (production runner, without actually exec-ing)
// ----------------------------------------------------------------------

func TestBuildLlamaCppArgs_ContainsCoreFlags(t *testing.T) {
	cfg := LLMConfig{
		Binary: "./llama.cpp/main", ModelPath: "/m/gemma.gguf",
		MaxTokens: 256, Temperature: 0.2, TopP: 0.9, Seed: 42, ContextSize: 4096, Threads: 4,
	}
	args := buildLlamaCppArgs(cfg, "/tmp/prompt.txt")
	joined := strings.Join(args, " ")

	for _, want := range []string{"-m /m/gemma.gguf", "-f /tmp/prompt.txt", "-n 256", "--temp 0.200", "--top-p 0.900", "-c 4096", "--no-display-prompt", "-no-cnv", "--seed 42", "-t 4"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q; got: %s", want, joined)
		}
	}
}

func TestBuildLlamaCppArgs_OmitsSeedWhenDisabled(t *testing.T) {
	cfg := LLMConfig{Binary: "b", ModelPath: "m", MaxTokens: 1, Temperature: 0.1, TopP: 0.1, Seed: -1, ContextSize: 1}
	args := buildLlamaCppArgs(cfg, "/tmp/p.txt")
	if strings.Contains(strings.Join(args, " "), "--seed") {
		t.Errorf("seed flag should be omitted when Seed<=0; got %v", args)
	}
}

func TestCleanLlamaCppOutput_StripsMarkers(t *testing.T) {
	in := "SUMMARY: foo\n\nEXPLANATION: bar<end_of_turn>\n[end of text]"
	out := cleanLlamaCppOutput(in)
	if strings.Contains(out, "end_of_turn") || strings.Contains(out, "end of text") {
		t.Errorf("markers not stripped: %q", out)
	}
}

// ----------------------------------------------------------------------
// HealthCheck
// ----------------------------------------------------------------------

func TestHealthCheck_MissingBinary(t *testing.T) {
	l := NewLLMService(LLMConfig{Binary: "/definitely/not/a/real/binary", ModelPath: "/tmp"})
	if err := l.HealthCheck(); err == nil {
		t.Error("expected error for missing binary")
	}
}

func TestHealthCheck_HappyPath(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-llama")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("create fake binary: %v", err)
	}
	model := filepath.Join(dir, "gemma.gguf")
	if err := os.WriteFile(model, []byte("not-really-a-model"), 0o644); err != nil {
		t.Fatalf("create fake model: %v", err)
	}

	l := NewLLMService(LLMConfig{Binary: bin, ModelPath: model})
	if err := l.HealthCheck(); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
}

func TestHealthCheck_MissingModel(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-llama")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("create fake binary: %v", err)
	}
	l := NewLLMService(LLMConfig{Binary: bin, ModelPath: filepath.Join(dir, "missing.gguf")})
	if err := l.HealthCheck(); err == nil {
		t.Error("expected error for missing model")
	}
}
