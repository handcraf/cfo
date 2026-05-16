// Package service — local LLM runtime via llama.cpp.
//
// This module replaces the previous Ollama HTTP client. The runtime now
// shells out to a locally built `llama.cpp` binary with a Gemma GGUF
// model file. Goals:
//
//   - Fully offline. No HTTP, no daemon, no cloud APIs.
//   - Deterministic. Temperature 0.2 + top_p 0.9 + a fixed seed keep
//     repeated asks producing identical explanations, which matters for
//     the ask_audit log.
//   - Auditable. The exact command + prompt that produced an answer is
//     traceable from logs.
//
// Architectural rule (unchanged): "Backend decides facts. LLM explains
// facts." The runtime swap does NOT touch the ask pipeline contract.
//
// TODO: Optimization — reuse a `llama-server` process instead of forking
// a fresh `main` per question. Cold start dominates latency today.
// TODO: Support a direct GGUF reader (purego) so we drop the shell-out
// entirely. Needs benchmarking first.
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LLMExplanation is the structured output the ask handler consumes.
// Shape is UNCHANGED from the Ollama version so callers don't churn.
type LLMExplanation struct {
	Summary string
	Detail  string
}

// LLMConfig is the knob set for the local llama.cpp runtime. All fields
// have safe defaults via DefaultLLMConfig; callers usually only set
// Binary and ModelPath.
type LLMConfig struct {
	// Binary is the path to the llama.cpp executable (`main` / `llama-cli`).
	// Default: "./llama.cpp/main".
	Binary string

	// ModelPath is the path to the Gemma GGUF model file on disk.
	// Example: "./models/gemma-7b-q4.gguf".
	ModelPath string

	// MaxTokens caps generation length. Default 512 — enough for the
	// SUMMARY + EXPLANATION block without blowing out the context.
	MaxTokens int

	// Temperature is the sampling temperature. Default 0.2 for
	// near-deterministic explanations.
	Temperature float32

	// TopP is nucleus sampling. Default 0.9.
	TopP float32

	// Seed fixes the RNG. Default 42. Set to -1 to disable.
	Seed int

	// ContextSize caps the effective context. Default 4096.
	ContextSize int

	// Timeout is the hard wall-clock limit per generation. Default 120s.
	Timeout time.Duration

	// Threads caps CPU threads. Default 0 (llama.cpp chooses).
	Threads int

	// ExtraArgs are appended verbatim to the llama.cpp command line.
	// Use sparingly; these bypass the typed fields above.
	ExtraArgs []string
}

// DefaultLLMConfig returns a ready-to-use config with deterministic
// sampling and a conservative generation budget.
func DefaultLLMConfig() LLMConfig {
	return LLMConfig{
		Binary:      "./llama.cpp/main",
		ModelPath:   "./models/gemma.gguf",
		MaxTokens:   512,
		Temperature: 0.2,
		TopP:        0.9,
		Seed:        42,
		ContextSize: 4096,
		Timeout:     120 * time.Second,
	}
}

// promptRunner is the seam between the deterministic prompt-building
// logic and the concrete execution strategy. Tests inject a fake;
// production uses llamaCppRunner.
type promptRunner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

// LLMService is the public facade the ask handler uses. It is safe for
// concurrent use; each Run spawns its own process.
type LLMService struct {
	cfg    LLMConfig
	runner promptRunner
}

// NewLLMService constructs a service backed by a real llama.cpp process.
// It does NOT probe the binary — construction is cheap and offline-safe.
// Call HealthCheck if you want early validation.
func NewLLMService(cfg LLMConfig) *LLMService {
	applyLLMDefaults(&cfg)
	return &LLMService{
		cfg:    cfg,
		runner: &llamaCppRunner{cfg: cfg},
	}
}

// newLLMServiceWithRunner is used by tests. Not exported.
func newLLMServiceWithRunner(cfg LLMConfig, r promptRunner) *LLMService {
	applyLLMDefaults(&cfg)
	return &LLMService{cfg: cfg, runner: r}
}

