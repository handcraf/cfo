# Changelog

All notable changes to AI CFO are recorded here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

The `Unreleased` section accumulates work since the most recent tag; on cut, it gets promoted to a version heading.

---

## [Unreleased]

### Added

- (track upcoming work here)

### Changed

- (track changes here)

### Fixed

- (track fixes here)

---

## [0.1.0] — 2026-05-15

First versioned release. Establishes the production baseline before any customer deployment.

### Added

- **Ed25519-signed licensing system**: `license.lic` + canonical-JSON signing, machine-fingerprint binding, AES-256-GCM encrypted local state, signed migration tokens with nonce-replay protection, no-internet renewal flow via `request.dat`. 24 license unit tests, all green. See `README.md` Licensing & login section and `docs/RELEASE_PROCESS.md`.
- **Single-password login**: bcrypt-hashed password persisted in the encrypted license state, HTTP-only HMAC-signed cookie sessions (24 h TTL), 250 ms constant-time delay on wrong password, first-run setup flow.
- **HTTP gate middleware**: `LicenseGate` (503 with structured `reason` + `action`) and `AuthGate` (401) protect every business endpoint. Bypass list: `/health`, `/auth/*`, `/license/status`.
- **Vendor CLI** (`backend/cmd/license-gen`, NOT shipped): `keygen`, `create`, `renew`, `show`.
- **Customer CLI** (`backend/cmd/cfo-license`, ships): `status`, `deactivate`, `activate`, `export-request`.
- **Frontend gate**: `LoginPage` + `LicenseError` page with context-aware Next-Steps keyed off `reason`.
- **Intent classifier** + **synonym expansion** + **period-aware conflict detection** + canned greeting short-circuit. Deterministic, no LLM in the routing path.
- **End-to-end test runner** (`scripts/e2e.py`) with cookie-jar session reuse; 18 scenarios covering Phases 0–5.
- **Client onboarding doc** (`docs/ONBOARDING.md`) and **internal release process** (`docs/RELEASE_PROCESS.md`).

### Changed

- LLM runtime switched from Ollama → local `llama.cpp` + Gemma GGUF with deterministic sampling (`temp=0.2`, `top_p=0.9`, `seed=42`, `-no-cnv`). No cloud calls.
- Backend startup now runs license verification before binding business APIs. Invalid license → server still binds the port but every protected endpoint returns 503 so the frontend can render the `LicenseError` page.
- `run.sh` resolves all path-related env vars to absolute paths so `DATA_DIR`, `MODEL_PATH`, `LICENSE_FILE`, and `LICENSE_STATE` are stable regardless of the backend's working directory.
- Conflict detector now operates only on evidence chunks that overlap the metrics' period, and dedupes claims by `(metric, source)` to avoid false positives from internal sub-totals.

### Fixed

- "hi" greeting no longer triggers the full RAG/LLM pipeline. Returns a canned friendly response.
- `run.sh stop` now reliably kills `go run` child processes via `pgrep -P` + `lsof` cleanup on bound ports.

### Security

- Vendor private key (`config/license_privkey.pem`) added to `.gitignore`. Per-install artifacts (`license.lic`, `*.state.enc`, `migration.dat`, `migration_pub.key`, `request.dat`) likewise excluded from version control.
- CORS hardened: localhost variants only when `credentials: include` is required; no wildcard origin with credentials.
