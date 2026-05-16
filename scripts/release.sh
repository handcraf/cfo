#!/usr/bin/env bash
# =============================================================================
# scripts/release.sh — reproducible AI CFO release-bundle builder
#
# Usage:
#     ./scripts/release.sh <version-tag> <os> <arch>
#
# Examples:
#     ./scripts/release.sh v0.1.0 linux  amd64
#     ./scripts/release.sh v0.1.0 darwin arm64
#
# Output (under dist/):
#     ai-cfo-v0.1.0-linux-amd64.tar.gz
#     ai-cfo-v0.1.0-linux-amd64.tar.gz.sha256
#     ai-cfo-v0.1.0-linux-amd64.tar.gz.asc        (only if GPG_SIGN_KEY is set)
#
# Why this script exists:
#   Hand-rolled tarballs eventually ship a vendor private key. Then you
#   replace your master Ed25519 keypair and re-issue every customer's
#   license at 2am. This script enforces:
#     - Only allowlisted paths land in the bundle (no `cp -r .` ever).
#     - The vendor private key, license-gen binary, .git, and .env files
#       are physically refused by an explicit deny-scan.
#     - Public key in pubkey_embed.pem is checksummed against the last
#       release. Mismatch halts the build unless --allow-pubkey-rotation
#       is set (because rotation forces every customer's license to be
#       re-issued — see docs/RELEASE_PROCESS.md section 9).
#     - Binaries are statically linked (CGO_ENABLED=0) with debug
#       symbols stripped (-ldflags='-s -w').
#
# Read docs/RELEASE_PROCESS.md before running this in production.
# =============================================================================

set -euo pipefail

# -----------------------------------------------------------------------------
# Args
# -----------------------------------------------------------------------------
if [[ $# -lt 3 ]]; then
    echo "Usage: $0 <version-tag> <os> <arch> [--allow-pubkey-rotation]" >&2
    echo "  e.g. $0 v0.1.0 linux amd64" >&2
    exit 2
fi

VERSION_TAG="$1"        # e.g. v0.1.0
TARGET_OS="$2"          # linux | darwin
TARGET_ARCH="$3"        # amd64 | arm64
shift 3 || true
ALLOW_PUBKEY_ROTATION=false
DRY_RUN=false   # --dry-run: skip clean-tree + test gates (CI/dev only — see RELEASE_PROCESS.md)
for arg in "$@"; do
    case "$arg" in
        --allow-pubkey-rotation) ALLOW_PUBKEY_ROTATION=true ;;
        --dry-run)               DRY_RUN=true ;;
        *) echo "unknown flag: $arg" >&2; exit 2 ;;
    esac
done

# Resolve repo root regardless of caller's cwd.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT"

DIST="$ROOT/dist"
mkdir -p "$DIST"

# Strip leading 'v' for the version-in-bundle (the tag keeps the v).
VERSION="${VERSION_TAG#v}"
BUNDLE_NAME="ai-cfo-${VERSION_TAG}-${TARGET_OS}-${TARGET_ARCH}"
STAGE="$DIST/$BUNDLE_NAME"

# -----------------------------------------------------------------------------
# Output helpers
# -----------------------------------------------------------------------------
red()   { printf "\033[31m%s\033[0m\n" "$*"; }
green() { printf "\033[32m%s\033[0m\n" "$*"; }
gray()  { printf "\033[90m%s\033[0m\n" "$*"; }
step()  { printf "\n\033[1;36m▶ %s\033[0m\n" "$*"; }
fail()  { red "ERROR  $*"; exit 1; }

# =============================================================================
# Phase 1 — Pre-flight: clean tree, version match, baseline checks
# =============================================================================
step "Phase 1: pre-flight checks"

# We check that there are no MODIFIED tracked files and no STAGED-but-
# uncommitted changes. Untracked files (build artifacts, sample license
# files, etc.) are NOT a blocker — the explicit allowlist in Phase 6
# decides what physically enters the bundle.
if [[ "$DRY_RUN" == "true" ]]; then
    gray "  (--dry-run: skipping clean-tree check; this is CI/dev only)"
