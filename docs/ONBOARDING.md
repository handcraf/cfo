# AI CFO — Client Onboarding Guide

Welcome. This document walks you through everything from unboxing your AI CFO bundle to running the product daily, applying updates we ship you, and migrating to a new server. The product is fully on-premises and air-gapped — nothing in this guide requires internet access after the initial bundle is on your machine.

If you're a non-technical IT admin: every command in this guide is copy-paste. You will not need to write any code.

---

## Table of contents

1. [What you've received](#1-what-youve-received)
2. [System requirements](#2-system-requirements)
3. [One-time installation](#3-one-time-installation)
4. [Daily usage](#4-daily-usage)
5. [Backups](#5-backups)
6. [Applying an update we ship to you](#6-applying-an-update-we-ship-to-you)
7. [Migrating to a new server](#7-migrating-to-a-new-server)
8. [Renewing your license](#8-renewing-your-license)
9. [Troubleshooting](#9-troubleshooting)
10. [Privacy & data handling](#10-privacy--data-handling)
11. [Support & escalation](#11-support--escalation)

---

## 1. What you've received

You will get **two separate deliveries** from us. They are kept separate on purpose so the license file is never sitting next to a copy of the binary on some shared drive.

### Delivery A — the install bundle

A single archive named like `ai-cfo-v<version>-<os>-<arch>.tar.gz`, for example:

```
ai-cfo-v1.0.0-linux-amd64.tar.gz
```

Inside (paths shown after extraction):

| Path | What it is | Customer touches it? |
|---|---|---|
| `bin/cfo-server` | The backend HTTP API (Go binary, statically linked) | No, just runs |
| `bin/cfo-license` | The on-device license CLI (`status`, `deactivate`, `activate`, `export-request`) | Yes — for migration & renewal |
| `bin/llama-completion` | The local LLM inference binary (llama.cpp), pre-compiled for your platform | No |
| `models/gemma-*.gguf` | The Gemma language model (quantized, ~5–8 GB) | No |
| `frontend/dist/` | The pre-built React UI (HTML + JS + CSS, no Node.js required at runtime) | No |
| `data/` | Empty data directories with `.gitkeep` placeholders | Your data will accumulate here |
| `config/.env.example` | Template for environment variables (ports, paths) | Copy to `.env` and edit |
| `run.sh` | The one-stop start/stop/status/test script | Yes, every day |
| `INSTALL.sh` | One-shot first-time installer | Run once |
| `VERSION` | Plain-text version string (e.g. `1.0.0`) | Read-only |
| `CHANGELOG.md` | What changed since the previous version we shipped you | Read-only |
| `README.md` | This guide + the architecture reference | Read-only |

### Delivery B — your license

A single file named `license.lic`, signed cryptographically by us specifically for your organization. It contains your customer name, ID, expiry, and the features you've purchased. **Do not edit it** — any change (even reformatting whitespace) invalidates the cryptographic signature and the product will refuse to start.

Optionally we may also send `LICENSE_INSTRUCTIONS.txt` with your customer ID, expiry date, and license type printed in plain text for your records.

### What we do **NOT** send

| Item | Why |
|---|---|
| Source code | This is a compiled commercial product |
| Vendor private key | Stays only with us; used to sign all licenses |
| `license-gen` CLI | Internal tool — used by us to issue licenses |
| Any telemetry agent | The product makes zero outbound network calls |

---

## 2. System requirements

### Minimum (for trial / development)

| Resource | Spec |
|---|---|
| CPU | 4 cores, x86_64 or arm64 |
| RAM | 16 GB |
| Disk | 20 GB free (model is ~7 GB; your documents add to this) |
| OS | Linux (Ubuntu 22.04+, RHEL 9+, Debian 12+) or macOS 13+ |
| Network | None required after install |

### Recommended (production)

| Resource | Spec |
|---|---|
| CPU | 8+ cores, AVX2 instruction set (any Intel/AMD chip from the last ~6 years) |
| RAM | 32 GB |
| Disk | 100 GB SSD (logs + audit history grow over time) |
| GPU | Optional. NVIDIA with ≥8 GB VRAM substantially speeds up LLM responses but is not required |

### Ports

The product listens on three local TCP ports by default. None need to be open to the internet — they're only used between the UI and the backend on the same machine.

| Port | Service | Required? |
|---|---|---|
| 8080 | Backend HTTP API | Yes |
| 3000 | Frontend UI | Yes |
| 8081 | Optional LLM HTTP shim | Only if `START_LLAMA_SERVER=true` in your `.env` |

If any of these conflict with another service on your machine, you can change them in your `.env` file — see step 3.5.

---

## 3. One-time installation

These steps run **once**, the day you set up the product. After this, daily usage is just `./run.sh start`.

### 3.1 Extract the bundle

Put the bundle somewhere stable — not `/tmp`, not a removable drive. A home folder or `/opt` works well.

```bash
sudo mkdir -p /opt/ai-cfo
sudo chown $USER:$USER /opt/ai-cfo
cd /opt/ai-cfo
tar -xzf ~/Downloads/ai-cfo-v1.0.0-linux-amd64.tar.gz
ls -la
```

You should now see `bin/`, `frontend/`, `models/`, `data/`, `run.sh`, etc.

### 3.2 Drop your license file in place

Copy the `license.lic` we sent you (Delivery B) into the install directory next to `run.sh`:

```bash
cp ~/Downloads/license.lic /opt/ai-cfo/license.lic
chmod 600 /opt/ai-cfo/license.lic
```

### 3.3 Make scripts executable

```bash
chmod +x run.sh bin/cfo-server bin/cfo-license bin/llama-completion
```

### 3.4 Configure environment (optional)

If the defaults (ports 8080/3000/8081, data in `./data/`) suit you, **skip this step** entirely.

Otherwise:

```bash
cp config/.env.example .env
nano .env       # or vim, or any editor
```

The most common things to change:

```bash
# Network — change if you have a port conflict
PORT=8080
FRONTEND_PORT=3000

# Storage — change if you want data on a different volume (e.g. a backup drive)
DATA_DIR=/opt/ai-cfo/data

# LLM — leave these alone unless you know what you're doing
MODEL_PATH=/opt/ai-cfo/models/gemma-2-9b-it-Q4_K_M.gguf
```

### 3.5 First start

```bash
./run.sh start
```

You should see output like:

```
INFO  Launching Go backend…
OK    backend listening on :8080
INFO  Launching Vite frontend…  (or static server, depending on bundle)
OK    frontend listening on :3000
[license] OK: customer=ACME Corp type=Enterprise expires=2027-05-15 (365 days remaining)
OK    Stack is up. Open http://localhost:3000 in your browser.
```

If the license line says `OK` and the ports bind, you're done.

### 3.6 Open the UI and set your password

In a browser on the same machine, visit:

```
http://localhost:3000
```

You'll see a **first-time setup screen** asking you to choose a password. Pick something at least 6 characters. This is the password you (and anyone else who needs access on this machine) will use every day.

There is no "forgot password" link — see [Troubleshooting](#9-troubleshooting) for the reset procedure.

After setting the password, you land on the dashboard. **That's the entire install.**

### 3.7 Verify the install (recommended)

From a separate terminal on the same machine:

```bash
./run.sh license status
```

You should see your customer details, your expiry date, the machine fingerprint, and `STATUS : valid — N days remaining`.

Write down or photograph the **machine ID** shown. You'll need it if you ever contact support.

---

## 4. Daily usage

### Start

```bash
cd /opt/ai-cfo
./run.sh start
```

### Stop

```bash
./run.sh stop
```

### Check status

```bash
./run.sh status
```

### View logs (when something looks wrong)

```bash
./run.sh logs backend         # backend application log
./run.sh logs frontend        # UI server log
./run.sh logs llama-server    # LLM log (only if you enabled it)
```

### Access the UI

Open `http://localhost:3000` in any browser on the same machine. Log in with your password. Ask financial questions, upload documents (P&L, balance sheets, CSVs, PDFs), and let the system explain your numbers.

### Run as a service (recommended for production)

If you want the product to survive reboots, wrap `./run.sh start` in a systemd unit. A starter is included:

```bash
sudo cp config/systemd/ai-cfo.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now ai-cfo
sudo systemctl status ai-cfo
```

> If `config/systemd/ai-cfo.service` isn't in your bundle, ask support — we can send it; it's a five-line file.

---

## 5. Backups

The product writes everything important under one directory (your `DATA_DIR`, default `./data/`). Backing up that directory plus your `license.lic` is sufficient.

### What to back up

| Path | Why |
|---|---|
| `data/state/cfo.db` | SQLite database — companies, documents, parsed line items, ask history |
| `data/documents/` | Original uploaded files |
| `data/state/license.state.enc` | Activation state + your password hash. Tied to THIS machine — see notes below |
| `license.lic` | Your signed license file (we have a copy on file too, but recovery is faster if you have it) |

### What does NOT need backup

- `bin/`, `frontend/`, `models/` — re-extract from the install bundle if you ever lose them.
- `logs/` — purely informational; old entries can be deleted any time.

### Suggested cadence

| Schedule | What | Where |
|---|---|---|
| Hourly | `data/state/cfo.db` | Local backup volume |
| Daily | Full `data/` directory + `license.lic` | Off-host backup (USB, NAS) |
| Weekly | Same, plus the install bundle archive | Secure long-term storage |

### Important: state file is machine-bound

`data/state/license.state.enc` is encrypted with a key derived from this machine's fingerprint. **It cannot be restored to a different machine.** If you're rebuilding the same server (same hardware), restoring it works. If you're migrating to new hardware, use the [migration flow](#7-migrating-to-a-new-server) instead.

---

## 6. Applying an update we ship to you

When we release a new version (bug fixes, new features, new industry modules), we send you a new tarball:

```
ai-cfo-v1.1.0-linux-amd64.tar.gz
```

Updates are designed to be **non-destructive**. Your data, license, and password are preserved across the update.

### Update steps

```bash
# 1. Stop the running service.
cd /opt/ai-cfo
./run.sh stop

# 2. Back up data + license (insurance, takes one minute).
tar -czf ~/ai-cfo-backup-$(date +%F).tar.gz data/ license.lic .env 2>/dev/null

# 3. Move the OLD install aside, don't delete it yet.
cd /opt
sudo mv ai-cfo ai-cfo.v1.0.0

# 4. Extract the NEW bundle in place.
sudo mkdir -p ai-cfo
sudo chown $USER:$USER ai-cfo
cd ai-cfo
tar -xzf ~/Downloads/ai-cfo-v1.1.0-linux-amd64.tar.gz

# 5. Carry over YOUR files from the old install.
cp ../ai-cfo.v1.0.0/license.lic .
cp -r ../ai-cfo.v1.0.0/data/. ./data/
[ -f ../ai-cfo.v1.0.0/.env ] && cp ../ai-cfo.v1.0.0/.env .

# 6. Make scripts executable (new binaries replace old ones).
chmod +x run.sh bin/*

# 7. Start.
./run.sh start

# 8. Verify the version + license.
cat VERSION                # should print 1.1.0
./run.sh license status     # should still say valid; your customer/expiry unchanged

# 9. Smoke-test by opening http://localhost:3000 and logging in.
#    Use the SAME password you set during the original install.

# 10. Only after you're confident the new version works, remove the old install:
sudo rm -rf /opt/ai-cfo.v1.0.0
```

### What the update CAN change

- New API endpoints, new UI pages, new metrics, new industries
- Improved retrieval / better answers
- Bug fixes and performance work
- LLM model upgrades (if `models/` differs between versions — we'll tell you in CHANGELOG.md)

### What the update will NEVER do

- Force you to install a new `license.lic`. Your existing one continues to work for the duration of its expiry.
- Lose any of your existing data.
- Require an internet connection.
- Change your password.
- Re-bind to a different machine.

### Rolling back

If something looks off after an update, roll back in five seconds:

```bash
cd /opt
./ai-cfo/run.sh stop
sudo mv ai-cfo ai-cfo.broken
sudo mv ai-cfo.v1.0.0 ai-cfo       # the directory you renamed in step 3 above
./ai-cfo/run.sh start
```

Then email support with the contents of `ai-cfo.broken/logs/` attached.

---

## 7. Migrating to a new server

You may move AI CFO between physical machines as often as you need. There is no machine-count limit. The process uses cryptographically signed migration tokens to ensure the license remains bound to exactly one host at a time.

### On the OLD machine

```bash
cd /opt/ai-cfo
./run.sh license deactivate
# Outputs:
#   migration.dat          <- signed migration token
#   migration_pub.key      <- public key the NEW host needs to verify the token
./run.sh stop
```

After deactivation, the old host's backend will refuse to start. This is by design.

### Transfer

Copy these three files to the new machine (USB, SCP, encrypted email — your call):

1. The full install bundle (or extract it on the new machine from your archive)
2. `license.lic`
3. `migration.dat` and `migration_pub.key`
4. (optional but recommended) Your `data/` directory if you want to keep your historical document uploads and ask history.

### On the NEW machine

```bash
# 1. Install the bundle as in section 3 (one-time installation).
# 2. Place license.lic in /opt/ai-cfo/.
# 3. Activate:
./run.sh license activate /path/to/migration.dat -pub /path/to/migration_pub.key
# Outputs: OK   license activated on this machine.

# 4. Start.
./run.sh start
```

That's the entire migration. The new machine is now bound, the old machine is dormant, your license expiry and feature set carry over exactly.

### Re-binding back to the original machine

Sometimes you migrate, decide you wanted to stay on the original, and want to swing back. Same flow in reverse:

```bash
# NEW machine
./run.sh license deactivate
# ORIGINAL machine
./run.sh license activate <migration.dat> -pub <migration_pub.key>
```

### Migration security guarantees

- The migration token is signed; cannot be forged
- Each token has a nonce — using the same token twice on the same target machine is refused as a replay
- Tokens older than 30 days are refused (don't let one sit around)
- Tokens timestamped more than 5 minutes in the future are refused (clock-skew check)

---

## 8. Renewing your license

Your license has an expiry date. The product warns you in the UI and in the logs starting **30 days before** expiry. After expiry, the backend refuses to serve business endpoints — but the license-status and login endpoints stay reachable so you can fix it.

### Step 1 — Export a renewal request

On the customer side (you):

```bash
./run.sh license export-request -out request.dat
```

This writes a small JSON file containing your customer ID, the current expiry date, and the machine ID it's bound to.

### Step 2 — Send the request to us

Email `request.dat` to your account manager (or whatever channel we've agreed on). It contains no sensitive data — feel free to copy/paste its contents into a ticket if that's easier.

### Step 3 — Receive a renewed license

We send back a new `license.lic` signed for the extended expiry date and any feature changes you've purchased.

### Step 4 — Install the renewed license

```bash
cd /opt/ai-cfo
./run.sh stop
cp ~/Downloads/license.lic ./license.lic   # overwrite the old one
./run.sh start
./run.sh license status                     # confirm the new expiry
```

### Renewal does NOT require

- Internet access
- Reinstalling the product
- Re-entering your password
- Migrating your data
- A maintenance window beyond ~30 seconds for stop+start

---

## 9. Troubleshooting

### The UI shows a "License Required" page

The backend rejected the license. Read the **Code** shown on the screen and consult this table:

| Code | What it means | Fix |
|---|---|---|
| `file_missing` | `license.lic` isn't in the install directory | Copy `license.lic` next to `run.sh`, restart |
| `expired` | The license's expiry date has passed | Run section 8 (renewal) |
| `machine_mismatch` | This license is bound to a different machine | Run section 7 (migration) FROM the original host |
| `bad_signature` | The file has been edited or corrupted | Restore from your backup, or request a fresh copy from support |
| `bad_format` | The file is not valid JSON | Same as above |
| `no_public_key` | Build misconfiguration in the binary | Contact support immediately; your install bundle is broken |
| `clock_skew` | Migration token is too old or your system clock is wrong | Check `date`, fix the clock, re-run deactivate/activate |
| `replay_detected` | The same migration token was already used here | Generate a fresh migration token from the source host |

### "I forgot my password"

There is no email recovery for the password (the product is offline by design). To reset, wipe the encrypted state file and restart — you'll lose your **session and your password**, but **NOT your data, license, or settings**:

```bash
./run.sh stop
rm -f data/state/license.state.enc
./run.sh start
# Open http://localhost:3000 — it will show the first-time setup screen again.
```

You'll re-bind the license to this machine (same fingerprint, so it's seamless) and pick a new password.

### "The backend won't start: address already in use"

Another service is on the same port. Either:

```bash
./run.sh stop                    # kills our leftover processes
lsof -nP -iTCP:8080 -sTCP:LISTEN # shows what else is on 8080
```

…or change the port in `.env` (`PORT=8090`) and restart.

### "Answers are slow"

The LLM is doing real work on your CPU. Expected latency on a 32 GB host with a Q4 quantized model:

- First-token: 2–8 seconds
- Full answer: 15–60 seconds depending on question complexity

If you have a GPU, run with `LLAMA_THREADS=0` and ensure the bundle includes a GPU-enabled `llama-completion` binary (we ship CPU-only by default; ask support for the GPU variant).

### "The system answers questions about data I haven't uploaded"

It shouldn't. This is a bug — please open a support ticket. Include:

1. The exact question
2. The full JSON response (open browser dev tools → Network → click `/api/ask` → Response tab → copy)
3. The output of `./run.sh license status`
4. The first 200 lines of `./run.sh logs backend` from around the time of the question

### "I see 'Uncertain — conflicting values' more than I expect"

The conflict detector is conservative on purpose. If you upload two documents (e.g. an old draft P&L and a final P&L) with different numbers for the same metric, the system will refuse to commit to an answer until you remove the stale one. To see exactly what's conflicting, expand the **Evidence** section in the UI under the answer.

---

## 10. Privacy & data handling

### Where your data lives

100% on this machine. The product has no telemetry, no analytics, no "phone home" beacons, no automatic update checks. You can verify this with `lsof -i` while it's running — the only listeners are localhost ports.

### What we (the vendor) have access to

- The license you were issued — we keep a record of the **customer ID, expiry, features, and license type**.
- Nothing else. We do not have a copy of your data, your password, your uploaded documents, or your question history.

### What's in our logs

The backend writes:

- `logs/backend.log` — every HTTP request (method + path + status, no bodies), plus license-check outcomes.
- SQLite `ask_audit` table — every question asked, the metrics used to answer it, and the response. This is **your** audit log; it stays on **your** disk.

Both are append-only by default. You may rotate them with standard tools (`logrotate`, etc.).

### Disk encryption

We strongly recommend running this product on an encrypted filesystem (LUKS on Linux, FileVault on macOS). The product's own state encryption is defense against the casual attacker — it's not a replacement for OS-level disk encryption.

---

## 11. Support & escalation

### Before contacting support

Have these three things ready:

1. **Version**: `cat /opt/ai-cfo/VERSION`
2. **License status**: `./run.sh license status` (copy the output verbatim, including the machine ID)
3. **Recent logs**: `./run.sh logs backend | tail -200`

### How to reach us

| Channel | When to use it |
|---|---|
| Email: `support@<your-vendor-domain>` | Standard questions, feature requests, renewal requests |
| Email: `urgent@<your-vendor-domain>` | Production-down (license rejection blocking your work) |
| Phone: (see your contract) | Sev-1 outages during business hours |

We commit to:

- 1 business day response on standard support
- 4-hour response on urgent
- Renewed license delivered within 1 business day of receiving a complete `request.dat`

### What we can help with remotely (and what we can't)

| ✅ We can | ❌ We can't (without you physically reading them to us) |
|---|---|
| Re-issue a license | See your data |
| Sign a migration approval | Read your password |
| Walk you through update apply | Diagnose without your logs (you must share them) |
| Build a custom industry module | Operate your machine for you |

---

## Appendix — fast reference card

```bash
# Start / stop
./run.sh start
./run.sh stop
./run.sh status

# Logs
./run.sh logs backend
./run.sh logs frontend

# License
./run.sh license status
./run.sh license deactivate                  # before moving to a new machine
./run.sh license activate <migration.dat>    # on the new machine
./run.sh license export-request              # before renewal expiry

# Reset password (keeps data + license)
./run.sh stop
rm -f data/state/license.state.enc
./run.sh start
# then re-set password in the UI

# Apply an update (sections 6 above)
./run.sh stop
# extract new bundle, carry over license.lic + data/ + .env
./run.sh start
```

Keep this guide. Future updates may extend it; we'll note that in the CHANGELOG.
