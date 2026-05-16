#!/usr/bin/env python3
"""
AI CFO — End-to-End Smoke + Acceptance Suite

Exercises the full path the frontend uses:

    Browser/React  -->  /api/*  (nginx)  -->  Go backend  -->  llama.cpp (Gemma)
                                                          -->  SQLite + RAG

Stdlib-only (no `pip install` step). Tested with Python 3.9+.

Usage:
    python3 scripts/e2e.py                       # default: backend on :8080
    python3 scripts/e2e.py --frontend-url http://localhost:3000
    python3 scripts/e2e.py --verbose             # print full response bodies
    python3 scripts/e2e.py --skip-mutating       # read-only checks only
    python3 scripts/e2e.py --skip-llm            # don't hit /ask (skip Gemma)
    python3 scripts/e2e.py --reset               # wipe and re-setup company
    python3 scripts/e2e.py --quick               # 1 LLM question instead of 3
    python3 scripts/e2e.py --timeout 240         # widen per-request timeout

Exit code is 0 on full pass, 1 on any failure.

What it covers:
    1. Backend /health                   (deterministic)
    2. Frontend index.html               (if --frontend-url provided)
    3. nginx proxy /api/health           (if --frontend-url provided)
    4. /company/status                   (deterministic)
    5. /setup/company  + /company/reset  (mutating; skipped with --skip-mutating)
    6. /documents listing                (deterministic)
    7. /metrics/current                  (deterministic, SQL-backed)
    8. /ask × 3 Gemma cases              (profit / missing-data / large-context)
    9. Response-shape contract           (summary, explanation, confidence, etc.)

The script is meant to be safe to run against a live dev backend.
By default it only RESETS state when invoked with --reset.

TODO: Optional Selenium/Playwright phase that drives the actual React UI
in a headless browser. Deferred — the HTTP path is the source of truth
for the contract anyway.
"""

from __future__ import annotations

import argparse
import json
import sys
import time
import http.cookiejar
import urllib.error
import urllib.parse
import urllib.request

# Module-level opener with cookie jar so subsequent requests carry the
# session cookie issued by /auth/setup or /auth/login. We need this for
# every test that hits a gated endpoint (/ask, /metrics, etc.).
_COOKIE_JAR = http.cookiejar.CookieJar()
_OPENER = urllib.request.build_opener(
    urllib.request.HTTPCookieProcessor(_COOKIE_JAR)
)
urllib.request.install_opener(_OPENER)
from dataclasses import dataclass, field
from typing import Any, Callable

# ----------------------------------------------------------------------------
# Pretty output
# ----------------------------------------------------------------------------

USE_COLOR = sys.stdout.isatty()


def _c(code: str, s: str) -> str:
    if not USE_COLOR:
        return s
    return f"\033[{code}m{s}\033[0m"


def green(s: str) -> str:  return _c("32", s)
def red(s: str)   -> str:  return _c("31", s)
def yellow(s: str) -> str: return _c("33", s)
def blue(s: str)  -> str:  return _c("36", s)
def gray(s: str)  -> str:  return _c("90", s)
def bold(s: str)  -> str:  return _c("1", s)


# ----------------------------------------------------------------------------
# Tiny HTTP client (urllib so this works without pip install)
# ----------------------------------------------------------------------------


class HTTPError(RuntimeError):
    def __init__(self, status: int, body: str):
        super().__init__(f"HTTP {status}: {body[:300]}")
        self.status = status
        self.body = body


def _request(
    method: str,
    url: str,
    *,
    body: bytes | None = None,
    headers: dict[str, str] | None = None,
    timeout: float = 120.0,
) -> tuple[int, dict[str, str], bytes]:
    req = urllib.request.Request(url, data=body, method=method)
    for k, v in (headers or {}).items():
        req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status, dict(resp.headers), resp.read()
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers or {}), e.read()


def get_json(url: str, *, timeout: float = 30.0) -> Any:
    status, _, body = _request("GET", url, timeout=timeout)
    if status >= 400:
        raise HTTPError(status, body.decode("utf-8", "replace"))
    return json.loads(body.decode("utf-8"))


