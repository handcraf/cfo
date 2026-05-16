# AI CFO — Internal Release & Update Process

**Audience:** engineering / release-management team.
**Status:** internal only — DO NOT ship this file to customers.

This document is the source of truth for how we produce, version, sign, and ship customer-facing builds. It is paired with `docs/ONBOARDING.md`, which is what the customer sees on the receiving end.

---

## Table of contents

1. [Release types & versioning](#1-release-types--versioning)
2. [Pre-release checklist](#2-pre-release-checklist)
3. [Build matrix & supported platforms](#3-build-matrix--supported-platforms)
4. [What goes in the customer bundle](#4-what-goes-in-the-customer-bundle)
5. [What stays with us (never ships)](#5-what-stays-with-us-never-ships)
6. [Producing the bundle (`scripts/release.sh`)](#6-producing-the-bundle-scriptsreleasesh)
7. [Signing & checksumming](#7-signing--checksumming)
8. [License issuance & renewal](#8-license-issuance--renewal)
9. [Public-key rotation policy](#9-public-key-rotation-policy)
10. [Customer notification + delivery](#10-customer-notification--delivery)
11. [Update compatibility guarantees](#11-update-compatibility-guarantees)
12. [Rollback & hot-fix process](#12-rollback--hot-fix-process)
13. [Common release pitfalls](#13-common-release-pitfalls)

---

## 1. Release types & versioning

We follow Semantic Versioning with a fixed contract on what each level can break.

| Level | Example | What it may change | License compat? |
|---|---|---|---|
| **PATCH** | 1.0.0 → 1.0.1 | Bug fixes only. No API changes. No license schema changes. | Always compatible |
| **MINOR** | 1.0.x → 1.1.0 | New features, new endpoints, new metrics, new industry modules, new optional config keys. Existing config keeps working. | Always compatible |
| **MAJOR** | 1.x → 2.0 | Breaking changes: removed endpoints, license schema changes, required new env vars, new minimum OS/CPU. | **Re-issue licenses if license schema changed**. Force a planned upgrade window per customer. |

### Version sources

The version string is written to ONE place — `VERSION` at repo root — and **everything else reads it**:

- `scripts/release.sh` stamps it into the bundle filename and the in-bundle `VERSION` file
- The backend reads it on startup and logs it
- The CHANGELOG.md tracks every version
- The license-gen CLI version-tags its build output

A release is **not legitimate** unless `VERSION`, `CHANGELOG.md`, and the git tag agree.

### Tag format

```
git tag -a v1.1.0 -m "Release 1.1.0: <one-line summary>"
git push origin v1.1.0
```

---

## 2. Pre-release checklist

Run this every time. No exceptions. Treat each line as a hard gate.

```
[ ] Branch is `main` and clean (`git status` empty)
[ ] All unit tests pass:    cd backend && go test ./... -count=1
[ ] E2E suite passes (18/18): ./run.sh test
[ ] License unit tests pass:  go test ./internal/license/ -count=1
[ ] Lint is clean:           (whatever you use — golangci-lint, go vet)
[ ] Frontend builds clean:   cd frontend && npm run build
[ ] No `TODO` or `FIXME` left in NEW code paths (use grep on the diff vs. previous tag)
[ ] CHANGELOG.md updated with this version's entry
[ ] VERSION file matches the planned tag
[ ] No accidentally-committed secrets:
      git diff $(git describe --tags --abbrev=0)..HEAD -- '*.pem' '*.lic' '*.state.enc' '.env*'
    (must be empty)
[ ] Public key in pubkey_embed.pem is the SAME as last release
    (rotation is a special process — see section 9)
[ ] Models referenced in MODEL_PATH default are available in the
    bundle's models/ directory OR clearly marked as customer-supplied
```

---

## 3. Build matrix & supported platforms

We cross-compile from a Linux-amd64 build host. We do **not** support Windows in this product line.

| OS | Arch | Status | Notes |
|---|---|---|---|
| Linux | amd64 | Primary | Default deployment target |
| Linux | arm64 | Supported | For ARM servers (Graviton, Ampere) |
| macOS | arm64 | Supported | Apple Silicon dev / small-team installs |
| macOS | amd64 | On request | Intel Macs are end-of-life; we ship per ticket |

Per platform, every release produces ONE bundle:

```
ai-cfo-v<VERSION>-<OS>-<ARCH>.tar.gz
```

### Cross-compile env vars

```bash
# Linux amd64 (most common)
GOOS=linux  GOARCH=amd64 CGO_ENABLED=0 go build -ldflags='-s -w' -o build/linux-amd64/cfo-server ./cmd/server

# Linux arm64
GOOS=linux  GOARCH=arm64 CGO_ENABLED=0 go build -ldflags='-s -w' -o build/linux-arm64/cfo-server ./cmd/server

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags='-s -w' -o build/darwin-arm64/cfo-server ./cmd/server
```

`-ldflags='-s -w'` strips the binary (smaller transfer + slightly harder to reverse-engineer). `CGO_ENABLED=0` produces a fully static binary that runs on any glibc / musl Linux without surprises.

### llama.cpp binary

This is a separate per-platform compile. Either:

- Build it from the pinned `llama.cpp` submodule for each platform on its native host (cleanest), OR
- Use the upstream pre-built releases from `ggerganov/llama.cpp` and vendor them.

Whichever you pick, name the binary `bin/llama-completion` in the bundle so the customer's `run.sh` finds it. Pin a specific upstream commit; record the SHA in `CHANGELOG.md` for each release.

### Model file

`models/gemma-2-9b-it-Q4_K_M.gguf` is ~5–6 GB. Bundle it. Air-gapped customers won't be downloading it on the spot. If a customer's bandwidth allowance is constrained, we ship the model on physical media in a separate envelope.

---

## 4. What goes in the customer bundle

Final tarball layout (this MUST match `docs/ONBOARDING.md` section 1):

```
ai-cfo-v1.0.0-linux-amd64/
├── VERSION                            # plaintext: "1.0.0"
├── CHANGELOG.md                       # what changed since previous version we shipped them
├── README.md                          # = docs/ONBOARDING.md, copied & retitled
├── LICENSE.txt                        # the legal license terms (not the .lic file)
├── bin/
│   ├── cfo-server                     # main backend binary
│   ├── cfo-license                    # on-device license CLI
│   └── llama-completion               # llama.cpp inference binary, pre-compiled
├── frontend/
│   └── dist/                          # pre-built React bundle (HTML + JS + CSS)
├── models/
│   └── gemma-*.gguf                   # Gemma quantized model
├── config/
│   ├── .env.example                   # template, all keys documented
│   └── systemd/
│       └── ai-cfo.service             # optional systemd unit
├── data/
│   ├── state/.gitkeep                 # empty, ready for SQLite
│   ├── documents/.gitkeep
│   └── rag/                           # industry sample chunks (education/, ecommerce/, etc.)
├── run.sh                             # the same script we develop with
└── INSTALL.sh                         # optional one-shot installer
```

Bundle is delivered as a single `.tar.gz` with checksum + signature (see section 7).

---

## 5. What stays with us (never ships)

| Item | Reason |
|---|---|
| `config/license_privkey.pem` | Master private key. **Compromise = product compromised**. |
| `backend/cmd/license-gen/` (source) | Vendor-only tool. Customer must not be able to issue their own licenses. |
| `backend/license-gen` (binary) | Same. Build it locally, never put it in the customer bundle. |
| `AGENTS.md` | Internal architecture notes & gotchas. |
| `docs/RELEASE_PROCESS.md` (this file) | Operational internals. |
| `scripts/release.sh` | Build automation (not destructive to ship, but no reason to). |
| Source code (any `.go`, `.jsx`) | Commercial product; ship binaries only. |
| `.git/` | Source history. |
| Any `*.env` with vendor credentials | Self-explanatory. |

### The `.gitignore` is your friend, but it's not enough

`scripts/release.sh` (section 6) starts from a **fresh checkout** in a temp directory and copies in only the explicitly-listed paths. Anything not on the allowlist physically cannot make it into the bundle.

---

## 6. Producing the bundle (`scripts/release.sh`)

We have an automation script that does all of section 6 in one command. Manual ad-hoc tarballing is **banned** — the script exists specifically so a developer in a hurry can't accidentally include `license_privkey.pem`.

### Pre-flight

```bash
git checkout main
git pull
go test ./...
./run.sh test
```

### Run the script

```bash
./scripts/release.sh v1.1.0 linux amd64
# Produces:
#   dist/ai-cfo-v1.1.0-linux-amd64.tar.gz
#   dist/ai-cfo-v1.1.0-linux-amd64.tar.gz.sha256
#   dist/ai-cfo-v1.1.0-linux-amd64.tar.gz.sig   (if you wired up signing)
```

For all platforms in one go:

```bash
for plat in "linux amd64" "linux arm64" "darwin arm64"; do
  ./scripts/release.sh v1.1.0 $plat
done
```

See `scripts/release.sh` for the full pipeline. Key behaviors:

1. Verifies `git status` is clean.
2. Verifies the requested `VERSION` matches the `VERSION` file in tree.
3. Cross-compiles `cfo-server` and `cfo-license` for the target platform.
4. Builds the frontend (`npm run build`).
5. **Copies, never moves** — only explicitly-allowlisted paths land in the bundle.
6. Refuses to include `license_privkey.pem`, `license-gen` binary, or anything under `.git`.
7. Writes a SHA-256 sidecar.
8. (Optional) GPG-signs the tarball.

---

## 7. Signing & checksumming

### SHA-256 (always)

Every bundle gets a sidecar checksum file:

```
ai-cfo-v1.1.0-linux-amd64.tar.gz.sha256
```

Generated automatically by `release.sh`:

```bash
sha256sum dist/ai-cfo-v1.1.0-linux-amd64.tar.gz > dist/ai-cfo-v1.1.0-linux-amd64.tar.gz.sha256
```

Tell the customer to verify on their end:

```bash
sha256sum -c ai-cfo-v1.1.0-linux-amd64.tar.gz.sha256
```

### GPG signature (recommended for production customers)

Sign the tarball with our release-engineering GPG key (separate identity from the license-signing Ed25519 key — different purpose, different lifecycle):

```bash
gpg --detach-sign --armor dist/ai-cfo-v1.1.0-linux-amd64.tar.gz
# produces ai-cfo-v1.1.0-linux-amd64.tar.gz.asc
```

Ship the `.asc` next to the tarball. Customers running compliance audits will want this. Our GPG public key should be published on our website (or whatever channel the customer trusts) so they can verify offline.

### Why two signing systems?

| System | Used for | Lifecycle |
|---|---|---|
| Ed25519 (in `config/license_*.pem`) | License files — proves a `license.lic` was issued by us | Long-lived (rotate only if compromised) |
| GPG | Release tarballs — proves a bundle wasn't tampered with in transit | Standard 2-year rotation per industry practice |

They serve different threats. Don't combine them.

---

## 8. License issuance & renewal

### New customer

```bash
./backend/license-gen create \
    -priv config/license_privkey.pem \
    -customer "ACME Corp" \
    -customer-id ACME-001 \
    -expiry 2027-05-15 \
    -features AI_CFO,Forecasting,FinancialReports,AuditAssistant \
    -max-users 1 \
    -type Enterprise \
    -out customers/acme-001/license.lic
```

**Storage convention:** every issued license is committed to a private vendor-only repo at `customers/<customer-id>/<issue-date>-license.lic` so we can re-deliver one if the customer loses theirs. Keep history forever — these are 256-byte files; storage is irrelevant.

### Customer ID format

Choose a stable ID per customer and never change it. Format: `<SHORT>-<NNN>` (e.g. `ACME-001`). The customer ID is what locks a renewal to a specific account — if it changes, every nonce-bound state file on their host breaks.

### Feature flags

Match string-for-string with `backend/internal/license/license.go`:

| Feature constant | What it gates |
|---|---|
| `AI_CFO` | The core /ask endpoint and dashboard |
| `Forecasting` | Forecast metrics + projections |
| `FinancialReports` | Auto-generated PDF/Excel reports (future) |
| `AuditAssistant` | Audit log export + audit-trail features |

Trial customers typically get `AI_CFO` only with a 30-day expiry.

### Renewal (response to a customer's `request.dat`)

```bash
./backend/license-gen renew \
    -priv config/license_privkey.pem \
    -request inbound/acme-001-request.dat \
    -expiry 2028-05-15 \
    -features AI_CFO,Forecasting,FinancialReports,AuditAssistant \
    -out customers/acme-001/2027-04-30-renewed-license.lic
```

Send back the renewed `.lic`. The customer's flow is in `docs/ONBOARDING.md` section 8.

### Auditing license issuance

Keep an append-only log (`customers/_log.csv`) with:

```
date,customer_id,action,expiry,features,issued_by
2026-05-15,ACME-001,create,2027-05-15,AI_CFO+Forecasting,sumit
2027-04-30,ACME-001,renew,2028-05-15,AI_CFO+Forecasting+FinancialReports,sumit
```

This isn't a security control — the licenses themselves are the cryptographic record. It's an operational record for billing & support.

---

## 9. Public-key rotation policy

The Ed25519 public key is **embedded at compile time** in every customer binary. Rotating it is therefore **expensive**:

> Every existing customer must receive both a new bundle (with new embedded public key) AND a new license signed by the new private key — **in lockstep**. If they install the new bundle without the new license, their old license fails to verify. If they apply the new license to the old bundle, same thing.

**Therefore:** treat the keypair as long-lived. Rotate ONLY if:

- The private key is suspected to be compromised
- Cryptographic best practices change (Ed25519 itself is fine for the foreseeable future)
- You're cutting a major version that intentionally breaks backwards compatibility

### Emergency rotation runbook

If you DO need to rotate:

```bash
# 1. On the vendor host, generate the new keypair.
./backend/license-gen keygen
# Writes:
#   config/license_privkey.pem  (NEW private — keep!)
#   config/license_pubkey.pem   (NEW public)
#   backend/internal/license/pubkey_embed.pem  (NEW embed source)

# 2. Stash the OLD private key under a versioned filename:
mv config/license_privkey.OLD.pem config/license_privkey.YYYY-MM-DD.pem
# Do NOT delete — you'll still need it to verify any in-flight artifacts.

# 3. Build a new release bundle as in section 6.
#    This bundle's binaries now ONLY accept licenses signed by the NEW key.

# 4. Re-issue licenses for every active customer with the same expiry
#    they had. Use the new private key.
for cid in $(cut -d, -f2 customers/_log.csv | sort -u); do
    ./backend/license-gen create -priv config/license_privkey.pem \
        -customer-id "$cid" -customer "..." -expiry "..." \
        -features "..." -out customers/$cid/rotated-license.lic
done

# 5. Bundle the new license alongside the new tarball for each customer
#    and ship together — they MUST install both at the same time.

# 6. Customer-side install order (in their copy of ONBOARDING.md update note):
#    - Stop the service
#    - Replace bin/ with the new bundle's bin/
#    - Replace license.lic with the new one
#    - Start
```

We have **never had to do this** in production. The aim is to never have to.

---

## 10. Customer notification + delivery

### Channels (in order of preference)

| Channel | When |
|---|---|
| SFTP / encrypted file share | Production customers. Per-customer credentials. |
| Encrypted email (PGP or `gpg --symmetric`) | Smaller customers, ad-hoc fixes |
| Physical media (USB) | Strict air-gap environments — courier delivery |
| Signed-URL download (one-time link, 7-day expiry) | Convenience tier; the URL itself does not carry sensitive content but DO require checksum verification |

### Notification template (email)

```
Subject: AI CFO v1.1.0 — update available for ACME Corp

Hello <name>,

We're pleased to share AI CFO v1.1.0, your scheduled feature release.

  Version    : 1.1.0
  Released   : 2026-05-15
  Bundle     : ai-cfo-v1.1.0-linux-amd64.tar.gz
  Checksum   : sha256:f3a9...d12c (verify with sha256sum -c)
  Size       : 6.2 GB (model bundled)

What's new (see CHANGELOG.md inside the bundle for full notes):
  • <bullet 1>
  • <bullet 2>
  • <bullet 3>

How to install:
  See docs/ONBOARDING.md section 6 ("Applying an update we ship to you").
  Expected downtime: ~30 seconds (a single stop/start).

Your existing license, password, and data are preserved.

Download link:    <sftp-url>
Username:         <customer-handle>
Password:         (per the secure channel we agreed on)

Questions:        support@<vendor>
Urgent issues:    urgent@<vendor>

— <vendor> Release Engineering
```

### What to attach where

| Channel | Tarball | Checksum | Signature | License file |
|---|---|---|---|---|
| SFTP | ✓ | ✓ | ✓ | Separate folder, never co-located |
| Email | Link only (size) | ✓ in body | ✓ link in body | **Never** — different channel |
| USB | ✓ | ✓ | ✓ | Separate USB or paper-printed for re-typing |

---

## 11. Update compatibility guarantees

These are commitments we make to customers — implementing them correctly is what release engineering's job is.

| Customer expectation | How we honor it |
|---|---|
| "My data survives the update" | Bundle never touches `data/` during install. ONBOARDING.md instructs the customer to keep their existing `data/` directory. |
| "My license still works" | Public key in `pubkey_embed.pem` is identical across versions (see section 9). |
| "My password still works" | Password lives in `data/state/license.state.enc` which we never overwrite during update. |
| "Old documents still parse correctly" | Any parser changes are additive. Old parsing rules remain registered until a MAJOR version cut. |
| "The endpoints my custom dashboards call still respond" | New endpoints additive only in MINOR releases. Removals require a MAJOR bump AND a 90-day deprecation notice. |
| "Rolling back is fast" | The OS-level rename in ONBOARDING.md section 6 takes ~1 second. We do not auto-migrate database schemas during update — see below. |

### Database schema changes

SQLite migrations are a hazard zone. Rules:

- **PATCH releases**: no schema changes, period.
- **MINOR releases**: schema changes are **additive only** (new tables, new optional columns with defaults). The previous version's binary must be able to read the new schema (forward compat for rollback).
- **MAJOR releases**: schema breaks are OK but require a one-way migration step we run on first boot. Always print a log message: `[migrate] applying schema migration to v2`.

Implementation: see `backend/internal/storage/sqlstore/migrations.go` — every migration is a numbered up-only script.

---

## 12. Rollback & hot-fix process

### Customer-initiated rollback

The customer's rollback is the rename-the-directory dance in ONBOARDING.md section 6. We do not need to do anything from our side; their license + data work as-is with the older binaries.

### Vendor-initiated hot-fix

If we discover a serious bug in a released version:

1. Cut a PATCH release (e.g. 1.1.0 → 1.1.1) within 24 h.
2. Bundle as usual via `release.sh`.
3. Email all active customers with the urgency channel.
4. CHANGELOG must clearly say "**Security**" or "**Critical bug**" so customers know it's not skippable.

### What we DON'T do

- We don't push updates automatically. The product has no auto-update mechanism. **The customer chooses when to install.** This is non-negotiable for on-prem deployments.
- We don't reach into the customer's machine to apply fixes ourselves. Even on urgent support calls, we walk them through it.

---

## 13. Common release pitfalls

Real things that have bitten us or will bite us. Internalize them.

### "I forgot to bump the VERSION file"

The release.sh script verifies `VERSION` matches the argument. If you pass `v1.1.0` but `VERSION` still says `1.0.0`, the script aborts. **Trust the script — don't `--force`.**

### "I built with `CGO_ENABLED=1` and now the binary needs glibc 2.35"

Always cross-compile with `CGO_ENABLED=0`. Verify the binary's dynamic linker:

```bash
file bin/cfo-server
# Should say: "statically linked"
ldd bin/cfo-server
# Should say: "not a dynamic executable"
```

### "The license-gen binary made it into the bundle"

This is the worst-case bug. Mitigation: `release.sh` ONLY copies explicit paths into the bundle. It never `cp -r` an entire directory that might contain `license-gen`. As a defense-in-depth check, the script also runs:

```bash
find dist/<bundle> -name 'license-gen' -o -name '*privkey*' -o -name '*.git*'
# Must produce ZERO output.
```

If this find prints anything, the script deletes the bundle and exits non-zero.

### "I rotated the embedded public key by accident"

`release.sh` checksums `backend/internal/license/pubkey_embed.pem` and compares against the previous release's checksum (stored in `dist/.last-pubkey-sha256`). On mismatch, the script halts and demands you confirm intentional rotation with `--allow-pubkey-rotation`.

### "The customer's antivirus quarantined the binary"

Most enterprise AV won't whitelist arbitrary Go binaries. Mitigations:

- Ship with our GPG signature so their security team can verify provenance.
- Sign the Linux binary with `signify` if their environment supports it.
- For Windows-based customers — refuse the engagement; we don't support that platform.

### "I tagged the release before verifying tests"

The release.sh runs `go test ./...` and `./run.sh test` as the FIRST step. Never bypass this with `--skip-tests`. There's no such flag — we removed it on purpose.

### "I shipped to the wrong customer"

Have a second pair of eyes verify the customer-id in `customer/<id>/license.lic` matches the destination email/SFTP credential. License files have customer info in plaintext — `./backend/license-gen show -in license.lic` confirms before shipping.

### "The new bundle is 30% larger and we didn't change anything"

This is almost always Go debug symbols. Always build with `-ldflags='-s -w'`. The `release.sh` already does this, but if you're hand-building for diagnostics, remember.

### "Customer's clock is wrong and migration fails"

Migration files have a 30-day window and ±5min skew tolerance. If customer's NTP is broken on a long-running air-gapped host, migrations fail with `clock_skew`. Tell them to `date -s` to a correct time before retrying.

---

## Appendix — Quick reference: per-release operator checklist

```
[ ] Pre-release checks pass (section 2)
[ ] VERSION file updated
[ ] CHANGELOG.md updated
[ ] git tag v<X.Y.Z> created and pushed
[ ] For each (OS, ARCH) in build matrix:
      [ ] ./scripts/release.sh vX.Y.Z <os> <arch>
      [ ] dist/.../tar.gz.sha256 produced
      [ ] dist/.../tar.gz.asc produced (if GPG enabled)
      [ ] Bundle inspection: no license-gen, no privkey, no .git
[ ] Per active customer:
      [ ] License still valid for ≥ 30 days, OR renewal already scheduled
      [ ] Delivery channel confirmed
      [ ] Notification email drafted (template above)
[ ] Update internal `customers/_log.csv` with the release date column
[ ] Post-release: monitor support@ for the next 72 h
```

---

## Appendix — Files & paths cheat sheet

| Path | Lifecycle |
|---|---|
| `VERSION` | Single source of truth for version string |
| `CHANGELOG.md` | Append-only release notes |
| `config/license_privkey.pem` | Vendor private key. **Never commit.** Back up offline. |
| `config/license_pubkey.pem` | Vendor public key. Committed. Same as embed. |
| `backend/internal/license/pubkey_embed.pem` | Compile-time embed of the public key. Same content as above. |
| `customers/<id>/*.lic` | All licenses we've ever issued, kept forever |
| `customers/_log.csv` | Operational audit trail of issuance |
| `dist/` | Build output. Gitignored. |
| `scripts/release.sh` | The only legitimate way to build a customer bundle |

— end —