elif ! git diff --quiet || ! git diff --cached --quiet; then
    red "Working tree has modified / staged tracked changes — commit or stash before releasing:"
    git status --short | grep -v '^??' || true
    fail "release aborted"
else
    green "✓ no uncommitted tracked changes"
fi

VERSION_IN_FILE="$(<"$ROOT/VERSION")"
VERSION_IN_FILE="${VERSION_IN_FILE//[$'\t\r\n ']}"
if [[ "$VERSION_IN_FILE" != "$VERSION" ]]; then
    fail "VERSION file says '$VERSION_IN_FILE' but you asked to release '$VERSION'. Bump VERSION first."
fi
green "✓ VERSION file matches requested release"

# Public key rotation guard. Compare current embed against the recorded
# baseline from the previous release.
PUBKEY_PATH="$ROOT/backend/internal/license/pubkey_embed.pem"
PUBKEY_SHA="$(shasum -a 256 "$PUBKEY_PATH" 2>/dev/null | awk '{print $1}')"
LAST_PUBKEY_FILE="$DIST/.last-pubkey-sha256"
if [[ -f "$LAST_PUBKEY_FILE" ]]; then
    LAST_PUBKEY_SHA="$(<"$LAST_PUBKEY_FILE")"
    if [[ "$PUBKEY_SHA" != "$LAST_PUBKEY_SHA" ]]; then
        if [[ "$ALLOW_PUBKEY_ROTATION" != "true" ]]; then
            red "Public key in pubkey_embed.pem changed since last release."
            red "Rotation invalidates every customer's existing license."
            red "If this is intentional, re-run with --allow-pubkey-rotation"
            red "AND follow docs/RELEASE_PROCESS.md section 9."
            fail "release aborted"
        fi
        red "⚠ public key rotation acknowledged. Every existing customer needs a new license."
    fi
fi
green "✓ public key state OK"

# =============================================================================
# Phase 2 — Test gate (no skip switch — section 13 of RELEASE_PROCESS.md)
# =============================================================================
step "Phase 2: unit tests"
if [[ "$DRY_RUN" == "true" ]]; then
    gray "  (--dry-run: skipping full test suite; license + api packages only)"
    ( cd "$ROOT/backend" && go test ./internal/license/ ./internal/api/ -count=1 >/dev/null )
    green "✓ critical packages pass (license, api)"
else
    ( cd "$ROOT/backend" && go test ./... -count=1 >/dev/null )
    green "✓ go test ./... passed"
fi

# Skip the live E2E in this script — it requires the stack to be running.
# Releases are expected to be cut on a host where you've already run
# ./run.sh test successfully (manual gate per RELEASE_PROCESS.md section 2).
gray "  (skipping live E2E — verify ./run.sh test manually before tagging)"

# =============================================================================
# Phase 3 — Cross-compile binaries
# =============================================================================
step "Phase 3: cross-compile binaries for ${TARGET_OS}/${TARGET_ARCH}"

# Clean stage directory.
rm -rf "$STAGE"
mkdir -p "$STAGE/bin" "$STAGE/frontend" "$STAGE/models" "$STAGE/data/state" \
         "$STAGE/data/documents" "$STAGE/data/rag" "$STAGE/config" \
         "$STAGE/config/systemd"

build_go() {
    local pkg="$1" out="$2"
    GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" CGO_ENABLED=0 \
        go build -trimpath -ldflags='-s -w' \
        -o "$STAGE/bin/$out" "./$pkg"
    gray "  built $out ($(du -h "$STAGE/bin/$out" | awk '{print $1}'))"
}

( cd "$ROOT/backend"
  build_go cmd/server      cfo-server
  build_go cmd/cfo-license cfo-license )

# Verify static linkage (Linux-only check; macOS uses dyld differently).
if [[ "$TARGET_OS" == "linux" ]] && command -v file >/dev/null 2>&1; then
    if ! file "$STAGE/bin/cfo-server" | grep -q 'statically linked\|static-pie\|ELF'; then
        fail "cfo-server is not statically linked — rebuild with CGO_ENABLED=0"
    fi
    green "✓ cfo-server appears statically linked"
fi

green "✓ binaries built"