func applyLLMDefaults(cfg *LLMConfig) {
	def := DefaultLLMConfig()
	if cfg.Binary == "" {
		cfg.Binary = def.Binary
	}
	if cfg.ModelPath == "" {
		cfg.ModelPath = def.ModelPath
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = def.MaxTokens
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = def.Temperature
	}
	if cfg.TopP == 0 {
		cfg.TopP = def.TopP
	}
	if cfg.Seed == 0 {
		cfg.Seed = def.Seed
	}
	if cfg.ContextSize == 0 {
		cfg.ContextSize = def.ContextSize
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = def.Timeout
	}
}

// ExplainMetrics asks the LLM to narrate pre-computed financial facts.
// The signature is intentionally unchanged from the Ollama version so
// callers (ask.go) don't have to move.
//
// IMPORTANT: This function never accepts a request the LLM should
// COMPUTE. All numbers must have been produced upstream by
// financial_logic.go or an industry module.
func (l *LLMService) ExplainMetrics(question string, numbers []string, context string) (*LLMExplanation, error) {
	prompt := l.buildPrompt(question, numbers, context)

	ctx, cancel := contextWithTimeoutBackground(l.cfg.Timeout)
	defer cancel()

	raw, err := l.runner.Run(ctx, prompt)
	if err != nil {
		return nil, err
	}
	return l.parseExplanation(raw), nil
}

// contextWithTimeoutBackground exists as a tiny indirection so tests can
// hand-craft cancellation without touching the public API.
func contextWithTimeoutBackground(d time.Duration) (context.Context, context.CancelFunc) {
	return ctxWithTimeout(d)
}

var ctxWithTimeout = func(d time.Duration) (context.Context, context.CancelFunc) {
	return contextTimeout(d)
}

func contextTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

// buildPrompt renders the CFO system prompt. The structure mirrors the
// previous Ollama prompt so answers stay stylistically consistent.
// Context is hard-capped so we don't overflow Gemma's context window.
func (l *LLMService) buildPrompt(question string, numbers []string, context string) string {
	numbersStr := strings.Join(numbers, "\n- ")
	if numbersStr == "" {
		numbersStr = "(no numbers available)"
	} else {
		numbersStr = "- " + numbersStr
	}

	// Reserve roughly half the context window for the prompt; the rest
	// is for generation. Gemma tokenizes slightly differently from
	// LLaMA3 but 4 chars/token is a conservative rule of thumb.
	maxContextChars := (l.cfg.ContextSize * 4) / 2
	if maxContextChars > 20000 {
		maxContextChars = 20000
	}
	if len(context) > maxContextChars {
		context = context[:maxContextChars] + "\n... [Context truncated for token budget]"
	}

	return fmt.Sprintf(`You are an AI CFO assistant. Your ONLY job is to EXPLAIN the financial numbers that the backend has already computed. You are a narrator, not an analyst, not a calculator.

================== HARD CONSTRAINTS ==================
1. NEVER invent or recompute numbers. If a number is not in CALCULATED METRICS, treat it as unavailable.
2. NEVER answer questions outside finance/CFO scope. The backend already filters these — if you see one, refuse politely.
3. NEVER hallucinate document names, periods, or sources. Only cite what's in DOCUMENT EXCERPTS.
4. If the data needed for the question is missing, say so explicitly. Better to say "data not available" than to guess.
5. When you use a number from CALCULATED METRICS, mention it verbatim (same value, same currency).
6. When you cite evidence, refer to it by its tag like [E1], [E2] as shown in the excerpts.

================== CALCULATED METRICS ==================
(These come from verified SQL sources. Do NOT recompute or aggregate them.)
%s

================== DOCUMENT EXCERPTS ==================
(Retrieved evidence chunks. Each is tagged [E1], [E2], etc. Cite by tag.)
%s

================== USER QUESTION ==================
%s

================== INSTRUCTIONS ==================
1. Write a brief 1-2 sentence SUMMARY that directly answers the question, citing the relevant metric value.
2. Write an EXPLANATION that:
   - References specific evidence by tag, e.g. "as shown in [E1]"
   - Explains what the numbers mean for business health
   - Notes any caveats from the data (period mismatch, missing values, conflicting evidence)
3. If the question asks for something the CALCULATED METRICS don't contain, your SUMMARY must say "The data needed for this is not available." and your EXPLANATION must explain what would be needed.
4. Use plain business language. No jargon dumps. No filler.

================== OUTPUT FORMAT (strict) ==================
SUMMARY: <one or two sentences answering the question>

EXPLANATION: <details with [E#] citations and caveats>`, numbersStr, context, question)
}