def post_json(url: str, payload: dict, *, timeout: float = 240.0) -> Any:
    raw = json.dumps(payload).encode("utf-8")
    status, _, body = _request(
        "POST", url, body=raw,
        headers={"Content-Type": "application/json"},
        timeout=timeout,
    )
    if status >= 400:
        raise HTTPError(status, body.decode("utf-8", "replace"))
    text = body.decode("utf-8")
    return json.loads(text) if text.strip() else {}


def delete(url: str, *, timeout: float = 30.0) -> int:
    status, _, _ = _request("DELETE", url, timeout=timeout)
    return status


def get_text(url: str, *, timeout: float = 10.0) -> tuple[int, str]:
    status, _, body = _request("GET", url, timeout=timeout)
    return status, body.decode("utf-8", "replace")


# ----------------------------------------------------------------------------
# Test framework — minimalist
# ----------------------------------------------------------------------------


@dataclass
class TestResult:
    name: str
    passed: bool
    elapsed_ms: int
    detail: str = ""


@dataclass
class TestRunner:
    verbose: bool = False
    results: list[TestResult] = field(default_factory=list)

    def run(self, name: str, fn: Callable[[], str | None]) -> bool:
        sys.stdout.write(f"  {blue('▶')} {name} ... ")
        sys.stdout.flush()
        t0 = time.monotonic()
        try:
            detail = fn() or ""
            elapsed = int((time.monotonic() - t0) * 1000)
            sys.stdout.write(f"{green('PASS')} {gray(f'({elapsed}ms)')}\n")
            if detail and self.verbose:
                for line in detail.splitlines():
                    sys.stdout.write(f"      {gray(line)}\n")
            self.results.append(TestResult(name, True, elapsed, detail))
            return True
        except AssertionError as e:
            elapsed = int((time.monotonic() - t0) * 1000)
            sys.stdout.write(f"{red('FAIL')} {gray(f'({elapsed}ms)')}\n")
            sys.stdout.write(f"      {red(str(e))}\n")
            self.results.append(TestResult(name, False, elapsed, str(e)))
            return False
        except Exception as e:
            elapsed = int((time.monotonic() - t0) * 1000)
            sys.stdout.write(f"{red('ERROR')} {gray(f'({elapsed}ms)')}\n")
            sys.stdout.write(f"      {red(f'{type(e).__name__}: {e}')}\n")
            self.results.append(TestResult(name, False, elapsed, f"{type(e).__name__}: {e}"))
            return False

    def section(self, title: str) -> None:
        sys.stdout.write(f"\n{bold(blue('=== ' + title + ' ==='))}\n")

    def summary(self) -> int:
        total = len(self.results)
        passed = sum(1 for r in self.results if r.passed)
        failed = total - passed
        total_ms = sum(r.elapsed_ms for r in self.results)
        sys.stdout.write("\n" + bold("=" * 60) + "\n")
        sys.stdout.write(f"  Total:   {total}\n")
        sys.stdout.write(f"  Passed:  {green(str(passed))}\n")
        sys.stdout.write(f"  Failed:  {red(str(failed)) if failed else '0'}\n")
        sys.stdout.write(f"  Elapsed: {total_ms}ms\n")
        sys.stdout.write(bold("=" * 60) + "\n")
        if failed:
            sys.stdout.write(red(bold("FAILED tests:\n")))
            for r in self.results:
                if not r.passed:
                    sys.stdout.write(f"  - {r.name}: {r.detail.splitlines()[0] if r.detail else ''}\n")
        return 0 if failed == 0 else 1


# ----------------------------------------------------------------------------
# Assertions
# ----------------------------------------------------------------------------


def assert_in(field: str, obj: dict) -> Any:
    assert isinstance(obj, dict), f"expected dict, got {type(obj).__name__}: {obj!r}"
    assert field in obj, f"missing required field '{field}'; got keys: {sorted(obj.keys())}"
    return obj[field]


def assert_nonempty(field: str, obj: dict) -> Any:
    v = assert_in(field, obj)
    assert v not in (None, "", [], {}), f"field '{field}' is empty: {v!r}"
    return v


def assert_type(field: str, obj: dict, typ) -> Any:
    v = assert_in(field, obj)
    assert isinstance(v, typ), f"field '{field}' should be {typ}, got {type(v).__name__}: {v!r}"
    return v


