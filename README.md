# AI CFO

A local-first, deterministic financial-analysis assistant. Backed by SQLite for structured facts, hybrid RAG for evidence, and a locally-hosted Gemma model (via `llama.cpp`) for narration. No cloud APIs. No telemetry. No data leaves the host.

> **Read this section before selling to enterprise clients:** the [Production-readiness assessment](#production-readiness-assessment) section is honest about what's done and what isn't. Don't skip it.

---

## Table of contents

- [Quickstart](#quickstart) — 60-second bring-up
- [What it does](#what-it-does)
- [Architecture](#architecture)
- [Features](#features)
- [Configuration reference](#configuration-reference) — every env var
- [`run.sh` reference](#runsh-reference)
- [Licensing & login](#licensing--login) — Ed25519, machine binding, migration, renewal
- [Production-readiness assessment](#production-readiness-assessment) — the truth
- [Testing](#testing)
- [Troubleshooting](#troubleshooting)

---

## Quickstart

### Prerequisites (one-time setup on each host)

| Tool | Min version | Install |
|---|---|---|
| Go | 1.21 | <https://go.dev/dl/> |
| Node | 18 | <https://nodejs.org/> |
| Python | 3.9 | already on macOS/Linux, used only for the test runner |
| `llama.cpp` | latest | see below |
| Gemma GGUF model | 7B or 9B Q4_K_M | see below |
| Disk | ~15 GB | model + repo |
| RAM | 16 GB min, 32 GB recommended | for the 9B model |

#### Build `llama.cpp`

```bash
git clone https://github.com/ggerganov/llama.cpp
cd llama.cpp
make -j        # add LLAMA_METAL=1 on Apple Silicon for GPU
ln -s build/bin/llama-completion main
cd ..
```

#### Download the Gemma model (~5 GB)

You need a Hugging Face account and to accept Google's Gemma license once.

```bash
pip install --user "huggingface_hub[cli]"
hf auth login                                       # paste your HF token
hf download google/gemma-2-9b-it-GGUF gemma-2-9b-it-Q4_K_M.gguf \
   --local-dir ./backend/models
mv ./backend/models/gemma-*.gguf ./backend/models/gemma.gguf
```

For machines with **less than 32 GB RAM**, use the 7B variant:
`google/gemma-7b-it-GGUF`, file `gemma-7b-it.Q4_K_M.gguf`.

### Bring up the stack

```bash
cp .env.example .env       # edit any values you need to change
./run.sh check             # verify deps
./run.sh start             # boots backend, frontend (+ optional llama-server)
./run.sh status            # show what's listening where
./run.sh test              # run the E2E suite against the running stack
```

Then open <http://localhost:3000> in your browser.

To stop everything: `./run.sh stop`.

---

## What it does

The AI CFO answers natural-language questions about a company's financials *only* when it has structured evidence to back the answer. It is engineered so that **numbers come from SQL, the LLM only narrates**.

Example questions it handles well:

- *"What was our profit last quarter?"*
- *"How much cash do we have right now?"*
- *"Compare Q1 vs Q2 revenue."*
- *"Why did expenses spike in March?"*
- *"What is our cash runway?"*

Example inputs it refuses (deterministically, without ever calling the model):

- *"What's the weather in Mumbai?"* → out-of-scope refusal
- *"Are you ChatGPT?"* → self-identity refusal
- *"hi" / "hello" / "thanks"* → friendly greeting, no data summary

---

## Architecture

```
                       ┌─────────────────────────────────────────────┐
                       │  Frontend (React + Vite, :3000)             │
                       │   ─ dev-mode proxy /api → backend           │
                       └────────────────┬────────────────────────────┘
                                        │ HTTP
                       ┌────────────────▼────────────────────────────┐
                       │  Go backend (:8080) — `cmd/server`          │
                       │                                             │
   POST /ask  ──►  ┌───┴──────────────────────────────────────────┐  │
                   │  Ask pipeline (deterministic, ordered)       │  │
                   │  0. IntentClassifier  ── greeting? refuse?   │  │
                   │  1. PeriodParser      ── "last quarter"      │  │
                   │  2. FinancialLogic    ── SQL-backed numbers  │  │
                   │  3. Industry resolve  ── filters + vocab     │  │
                   │  4. HybridRetriever   ── vector + keyword    │  │
                   │  5. Reranker          ── top-K=5             │  │
                   │  6. ConflictDetector  ── period-scoped       │  │
                   │  7. Confidence score  ── deterministic       │  │
                   │  8. LLM (Gemma)       ── narrates, no math   │  │
                   │  9. AuditSink         ── append-only SQLite  │  │
                   └──────────────────────────────────────────────┘  │
                       │                                             │
                       │ exec.Command (per-request subprocess)       │
                       ▼                                             │
                   llama.cpp + Gemma GGUF  (Metal/CPU)               │
                                                                     │
   storage:  SQLite (facts + audit)  +  in-memory vectors / Qdrant   │
                       └─────────────────────────────────────────────┘
```

**Key invariants:**

1. Numbers come from `FinancialLogic` (SQL). The LLM is forbidden from computing.
2. Retrieval filters are applied **before** vector search, not after.
3. Conflicts cap confidence at "Low".
4. Out-of-scope and greetings short-circuit the pipeline; the LLM is not invoked.

---

## Features

### Deterministic, auditable backend
- **Intent classifier** with 30+ canonical greetings and ~50 keyword groups for out-of-scope detection. Pure regex/keyword — no ML, no surprises.
- **Period parser** for natural-language time references (`"Q1 2024"`, `"last quarter"`, `"FY 2023"`).
- **Financial logic** computes revenue, expenses, net income, cash, runway, monthly burn, total assets/liabilities/equity from line-item data stored in SQLite.
- **Hybrid retrieval**: semantic (vector) + keyword (BM25-ish over `FileStore`) fused via Reciprocal Rank Fusion. Two-pass with broadened filter on empty first pass.
- **Synonym expansion** for retrieval recall: 22 financial-term groups (`burn` → `monthly burn`, `earnings` → `net income`, etc.). The LLM still receives the user's original words.
- **Reranker** with deterministic top-K cut.
- **Conflict detector** that's period-aware (won't flag 2024 vs 2026 numbers as conflicts) and source-aware (dedups sub-totals within one document).
- **Confidence scorer** that combines SQL coverage, evidence count, agreement ratio, conflicts, and period match into a single deterministic score.

### LLM contract
- Local Gemma via `llama.cpp` — no cloud, no network egress.
- Deterministic sampling (`temperature=0.2`, `top_p=0.9`, `seed=42`).
- Prompt forces: never invent numbers, never answer out-of-scope, always cite evidence by `[E1]/[E2]` tags.
- Hard wall-clock timeout per request.
- Graceful fallback message on LLM failure (never a stack trace to the user).

### Data
- **SQLite** is the source of truth for structured metrics.
- **JSON migration** from `data/` on startup — backward compatible.
- **Document parser** for CSV, XLSX, and PDF.
- **Industry handlers** for `ecommerce`, `education`, `pharma`, and a `generic` fallback. Each contributes vocabulary and intent hints into retrieval.
- **Append-only audit log** of every `/ask` call (question, period, numbers used, evidence IDs, confidence, conflicts, errors).

### API surface

| Method | Path | Purpose |
|---|---|---|
| GET | `/health` | Health check |
| GET | `/company/status` | Setup state + company profile |
| POST | `/setup/company` | One-time onboarding (multipart with docs) |
| POST | `/company/reset` | Wipe and start over |
| GET | `/documents` | List uploaded documents |
| POST | `/documents` | Upload a document |
| GET | `/metrics/current` | Latest computed metrics |
| POST | `/ask` | The main Q&A endpoint |

### Frontend
- React + Vite. Dev mode proxies `/api/*` to backend.
- Talks to backend only — no third-party endpoints.

---

## Configuration reference

All values are environment variables. Defaults are in `backend/internal/config/config.go`. Copy `.env.example` to `.env` and edit; `run.sh` auto-loads it.

### Network

| Variable | Default | What it controls |
|---|---|---|
| `PORT` | `8080` | Go backend listen port |
| `FRONTEND_PORT` | `3000` | Vite dev / nginx prod port |
| `LLAMA_SERVER_PORT` | `8081` | Optional raw Gemma chat UI |
| `START_LLAMA_SERVER` | `false` | Whether `run.sh start` also launches the standalone llama-server |

### Storage

| Variable | Default | What it controls |
|---|---|---|
| `DATA_DIR` | `./backend/data` | Root for uploaded docs, state, audit log |
| `SQLITE_ENABLED` | `true` | Use SQLite as source of truth for metrics |
| `SQLITE_PATH` | `${DATA_DIR}/state/cfo.db` | SQLite database file |

### Vector backend

| Variable | Default | What it controls |
|---|---|---|
| `VECTOR_BACKEND` | `memory` | `memory` (in-process) or `qdrant` |
| `QDRANT_URL` | `http://qdrant:6333` | Qdrant endpoint (when used) |
| `QDRANT_COLLECTION` | `cfo_chunks` | Qdrant collection name |
| `QDRANT_API_KEY` | `""` | Required for Qdrant Cloud |
| `EMBEDDING_DIM` | `768` | Must match the model used to embed |

### LLM runtime

| Variable | Default | What it controls |
|---|---|---|
| `LLAMA_CPP_BINARY` | `./llama.cpp/main` | Path to compiled `llama-completion` binary |
| `MODEL_PATH` | `./backend/models/gemma.gguf` | Path to Gemma GGUF weights |
| `MODEL_NAME` | `gemma` | Cosmetic, used in logs |
| `LLM_MAX_TOKENS` | `512` | Hard cap on tokens generated per `/ask` |
| `LLM_TEMPERATURE` | `0.2` | 0 = greedy, 1 = creative |
| `LLM_TOP_P` | `0.9` | Nucleus sampling cutoff |
| `LLM_SEED` | `42` | `-1` disables; fixed seed = reproducible answers |
| `LLM_CONTEXT_SIZE` | `4096` | Must match the GGUF's training context |
| `LLM_TIMEOUT_SEC` | `120` | Wall-clock cap per LLM invocation |
| `LLM_THREADS` | `0` | `0` = llama.cpp auto; tune to your CPU |

### Logs

| Variable | Default | What it controls |
|---|---|---|
| `LOG_DIR` | `/tmp/cfo-logs` | Where `run.sh` writes service stdout/stderr |

---

## `run.sh` reference

```
./run.sh start       # start backend + frontend (+ optional llama-server)
./run.sh stop        # stop everything
./run.sh status      # show port + pid for each service
./run.sh logs SVC    # tail logs (SVC = backend | frontend | llama-server)
./run.sh check       # verify prerequisites without starting anything
./run.sh test        # run the python E2E suite against the running stack
./run.sh help        # this usage
```

The script:
- Auto-loads `.env` if present.
- Refuses to start a service if its port is already in use.
- Runs `npm install` only on first start (when `frontend/node_modules` is missing).
- Writes one log file per service to `$LOG_DIR`, plus a `.pid` file used by `stop`.
- Waits for each service to bind its port before declaring success.

---

## Licensing & login

The product ships as a single-tenant, on-prem, air-gapped application. There are no remote auth servers, no telemetry, and no internet calls — but the backend still enforces a real cryptographic license check at startup before exposing any business API.

### Architecture

```
┌──────────────────┐                    ┌─────────────────────────────────────┐
│  VENDOR HOST     │   never shipped    │  CUSTOMER HOST                      │
│  ─────────────   │ ────────────────►  │  ─────────────                      │
│  private key  ◄──┤                    │  embedded public key (compile-time) │
│  license-gen ───►│  license.lic       │  cfo-server: VERIFIES at startup    │
│  signs payload   │  ───────────────►  │      └─ machine fingerprint binding │
│                  │                    │      └─ expiry check                │
│                  │                    │      └─ AES-256-GCM encrypted state │
└──────────────────┘                    └─────────────────────────────────────┘
```

- **Algorithm:** Ed25519. Tampering with even one byte of the license payload breaks verification.
- **Public key:** baked into the backend binary at compile time (`backend/internal/license/pubkey_embed.pem`).
- **Private key:** stays with you. NEVER ship `license-gen` to customers.
- **Machine binding:** the license activates against a fingerprint derived from CPU / motherboard UUID / disk serial / hostname / MAC addresses. Override with `LICENSE_MACHINE_ID` for Kubernetes / VM deployments.
- **Local state:** activation metadata + nonces + password hash are encrypted (AES-256-GCM) under a key derived from the machine fingerprint, so the state file can't be copied to another host and decrypted there.

### Backend startup flow

1. `LoadLicense` — read `$LICENSE_FILE` (default `./license.lic`).
2. `VerifySignature` — Ed25519 verify against the embedded public key.
3. `CheckMachineBinding` — first run binds to this host, subsequent runs compare.
4. `CheckExpiry` — refuse if past expiry, warn if within 30 days.
5. Server binds the port either way. If license is invalid, every business endpoint returns `503 license_invalid` and `/license/status` returns the structured reason so the frontend can render an actionable error page.

### Frontend flow

```
   ┌──────────────────────────────────────────────────────────┐
   │ on load → GET /license/status                            │
   │   └─ ok=false  → LicenseError page (file_missing /       │
   │                                     expired / machine    │
   │                                     mismatch / etc.)     │
   │   └─ ok=true   → GET /auth/status                        │
   │        └─ needs_setup=true → LoginPage (first-run setup) │
   │        └─ authenticated=false → LoginPage (login)        │
   │        └─ authenticated=true → main dashboard            │
   └──────────────────────────────────────────────────────────┘
```

Single-password login (bcrypt-hashed, stored in the encrypted state). The user picks the password on first run; subsequent runs require it. Session is an HTTP-only, HMAC-signed cookie scoped to localhost. 24-hour TTL.

### Issuing a license (vendor side)

```bash
# 1. ONE TIME: generate the vendor keypair. PRIVATE KEY STAYS WITH YOU.
./backend/license-gen keygen          # writes config/license_{priv,pub}key.pem
                                      # + backend/internal/license/pubkey_embed.pem
# 2. Rebuild the customer binary so the new public key is baked in.
cd backend && go build ./cmd/server ./cmd/cfo-license

# 3. Issue a license for a specific customer.
./backend/license-gen create \
    -priv config/license_privkey.pem \
    -customer "ACME Corp" \
    -customer-id ACME-001 \
    -expiry 2027-05-15 \
    -features AI_CFO,Forecasting,FinancialReports,AuditAssistant \
    -max-users 1 \
    -type Enterprise \
    -out customers/acme/license.lic

# 4. Inspect / verify a license:
./backend/license-gen show -in customers/acme/license.lic
```

License types: `Trial | Startup | Enterprise | Unlimited`.
Features (free-form, the backend gates on string equality):
`AI_CFO` · `Forecasting` · `FinancialReports` · `AuditAssistant` (extend as needed).

### Customer-side CLI (`cfo-license`)

```bash
./run.sh license status              # show binding state, expiry, features, machine_id
./run.sh license deactivate          # emit migration.dat + migration_pub.key
./run.sh license activate migration.dat -pub migration_pub.key   # bind to new host
./run.sh license export-request      # generate request.dat for renewal
```

Status output:

```
=== AI CFO License Status ===
  customer     : ACME Corp (ACME-001)
  type         : Enterprise
  expires      : 2027-12-31
  features     : [AI_CFO Forecasting FinancialReports AuditAssistant]
  machine id   : ca161cdf…57f4b83
  STATUS       : valid — 594 days remaining
```

### Machine migration

The system **does not** enforce a fixed machine count — customers can move freely as long as they go through deactivate-then-activate:

```bash
# OLD MACHINE
./run.sh license deactivate          # emits migration.dat + migration_pub.key
                                      # AND marks the local state as deactivated
                                      # (backend on this host will refuse to start)

# NEW MACHINE
# Copy license.lic, migration.dat, migration_pub.key to the new host.
./run.sh license activate migration.dat -pub migration_pub.key
# Verifies signature, verifies digest matches license.lic on this host,
# checks nonce freshness (replay-protection), checks timestamp drift,
# then binds the new machine fingerprint. The old machine stays disabled.
```

Replay protection: every migration carries a 128-bit nonce. Importing the same `migration.dat` twice fails with `replay_detected`.

### Renewal (no internet required)

```bash
# Customer (nearing expiry — the backend log + LicenseError UI both warn at 30 days)
./run.sh license export-request -out request.dat
# Send request.dat to vendor (email, USB, whatever).

# Vendor
./backend/license-gen renew \
    -priv config/license_privkey.pem \
    -request request.dat \
    -expiry 2028-12-31 \
    -features AI_CFO,Forecasting,FinancialReports,AuditAssistant \
    -out renewed.lic
# Send renewed.lic back to customer.

# Customer: drop the renewed file in as license.lic and restart.
mv renewed.lic license.lic && ./run.sh stop && ./run.sh start
```

### Security guarantees and honest caveats

| Threat | Defense |
|---|---|
| Manual JSON edit of `license.lic` | Ed25519 signature breaks on any byte change |
| Copying `license.lic` to another host | Machine-fingerprint binding rejects on first verify |
| Copying the encrypted state file | AES-256-GCM key is derived from the destination host's fingerprint → decryption fails |
| Forging a license without our private key | Cryptographically infeasible (Ed25519) |
| Replaying a deactivation migration | Nonce list in encrypted state |
| Clock tampering on the customer host | Migration files are rejected if >30 days old or >5min in the future |

**Honest caveats** (so you don't oversell this to a CISO):

1. **"Encrypted local state" is defense against the casual attacker.** A determined attacker with root on the customer host can re-derive the state key from the same static prefix + machine fingerprint our backend uses. The real anti-tamper is the Ed25519 signature on `license.lic` itself, which is unforgeable without the vendor private key. The state encryption mainly prevents trivial JSON-editing.
2. **Public-key embed is at compile time.** If you ship a binary with the wrong / outdated public key, every license fails to verify. Always rebuild the backend after `license-gen keygen`.
3. **Single password, no users.** This matches the user's spec — "no user is included here, just match password." There is no role-based access, no per-user audit, no SSO, no rotation flow beyond wiping the encrypted state.
4. **Kubernetes / Docker fingerprint stability.** Container fingerprints can shift on host migration. Set `LICENSE_MACHINE_ID` to a stable value from a Secret/ConfigMap to make the binding deterministic across pod restarts.

### License config reference

| Variable | Default | Purpose |
|---|---|---|
| `LICENSE_FILE` | `./license.lic` | path to the signed license file the backend loads at startup |
| `LICENSE_STATE` | `$DATA_DIR/state/license.state.enc` | path to the encrypted local state (activations, nonces, auth blob) |
| `LICENSE_PUBLIC_KEY` | — | base64 raw 32-byte Ed25519 public key — only used when the compile-time embed is empty (tests, dev) |
| `LICENSE_MACHINE_ID` | — | override the auto-detected fingerprint. Required for stable K8s deployments |

---

## Production-readiness assessment

**Bottom line: this is a strong proof-of-concept and a good demo for one user on one machine. It is NOT enterprise-production-grade. Do not represent it as such to international clients.**

### What is production-grade today

- Deterministic numerical layer (SQL > LLM) — auditable, reproducible.
- Hard contract between LLM and pipeline (numbers from SQL only).
- Pre-retrieval filtering and conflict-aware confidence scoring.
- Append-only audit log of every Q&A call.
- Local-only execution — no data egress.
- Health check endpoint, graceful LLM-failure fallback.
- 14/14 end-to-end test coverage on the Ask flow.
- Out-of-scope and greeting short-circuits prevent the LLM from going off-rails.

### What is NOT production-grade — checklist for enterprise readiness

| Capability | Status | Effort to ship |
|---|---|---|
| **Authentication** (the `/ask` endpoint is open to anyone on the network) | missing | 1–2 weeks (OAuth/OIDC, sessions) |
| **Authorization / RBAC** (no concept of users or roles) | missing | 1 week |
| **Multi-tenancy** (one company per process today) | missing | 2–3 weeks (per-tenant SQLite or shared with tenant_id) |
| **TLS termination** | missing | 1 day (nginx/Caddy in front) |
| **Rate limiting** | missing | 1 day (middleware) |
| **HA / clustering** | single instance | 1–2 weeks |
| **Concurrent request handling for LLM** (spawn-per-request crushes RAM at >5 concurrent) | weak | 1 week (request queue + worker pool, or move to llama-server HTTP backend) |
| **Persistent vector store** (in-memory is the default; Qdrant path exists but is not exercised in CI) | partial | 3–5 days to harden |
| **Observability** (Prometheus, OpenTelemetry tracing) | missing | 1 week |
| **Secrets management** (env vars in plaintext only) | missing | 3–5 days (Vault / AWS Secrets Manager / GCP SM) |
| **Backups / DR** (SQLite file on disk, no replication) | missing | 1 week |
| **Encryption at rest** | missing | 3–5 days (SQLite SEE / filesystem-level) |
| **File-upload sanitization / virus scanning** | missing | 1 week |
| **CORS hardening** (currently permissive) | weak | 1 day |
| **Idempotency keys** for retries | missing | 2–3 days |
| **PII redaction** in audit logs and prompts | missing | 1 week |
| **GDPR data-subject erasure flow** | missing | 1–2 weeks |
| **SOC 2 / ISO 27001 / HIPAA controls** | not started | months of org-level work |
| **CI/CD pipeline** (build, test, image scan, signed release) | missing | 1 week |
| **Container hardening** (distroless, non-root, resource limits) | partial | 3 days |
| **Data residency controls** (per-region pinning) | missing | 1–2 weeks |
| **Telemetry opt-out / privacy policy** | n/a (no telemetry today) | 1 day to document |
| **Disaster runbook** | missing | 1 week |

**Realistic timeline to "enterprise-production-ready":** 8–12 weeks with two senior engineers, assuming you also bring in legal/compliance for the privacy work.

### What you CAN sell honestly today

- A **single-tenant, on-premises** deployment for one enterprise client per host.
- A **demo / pilot** that runs on the client's laptop or a single VM behind their firewall.
- A **deterministic financial Q&A** layer that *does not* hallucinate numbers — a real differentiator vs ChatGPT-style apps.
- **Full data sovereignty**: nothing leaves the host. This is a strong sell to EU and India clients worried about US cloud exposure.

### What you should NOT promise

- "Production-grade multi-tenant SaaS" — it isn't.
- "Enterprise SLA" — there is no HA path yet.
- "SOC 2 / GDPR compliant" — no compliance work has been done.
- "Scales to millions of users" — the LLM layer alone caps you at ~5 concurrent users per host.

---

## Testing

```bash
# Go unit + integration tests (backend)
cd backend && go test ./... -count=1

# Python E2E suite (requires running stack)
./run.sh test                    # or:
python3 scripts/e2e.py --skip-mutating --timeout 300
```

Current state: 14/14 E2E tests passing (health, contract, intent routing, refusal, synonym expansion, evidence surfacing, large-context stability).

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| `ECONNREFUSED 0.0.0.0:8080` in frontend logs | Backend not running | `./run.sh start` |
| `address already in use` | Backend already running, or a zombie | `./run.sh stop` then `./run.sh start` |
| LLM responses take 60+ seconds | First call loads the model into RAM (~7 GB for 9B) | Normal; subsequent calls are faster on Metal. Reduce `LLM_MAX_TOKENS` or use the 7B model. |
| `Unable to generate explanation` | `llama.cpp` binary or model missing | `./run.sh check` |
| Out-of-memory crash on macOS | Trying to run 9B on 16 GB | Switch to the 7B Q4_K_M model |
| Backend hangs in chat mode | Using `llama-cli` instead of `llama-completion` | Re-symlink: `ln -sf build/bin/llama-completion llama.cpp/main` |
| `npm: command not found` | Node not installed | <https://nodejs.org/> |
| Frontend works on `:3000` but `/api/*` 404s | Vite proxy mis-configured | Verify `frontend/vite.config.js` proxies `/api → http://0.0.0.0:8080` |

For anything else, check the per-service log:

```bash
./run.sh logs backend
./run.sh logs frontend
./run.sh logs llama-server
```