// parseExplanation lifts the runtime's stdout into a structured result.
// Logic is identical to the Ollama parser — Gemma and LLaMA3 both
// follow the SUMMARY:/EXPLANATION: contract reliably at temp 0.2.
func (l *LLMService) parseExplanation(response string) *LLMExplanation {
	explanation := &LLMExplanation{}

	lines := strings.Split(response, "\n")
	var summaryLines []string
	var explanationLines []string

	inSummary := false
	inExplanation := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.HasPrefix(strings.ToUpper(trimmed), "SUMMARY:"):
			inSummary = true
			inExplanation = false
			rest := stripAnyPrefix(trimmed, "SUMMARY:", "Summary:", "summary:")
			if v := strings.TrimSpace(rest); v != "" {
				summaryLines = append(summaryLines, v)
			}
		case strings.HasPrefix(strings.ToUpper(trimmed), "EXPLANATION:"):
			inSummary = false
			inExplanation = true
			rest := stripAnyPrefix(trimmed, "EXPLANATION:", "Explanation:", "explanation:")
			if v := strings.TrimSpace(rest); v != "" {
				explanationLines = append(explanationLines, v)
			}
		case trimmed != "":
			if inSummary {
				summaryLines = append(summaryLines, trimmed)
			} else if inExplanation {
				explanationLines = append(explanationLines, trimmed)
			}
		}
	}

	explanation.Summary = strings.Join(summaryLines, " ")
	explanation.Detail = strings.Join(explanationLines, "\n")

	// Fallback if the model ignored our format directives.
	if explanation.Summary == "" && explanation.Detail == "" {
		sentences := strings.SplitN(response, ".", 2)
		if len(sentences) > 0 {
			explanation.Summary = strings.TrimSpace(sentences[0]) + "."
		}
		if len(sentences) > 1 {
			explanation.Detail = strings.TrimSpace(sentences[1])
		}
	}

	return explanation
}

func stripAnyPrefix(s string, prefixes ...string) string {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return strings.TrimPrefix(s, p)
		}
	}
	return s
}

// HealthCheck verifies the binary exists, is executable, and that the
// model file is readable. Does NOT actually run inference — that would
// be expensive for a liveness probe.
func (l *LLMService) HealthCheck() error {
	if l.cfg.Binary == "" {
		return errors.New("llm: empty binary path")
	}
	if _, err := exec.LookPath(l.cfg.Binary); err != nil {
		// Fall back to checking the path directly in case it's relative.
		if info, statErr := os.Stat(l.cfg.Binary); statErr != nil {
			return fmt.Errorf("llm: binary not found at %s: %w", l.cfg.Binary, statErr)
		} else if info.IsDir() {
			return fmt.Errorf("llm: binary path is a directory: %s", l.cfg.Binary)
		} else if info.Mode()&0o111 == 0 {
			return fmt.Errorf("llm: binary not executable: %s", l.cfg.Binary)
		}
	}
	if l.cfg.ModelPath == "" {
		return errors.New("llm: empty model path")
	}
	if _, err := os.Stat(l.cfg.ModelPath); err != nil {
		return fmt.Errorf("llm: model file unreadable at %s: %w", l.cfg.ModelPath, err)
	}
	return nil
}