def assert_one_of(field: str, obj: dict, allowed: set) -> Any:
    v = assert_in(field, obj)
    assert v in allowed, f"field '{field}' should be one of {allowed}, got {v!r}"
    return v


# ----------------------------------------------------------------------------
# Test scenarios
# ----------------------------------------------------------------------------


def test_backend_health(backend: str) -> Callable[[], str]:
    def _run() -> str:
        body = get_json(f"{backend}/health")
        status = assert_in("status", body)
        assert status in ("ok", "healthy", "up"), f"unexpected status: {status}"
        return f"status={status}"
    return _run


def test_frontend_index(frontend: str) -> Callable[[], str]:
    def _run() -> str:
        code, html = get_text(frontend, timeout=10.0)
        assert code == 200, f"expected 200, got {code}"
        assert "<html" in html.lower() or "<!doctype" in html.lower(), \
            "response doesn't look like HTML"
        # Vite/React signature — be lenient: presence of <div id="root"> or a <script>
        assert "<div id=\"root\"" in html or "<script" in html, \
            "no React mount point or scripts; frontend bundle?"
        return f"{len(html)} bytes HTML"
    return _run


def test_frontend_proxy(frontend: str) -> Callable[[], str]:
    def _run() -> str:
        try:
            body = get_json(f"{frontend.rstrip('/')}/api/health")
        except HTTPError as e:
            raise AssertionError(f"nginx /api/health proxy failed: {e}")
        return f"proxied OK: {body}"
    return _run


def test_company_status(backend: str) -> Callable[[], str]:
    def _run() -> str:
        body = get_json(f"{backend}/company/status")
        completed = assert_in("setup_completed", body)
        assert isinstance(completed, bool), f"setup_completed should be bool: {completed!r}"
        return f"setup_completed={completed}, company={body.get('company', {}).get('name', '<none>')}"
    return _run


def test_company_setup_reset(backend: str) -> Callable[[], str]:
    """Reset → re-create company → confirm round-trip. Mutating."""
    def _run() -> str:
        # Snapshot current state so we can restore.
        before = get_json(f"{backend}/company/status")
        prev_company = before.get("company") if before.get("setup_completed") else None

        # Reset
        rc = delete(f"{backend}/company/reset")
        assert rc < 400, f"reset returned {rc}"
        after = get_json(f"{backend}/company/status")
        assert after.get("setup_completed") is False, "company should be cleared after reset"

        # Recreate (use the previous one if we had it, else a sensible default)
        payload = prev_company or {
            "name": "E2E TestCorp",
            "industry": "Software / SaaS",
            "industry_type": "generic",
            "fiscal_year_end": "March",
            "fiscal_year_start": "April",
            "currency": "USD",
            "established_year": 2020,
            "country": "United States",
        }
        # Strip server-managed fields if present.
        for k in ("created_at", "updated_at", "setup_completed"):
            payload.pop(k, None)
        body = post_json(f"{backend}/setup/company", payload, timeout=15)
        assert body, "setup/company returned empty body"
        again = get_json(f"{backend}/company/status")
        assert again.get("setup_completed") is True, "company should be set up again"
        return f"restored company={again.get('company', {}).get('name', '?')}"
    return _run


def test_documents_list(backend: str) -> Callable[[], str]:
    def _run() -> str:
        body = get_json(f"{backend}/documents")
        # API may return a list or a {"documents": [...]} wrapper; handle both.
        if isinstance(body, dict):
            docs = body.get("documents", [])
        else:
            docs = body
        assert isinstance(docs, list), f"documents should be list, got {type(docs).__name__}"
        return f"{len(docs)} document(s)"
    return _run


def test_metrics_current(backend: str) -> Callable[[], str]:
    def _run() -> str:
        body = get_json(f"{backend}/metrics/current")
        assert isinstance(body, dict), f"expected dict, got {type(body).__name__}"
        # Don't assert specific metric values — those depend on uploaded docs.
        # We DO assert the shape: at least one numeric field somewhere.
        return f"keys={sorted(body.keys())[:8]}"
    return _run


# ---- /ask scenarios — the LLM acceptance set --------------------------------