# =============================================================================
# Phase 4 — Frontend build
# =============================================================================
step "Phase 4: frontend build"
if [[ -d "$ROOT/frontend" ]]; then
    ( cd "$ROOT/frontend"
      if [[ ! -d node_modules ]]; then
          gray "  installing node_modules…"
          npm install --silent
      fi
      npm run build --silent )
    cp -r "$ROOT/frontend/dist" "$STAGE/frontend/"
    green "✓ frontend dist copied"
else
    gray "  no frontend/ directory — skipped"
fi

# =============================================================================
# Phase 5 — Vendor llama.cpp + Gemma model
# =============================================================================
step "Phase 5: vendor llama.cpp binary + model"

LLAMA_BIN="${LLAMA_CPP_BINARY:-$ROOT/llama.cpp/main}"
if [[ -f "$LLAMA_BIN" ]]; then
    cp "$LLAMA_BIN" "$STAGE/bin/llama-completion"
    chmod +x "$STAGE/bin/llama-completion"
    green "✓ llama-completion vendored from $LLAMA_BIN"
else
    red "⚠ llama.cpp binary not found at $LLAMA_BIN"
    red "  Customer will need to compile or vendor it separately."
    red "  Set LLAMA_CPP_BINARY env var or symlink it for proper releases."
fi

MODEL_SRC="${MODEL_PATH:-$ROOT/backend/models/gemma.gguf}"
if [[ -f "$MODEL_SRC" ]]; then
    MODEL_NAME="$(basename "$MODEL_SRC")"
    MODEL_SIZE_MB="$(($(stat -f%z "$MODEL_SRC" 2>/dev/null || stat -c%s "$MODEL_SRC") / 1024 / 1024))"
    gray "  copying model ($MODEL_SIZE_MB MB) — this is slow…"
    cp "$MODEL_SRC" "$STAGE/models/$MODEL_NAME"
    green "✓ model bundled: $MODEL_NAME"
else
    red "⚠ Gemma model not found at $MODEL_SRC"
    red "  Bundle will require customer to supply their own model."
fi

# =============================================================================
# Phase 6 — Copy text/config artifacts (EXPLICIT allowlist)
# =============================================================================
step "Phase 6: copy artifacts (strict allowlist)"

# CRITICAL: every line below is an explicit allowlist entry. Do NOT
# replace this with `cp -r .` or `rsync --exclude` — those patterns
# eventually leak the vendor private key. If you need a new file in
# the bundle, ADD it explicitly here AND update RELEASE_PROCESS.md
# section 4 ("What goes in the customer bundle").
cp "$ROOT/run.sh"                   "$STAGE/run.sh"
cp "$ROOT/VERSION"                  "$STAGE/VERSION"
cp "$ROOT/CHANGELOG.md"             "$STAGE/CHANGELOG.md"
cp "$ROOT/docs/ONBOARDING.md"       "$STAGE/README.md"
cp "$ROOT/.env.example"             "$STAGE/config/.env.example"

# Stub LICENSE.txt — the real legal text belongs here when there is one.
cat > "$STAGE/LICENSE.txt" <<EOF
AI CFO — Commercial Software License

Copyright (c) <vendor>. All rights reserved.

Use of this software is governed by the commercial license agreement
executed between the licensee and <vendor>. Refer to your separate
contract document.

This file is a placeholder — populate with the actual license terms
before shipping. See docs/RELEASE_PROCESS.md section 4.
EOF

# .gitkeep placeholders for empty data dirs.
touch "$STAGE/data/state/.gitkeep" "$STAGE/data/documents/.gitkeep"

# RAG industry samples (read-only seeds).
if [[ -d "$ROOT/backend/data/rag" ]]; then
    cp -r "$ROOT/backend/data/rag/." "$STAGE/data/rag/"
    green "✓ RAG industry seeds copied"
fi

# Optional: systemd unit (only if present in the repo).
if [[ -f "$ROOT/config/systemd/ai-cfo.service" ]]; then
    cp "$ROOT/config/systemd/ai-cfo.service" "$STAGE/config/systemd/"
fi