// ============================================================================
// llamaCppRunner — production implementation of promptRunner.
// ============================================================================

type llamaCppRunner struct {
	cfg LLMConfig
}

// Run writes the prompt to a temp file and invokes llama.cpp. The prompt
// goes via `-f <file>` because command-line argument escaping breaks on
// multi-kilobyte prompts with quotes/newlines.
//
// On timeout, the context is cancelled and the process is killed.
func (r *llamaCppRunner) Run(ctx context.Context, prompt string) (string, error) {
	promptFile, err := writePromptFile(prompt)
	if err != nil {
		return "", fmt.Errorf("llm: write prompt file: %w", err)
	}
	defer os.Remove(promptFile)

	args := buildLlamaCppArgs(r.cfg, promptFile)
	cmd := exec.CommandContext(ctx, r.cfg.Binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Printf("[LLM] invoke: %s %s", r.cfg.Binary, strings.Join(args, " "))
	err = cmd.Run()
	if err != nil {
		// Context deadline exceeded manifests as a non-zero exit.
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("llm: generation timed out after %s", r.cfg.Timeout)
		}
		return "", fmt.Errorf("llm: execution failed: %w (stderr: %s)", err, trimForLog(stderr.String()))
	}
	out := stdout.String()
	return cleanLlamaCppOutput(out), nil
}

// buildLlamaCppArgs composes the llama.cpp command line. Flags follow the
// `main` / `llama-cli` CLI. `--no-display-prompt` keeps stdout to the
// generated tokens only, which makes parsing predictable.
//
// TODO: Once we move to `llama-server`, this whole function goes away
// and we talk HTTP to the local process.
func buildLlamaCppArgs(cfg LLMConfig, promptFile string) []string {
	args := []string{
		"-m", cfg.ModelPath,
		"-f", promptFile,
		"-n", itoa(cfg.MaxTokens),
		"--temp", ftoa(cfg.Temperature),
		"--top-p", ftoa(cfg.TopP),
		"-c", itoa(cfg.ContextSize),
		"--no-display-prompt",
		// -no-cnv is REQUIRED. Models that ship a chat template (Gemma,
		// LLaMA-3, Mistral-instruct, …) auto-enable conversation mode in
		// modern llama.cpp builds, which causes the process to hang on
		// an interactive `>` prompt after generation. -no-cnv forces
		// single-shot completion mode and clean exit.
		"-no-cnv",
	}
	if cfg.Seed > 0 {
		args = append(args, "--seed", itoa(cfg.Seed))
	}
	if cfg.Threads > 0 {
		args = append(args, "-t", itoa(cfg.Threads))
	}
	args = append(args, cfg.ExtraArgs...)
	return args
}

// cleanLlamaCppOutput strips the trailing end-of-text markers some
// llama.cpp builds emit after the generated text.
func cleanLlamaCppOutput(s string) string {
	s = strings.ReplaceAll(s, "<|endoftext|>", "")
	s = strings.ReplaceAll(s, "<end_of_turn>", "") // Gemma chat template marker
	s = strings.ReplaceAll(s, "[end of text]", "")
	return strings.TrimSpace(s)
}

// writePromptFile persists the prompt to a unique temp file.
func writePromptFile(prompt string) (string, error) {
	f, err := os.CreateTemp("", "cfo-llm-prompt-*.txt")
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(prompt); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func trimForLog(s string) string {
	const max = 400
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max] + "...[truncated]"
	}
	return s
}

// tiny formatters, kept local so this file has no strconv import noise
func itoa(n int) string    { return fmt.Sprintf("%d", n) }
func ftoa(f float32) string { return fmt.Sprintf("%.3f", f) }

// PromptFilePath is a test-only accessor: it returns nothing useful in
// production but callers using it in-process (e.g., a future server
// variant) may find it handy to inspect the resolved temp directory.
func PromptFilePath() string { return filepath.Join(os.TempDir(), "cfo-llm-prompt") }