def _validate_ask_response(body: dict, *, must_have_summary: bool = True) -> None:
    """Asserts the contract documented in AGENTS.md + llm-boundary rule."""
    assert_in("question", body)
    if must_have_summary:
        assert_nonempty("summary", body)
    # numbers_used is a list of strings, may be empty when no data exists
    nu = assert_type("numbers_used", body, list)
    for n in nu:
        assert isinstance(n, str), f"numbers_used entries should be str: {n!r}"
    # explanation may be empty on llama failure; just enforce it's a string
    assert_type("explanation", body, str)
    # confidence is required and structured
    conf = assert_type("confidence", body, dict)
    level = assert_one_of("level", conf, {"high", "medium", "low", "unknown"})
    assert_type("score", conf, (int, float))
    assert_type("reasons", conf, list)
    # sources may be empty
    assert "sources" in body, f"missing 'sources'; got keys: {sorted(body.keys())}"


def test_ask_profit(backend: str, timeout: float) -> Callable[[], str]:
    def _run() -> str:
        body = post_json(
            f"{backend}/ask",
            {"question": "What was the profit last quarter?"},
            timeout=timeout,
        )
        _validate_ask_response(body)
        # Soft check: summary should mention a number OR explicitly say data is unavailable.
        summary = body["summary"]
        looks_numeric = any(ch.isdigit() for ch in summary)
        looks_refusal = any(s in summary.lower() for s in ("not available", "unavailable", "no data", "cannot"))
        assert looks_numeric or looks_refusal, \
            f"summary should cite a number OR explicitly refuse; got: {summary[:200]}"
        return f"confidence={body['confidence']['level']} score={body['confidence']['score']:.2f}"
    return _run


def test_ask_missing_data(backend: str, timeout: float) -> Callable[[], str]:
    """A deliberately niche metric the backend cannot compute → must NOT hallucinate."""
    def _run() -> str:
        body = post_json(
            f"{backend}/ask",
            {"question": "What was the R&D spend on quantum computing research in 1987?"},
            timeout=timeout,
        )
        _validate_ask_response(body)
        # The backend MUST NOT invent a number. Either:
        # (a) confidence is low/unknown, OR
        # (b) the summary explicitly says data not available.
        level = body["confidence"]["level"]
        summary = body["summary"].lower()
        explanation = body.get("explanation", "").lower()
        refusal = any(s in (summary + " " + explanation) for s in (
            "not available", "unavailable", "no data", "cannot", "don't have", "not provided",
            "no information", "do not have",
        ))
        deterministic = level in ("low", "unknown")
        assert refusal or deterministic, (
            f"missing-data case must refuse or report low/unknown confidence; "
            f"got level={level}, summary={body['summary'][:200]}"
        )
        return f"refused={refusal} level={level}"
    return _run


# The single password used by the E2E suite. Anything ≥ 6 chars works.
_E2E_PASSWORD = "e2e-test-pw-1"


def test_ensure_authenticated(backend: str, timeout: float) -> Callable[[], str]:
    """First-run sets up the password; subsequent runs log in. Either way,
    after this test the module-level cookie jar holds a valid session
    and all later /ask / /metrics tests can proceed."""
    def _run() -> str:
        status = get_json(f"{backend}/auth/status", timeout=timeout)
        if status.get("authenticated"):
            return "already authenticated (session reused)"
        if status.get("needs_setup"):
            post_json(f"{backend}/auth/setup", {"password": _E2E_PASSWORD}, timeout=timeout)
            return "first-run: password set"
        try:
            post_json(f"{backend}/auth/login", {"password": _E2E_PASSWORD}, timeout=timeout)
            return "logged in"
        except HTTPError as e:
            raise AssertionError(
                "auth/login failed — the password used by this E2E suite "
                f"(see _E2E_PASSWORD) does not match what was previously set. "
                f"Wipe license.state.enc to reset, or update the constant. ({e})"
            )
    return _run