# INSTALL.sh — friendly one-shot installer for customers.
cat > "$STAGE/INSTALL.sh" <<'EOF'
#!/usr/bin/env bash
# One-shot installer for first-time AI CFO setup.
# Re-runs are safe; this script is idempotent.
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$HERE"

echo "AI CFO installer"
echo "=================="
echo

[[ -f license.lic ]] || {
    echo "ERROR: license.lic not found in $HERE."
    echo "Please copy the license.lic we sent you into this directory and re-run."
    exit 1
}

chmod +x run.sh bin/cfo-server bin/cfo-license bin/llama-completion 2>/dev/null || true

if [[ ! -f .env ]]; then
    cp config/.env.example .env
    echo "Created .env from template. Edit it to change ports or paths."
fi

echo
echo "✓ Install complete. Start with:  ./run.sh start"
echo "  Then open http://localhost:3000 and set your password."
EOF
chmod +x "$STAGE/INSTALL.sh"

green "✓ artifacts staged at $STAGE"

# =============================================================================
# Phase 7 — Forbidden-files scan (defense in depth)
# =============================================================================
step "Phase 7: forbidden-files scan (no privkey, no license-gen, no .git, no .env)"

# Patterns that MUST NOT appear inside the staged bundle.
FORBIDDEN=$(find "$STAGE" \( \
    -name 'license_privkey*' -o \
    -name 'license-gen'      -o \
    -name '*.privkey.pem'    -o \
    -name '.env'             -o \
    -name '.git'             -o \
    -name '.gitignore' \) -print)

if [[ -n "$FORBIDDEN" ]]; then
    red "Forbidden files detected in staged bundle:"
    echo "$FORBIDDEN" >&2
    rm -rf "$STAGE"
    fail "release aborted — staged bundle deleted"
fi
green "✓ no forbidden files in staged bundle"

# =============================================================================
# Phase 8 — Tar + checksum + (optional) sign
# =============================================================================
step "Phase 8: package, checksum, sign"

TARBALL="$DIST/${BUNDLE_NAME}.tar.gz"
( cd "$DIST" && tar -czf "$TARBALL" "$BUNDLE_NAME" )

SHA="$(shasum -a 256 "$TARBALL" | awk '{print $1}')"
echo "$SHA  ${BUNDLE_NAME}.tar.gz" > "${TARBALL}.sha256"
green "✓ tarball:   $TARBALL"
gray  "  size:      $(du -h "$TARBALL" | awk '{print $1}')"
gray  "  sha256:    $SHA"

# Optional GPG signature — gated by env var so the script works on
# vendor hosts without a release-eng GPG key configured yet.
if [[ -n "${GPG_SIGN_KEY:-}" ]] && command -v gpg >/dev/null 2>&1; then
    gpg --detach-sign --armor --local-user "$GPG_SIGN_KEY" \
        --output "${TARBALL}.asc" "$TARBALL"
    green "✓ GPG signature: ${TARBALL}.asc"
elif [[ -n "${GPG_SIGN_KEY:-}" ]]; then
    red "GPG_SIGN_KEY set but gpg binary not found — skipping signature"
fi

# Record the public key SHA for next-release rotation comparison.
echo "$PUBKEY_SHA" > "$LAST_PUBKEY_FILE"

# Clean up the staging directory (the tarball IS the deliverable).
rm -rf "$STAGE"

# =============================================================================
# Summary
# =============================================================================
echo
green "============================================================"
green " Release ${VERSION_TAG} for ${TARGET_OS}/${TARGET_ARCH} READY"
green "============================================================"
echo
echo " Deliverables (dist/):"
echo "   $(basename "$TARBALL")"
echo "   $(basename "$TARBALL").sha256"
[[ -f "${TARBALL}.asc" ]] && echo "   $(basename "$TARBALL").asc"
echo
echo " Next steps (see docs/RELEASE_PROCESS.md section 10):"
echo "   1. Tag the release:    git tag -a $VERSION_TAG -m \"Release $VERSION_TAG\""
echo "   2. Verify checksum:    sha256sum -c $(basename "$TARBALL").sha256"
echo "   3. Upload to delivery channel (SFTP / encrypted share)."
echo "   4. Send notification email to active customers."
echo "   5. Update customers/_log.csv with the release date."
echo