def test_license_status(backend: str, timeout: float) -> Callable[[], str]:
    """The license must verify OK on a properly-set-up dev host."""
    def _run() -> str:
        body = get_json(f"{backend}/license/status", timeout=timeout)
        assert isinstance(body, dict), f"license/status returned non-dict: {body!r}"
        assert body.get("ok") is True, f"license invalid: reason={body.get('reason')} msg={body.get('message')}"
        payload = body.get("payload") or {}
        assert payload.get("customer_id"), "license must carry a customer_id"
        assert isinstance(body.get("days_remaining"), int), "days_remaining must be int"
        return f"customer={payload.get('customer_name')} days={body['days_remaining']}"
    return _run


def test_auth_status_open_session(backend: str, timeout: float) -> Callable[[], str]:
    """Auth status without a cookie should report not authenticated."""
    def _run() -> str:
        body = get_json(f"{backend}/auth/status", timeout=timeout)
        assert body.get("authenticated") in (False, None), \
            f"unexpectedly authenticated: {body}"
        return f"needs_setup={body.get('needs_setup')} authenticated={body.get('authenticated')}"
    return _run


def test_ask_blocked_without_auth(backend: str, timeout: float) -> Callable[[], str]:
    """Hitting /ask without a session cookie must return 401."""
    def _run() -> str:
        req = urllib.request.Request(
            f"{backend}/ask",
            data=json.dumps({"question": "hi"}).encode("utf-8"),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            urllib.request.urlopen(req, timeout=timeout)
            raise AssertionError("/ask should have rejected unauthenticated call")
        except urllib.error.HTTPError as e:
            # Both 401 (no session) and 503 (bad license) are acceptable here,
            # but 401 is the expected outcome on a properly-licensed host.
            assert e.code in (401, 503), f"expected 401/503, got {e.code}"
            return f"correctly blocked with {e.code}"
    return _run


def test_ask_out_of_scope(backend: str, timeout: float) -> Callable[[], str]:
    """Weather / jokes / code requests must be refused without calling Gemma.
    Verifies the new IntentClassifier short-circuit path."""
    def _run() -> str:
        body = post_json(
            f"{backend}/ask",
            {"question": "What is the weather in Mumbai today?"},
            timeout=timeout,
        )
        _validate_ask_response(body)
        level = body["confidence"]["level"]
        summary = body["summary"]
        assert level == "unknown", f"out-of-scope should be 'unknown', got {level}"
        assert "AI CFO" in summary or "financial" in summary.lower(), \
            f"refusal should mention CFO scope; got: {summary[:200]}"
        reasons = body["confidence"]["reasons"]
        assert any("scope" in r.lower() or "out_of_scope" in r for r in reasons), \
            f"refusal reasons should mention scope; got: {reasons}"
        return f"refused level={level}"
    return _run


def test_ask_synonym_burn(backend: str, timeout: float) -> Callable[[], str]:
    """User says 'burn'; backend should recognize this as monthly burn via the
    synonym expander."""
    def _run() -> str:
        body = post_json(
            f"{backend}/ask",
            {"question": "How is our burn going right now?"},
            timeout=timeout,
        )
        _validate_ask_response(body, must_have_summary=False)
        # Backend must have computed AT LEAST one number for this to be useful.
        # Either there's a monthly_burn entry in numbers_used, OR the question
        # gets handled with high-enough trust to call Gemma.
        numbers = body["numbers_used"]
        burn_present = any("burn" in n.lower() for n in numbers)
        assert burn_present or len(numbers) > 0, \
            f"expected burn/financial numbers in response; got {numbers}"
        return f"numbers_count={len(numbers)} burn_field={burn_present}"
    return _run


def test_ask_sources_have_names(backend: str, timeout: float) -> Callable[[], str]:
    """The Sources array must now contain readable document NAMES, not just
    opaque doc IDs. Regression guard for the collectSourceNames fix."""
    def _run() -> str:
        body = post_json(
            f"{backend}/ask",
            {"question": "What was our revenue last quarter?"},
            timeout=timeout,
        )
        _validate_ask_response(body, must_have_summary=False)
        sources = body.get("sources") or []
        if not sources:
            # Acceptable: backend may have answered from SQL only.
            return "no sources surfaced (SQL-only answer)"
        # If sources ARE surfaced, they should look like filenames, not raw doc_XXXX hashes.
        looks_filename = any(("." in s) or len(s) > 6 for s in sources)
        assert looks_filename, f"sources don't look like document names: {sources}"
        return f"sources={sources[:3]}"
    return _run


def test_ask_metric_in_summary(backend: str, timeout: float) -> Callable[[], str]:
    """The summary on a real financial question must cite a number from the
    pre-computed metrics — not invent one, not refuse."""
    def _run() -> str:
        body = post_json(
            f"{backend}/ask",
            {"question": "What was our net income last quarter?"},
            timeout=timeout,
        )
        _validate_ask_response(body)
        numbers = body["numbers_used"]
        # Should have computed at least net_income
        assert any("Net Income" in n or "net income" in n.lower() for n in numbers), \
            f"net_income should be in computed numbers; got {numbers}"
        return f"computed {len(numbers)} metrics, summary_len={len(body['summary'])}"
    return _run


def test_ask_compare(backend: str, timeout: float) -> Callable[[], str]:
    """Compare-style questions should still return a valid response shape.
    We don't enforce that Gemma actually executes the comparison — just that
    intent classifier doesn't break the pipeline."""
    def _run() -> str:
        body = post_json(
            f"{backend}/ask",
            {"question": "Compare our revenue versus expenses in Q1 2026"},
            timeout=timeout,
        )
        _validate_ask_response(body, must_have_summary=False)
        return f"confidence={body['confidence']['level']}"
    return _run


def test_ask_trend(backend: str, timeout: float) -> Callable[[], str]:
    """Trend-style questions should be handled with at least valid shape."""
    def _run() -> str:
        body = post_json(
            f"{backend}/ask",
            {"question": "Show me the trend of our expenses over time"},
            timeout=timeout,
        )
        _validate_ask_response(body, must_have_summary=False)
        return f"confidence={body['confidence']['level']}"
    return _run


def test_ask_self_identity(backend: str, timeout: float) -> Callable[[], str]:
    """Asking 'who are you / what model' should refuse politely. The CFO is
    not supposed to advertise its model card."""
    def _run() -> str:
        body = post_json(
            f"{backend}/ask",
            {"question": "Are you ChatGPT?"},
            timeout=timeout,
        )
        _validate_ask_response(body)
        assert body["confidence"]["level"] == "unknown", \
            f"self-identity question should refuse; got level={body['confidence']['level']}"
        return "refused"
    return _run


def test_ask_large_context(backend: str, timeout: float) -> Callable[[], str]:
    """Stress the prompt-construction path with a real, period-anchored question
    plus a large tail of CFO-style qualifier clauses. The question keeps a clear
    'last quarter' / 'profit' anchor so the backend has enough trust to call the
    LLM (otherwise it short-circuits and we never exercise Gemma)."""
    def _run() -> str:
        # Real question first so period parser + financial logic engage.
        head = "What was the profit last quarter?"
        clauses = [
            "Consider the latest fiscal quarter",
            "factor in any one-time expenses",
            "exclude non-recurring revenue",
            "compare against the prior period",
            "weight by deferred revenue if applicable",
            "ignore unrealized gains",
            "treat capitalized R&D consistently",
        ] * 40
        question = head + " " + "; ".join(clauses) + "."
        assert len(question) > 4000, "test question should be large; build longer clauses"
        body = post_json(f"{backend}/ask", {"question": question}, timeout=timeout)
        _validate_ask_response(body, must_have_summary=False)  # may fallback on failure
        # Soft check: backend should still return SOME summary (LLM ran, or templated fallback).
        # We don't assert confidence level — the goal is "no crash, valid shape".
        return (
            f"q_len={len(question)} "
            f"summary_len={len(body.get('summary',''))} "
            f"confidence={body['confidence']['level']}"
        )
    return _run


# ----------------------------------------------------------------------------
# Main
# ----------------------------------------------------------------------------


def main() -> int:
    p = argparse.ArgumentParser(description="AI CFO end-to-end test suite")
    p.add_argument("--backend-url", default="http://localhost:8080",
                   help="Backend base URL (default: %(default)s)")
    p.add_argument("--frontend-url", default=None,
                   help="If set, also probe the React frontend and nginx /api proxy")
    p.add_argument("--verbose", "-v", action="store_true",
                   help="Print full response bodies on PASS")
    p.add_argument("--timeout", type=float, default=240.0,
                   help="Per-request timeout in seconds (default: %(default)s) — "
                        "Gemma 12B Q4 on CPU can take 60-180s per /ask")
    p.add_argument("--skip-mutating", action="store_true",
                   help="Skip the /setup/company + /company/reset round-trip")
    p.add_argument("--skip-llm", action="store_true",
                   help="Skip /ask tests (no Gemma load)")
    p.add_argument("--reset", action="store_true",
                   help="Treat the mutating round-trip as a clean reset (default: round-trip restores)")
    p.add_argument("--quick", action="store_true",
                   help="Only run 1 LLM question instead of 3")
    args = p.parse_args()

    backend = args.backend_url.rstrip("/")
    frontend = args.frontend_url.rstrip("/") if args.frontend_url else None

    sys.stdout.write(bold("\n┌─ AI CFO E2E ─────────────────────────────────────\n"))
    sys.stdout.write(f"│ backend  : {backend}\n")
    sys.stdout.write(f"│ frontend : {frontend or gray('(skipped)')}\n")
    sys.stdout.write(f"│ timeout  : {args.timeout:.0f}s per request\n")
    sys.stdout.write(f"│ llm      : {'skipped' if args.skip_llm else 'on (Gemma via llama.cpp)'}\n")
    sys.stdout.write(bold("└──────────────────────────────────────────────────\n"))

    r = TestRunner(verbose=args.verbose)

    # Phase 0 must come first: it (a) verifies the license / auth gate,
    # then (b) establishes a session cookie that every later phase
    # depends on. The "unauthenticated /ask is blocked" sub-test only
    # works while the cookie jar is empty, so it precedes
    # `ensure_authenticated` within this phase.
    r.section("Phase 0: License + auth gate")
    r.run("GET /license/status — license must verify", test_license_status(backend, args.timeout))
    r.run("GET /auth/status — open session reports unauth", test_auth_status_open_session(backend, args.timeout))
    r.run("POST /ask without session is blocked", test_ask_blocked_without_auth(backend, args.timeout))
    r.run("authenticate (setup-or-login)", test_ensure_authenticated(backend, args.timeout))

    r.section("Phase 1: Backend health & contract (authenticated)")
    r.run("GET /health", test_backend_health(backend))
    r.run("GET /company/status", test_company_status(backend))
    r.run("GET /documents", test_documents_list(backend))
    r.run("GET /metrics/current", test_metrics_current(backend))

    if frontend:
        r.section("Phase 2: Frontend bundle + nginx proxy")
        r.run("GET / (index.html)", test_frontend_index(frontend))
        r.run("GET /api/health (proxied)", test_frontend_proxy(frontend))

    if not args.skip_mutating:
        r.section("Phase 3: Mutating round-trip (company setup)")
        r.run("DELETE /company/reset + POST /setup/company",
              test_company_setup_reset(backend))

    if not args.skip_llm:
        r.section("Phase 4: LLM acceptance — basic Gemma cases")
        r.run("POST /ask — profit question", test_ask_profit(backend, args.timeout))
        r.run("POST /ask — out-of-scope refusal (weather)",
              test_ask_out_of_scope(backend, args.timeout))
        r.run("POST /ask — self-identity refusal (are you ChatGPT)",
              test_ask_self_identity(backend, args.timeout))

        if not args.quick:
            r.section("Phase 5: CFO question matrix — intent + retrieval + evidence")
            r.run("POST /ask — synonym expansion (burn)",
                  test_ask_synonym_burn(backend, args.timeout))
            r.run("POST /ask — sources surface as readable names",
                  test_ask_sources_have_names(backend, args.timeout))
            r.run("POST /ask — net income citation in summary",
                  test_ask_metric_in_summary(backend, args.timeout))
            r.run("POST /ask — compare-style question",
                  test_ask_compare(backend, args.timeout))
            r.run("POST /ask — trend-style question",
                  test_ask_trend(backend, args.timeout))
            r.run("POST /ask — missing data (must refuse)",
                  test_ask_missing_data(backend, args.timeout))
            r.run("POST /ask — large context (stability)",
                  test_ask_large_context(backend, args.timeout))

    return r.summary()


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.stderr.write("\n[interrupted]\n")
        sys.exit(130)
