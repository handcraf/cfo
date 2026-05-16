# AI CFO — Local Run Guide (llama.cpp + Gemma)

This guide shows how to run the AI CFO platform fully offline, using
`llama.cpp` with a Gemma GGUF model as the local LLM runtime.

There is **no Ollama, no cloud API, no Python** in this path.

---

## 1. Install llama.cpp

Clone and build from source. You need a C++ toolchain (`make`, `clang`
or `g++`) and `cmake`.

```bash
git clone https://github.com/ggerganov/llama.cpp
cd llama.cpp

# Apple Silicon (Metal GPU acceleration):
cmake -B build -DGGML_METAL=ON
cmake --build build --config Release -j

# Intel Mac / Linux (CPU only):
# cmake -B build && cmake --build build --config Release -j
```

The current llama.cpp tree (build ≥ b9000) ships TWO completion binaries:

| Binary             | Mode                          | Use it for…                                     |
| ------------------ | ----------------------------- | ----------------------------------------------- |
| `llama-cli`        | Interactive chat (REPL)       | Manual prompting from your terminal             |
| `llama-completion` | **Single-shot completion**    | **What the backend invokes** — exits when done  |
| `llama-server`    | HTTP daemon                   | Future optimization (the `llama-server` TODO)   |

For the CFO backend you want **`llama-completion`**. Symlink it as
`main` so the project default `LLAMA_CPP_BINARY=./llama.cpp/main` works:

```bash
ln -sf build/bin/llama-completion main
./main --version                                        # sanity check
```

> Or set `LLAMA_CPP_BINARY=./llama.cpp/build/bin/llama-completion` in
> your env — same effect, no symlink needed.
>
> **Do NOT symlink `llama-cli`.** It does not support `-no-cnv` in
> recent builds and will refuse to run single-shot — the backend will
> hang forever on any model that ships a chat template (Gemma, LLaMA-3,
> Mistral-instruct, …).

CLI flags used by the backend: `-m`, `-f`, `-n`, `--temp`, `--top-p`,
`-c`, `--seed`, `-t`, `--no-display-prompt`, **`-no-cnv`** (mandatory).

TODO: Pin a specific llama.cpp release tag once we've validated one
against the full CFO test suite.

---

## 2. Download the Gemma GGUF model

Use a quantized build that fits your host RAM. Pick one row:

| Host RAM | Recommended GGUF                                                                | Size   |
| -------- | ------------------------------------------------------------------------------- | ------ |
| 8 GB     | `ggml-org/gemma-3-4b-it-GGUF` → `gemma-3-4b-it-Q4_K_M.gguf`                      | ~2.5 GB |
| 16 GB    | `bartowski/gemma-2-9b-it-GGUF` → `gemma-2-9b-it-Q4_K_M.gguf`                     | ~5.5 GB |
| **32 GB** | **`ggml-org/gemma-3-12b-it-GGUF` → `gemma-3-12b-it-Q4_K_M.gguf`** *(validated)* | ~7 GB   |
| 64 GB+   | `unsloth/gemma-4-31B-it-GGUF` → `gemma-4-31B-it-Q4_K_M.gguf`                     | ~18 GB  |

Gemma models are **gated on Hugging Face**. You must:

1. Create a read token: https://huggingface.co/settings/tokens
2. Acknowledge the license on the model page in your browser
3. Authenticate via the new `hf` CLI (the old `huggingface-cli` is
   deprecated and prints a warning telling you to use `hf`):

```bash
brew install huggingface-cli      # actually installs `hf`
hf auth login                     # paste token when prompted
hf auth whoami                    # verify username appears
```

Download the chosen GGUF — example uses the 32 GB tier (Gemma 3 12B):

```bash
mkdir -p backend/models
hf download ggml-org/gemma-3-12b-it-GGUF \
  --include "gemma-3-12b-it-Q4_K_M.gguf" \
  --local-dir backend/models

mv backend/models/gemma-3-12b-it-Q4_K_M.gguf backend/models/gemma.gguf
du -h backend/models/gemma.gguf   # expect multi-GB, not an HTML error
```

> The flag `--local-dir-use-symlinks False` from older guides is the
> default in `hf 1.x` and can be omitted.
>
> A 7 GB download over a typical home connection takes ~10-15 minutes.

### Smoke test the model before running the backend

```bash
./llama.cpp/main \
  -m ./backend/models/gemma.gguf \
  -p "Say hello in one sentence." \
  -n 64 --temp 0.2 --top-p 0.9 --seed 42 -c 4096 \
  --no-display-prompt -no-cnv
```

You should see a short generated sentence and `[end of text]`. On
Apple Silicon you'll also see `MTL` / Metal logs confirming GPU offload.
Process must **exit cleanly** — if it drops into a `>` chat prompt, you
forgot `-no-cnv`.

---

## 3. Configure the backend

Default env vars (already set in `docker-compose.yml`):

| Env var            | Default                  | Purpose                             |
| ------------------ | ------------------------ | ----------------------------------- |
| `LLAMA_CPP_BINARY` | `/app/llama.cpp/main`    | Path to the compiled binary         |
| `MODEL_PATH`       | `/app/models/gemma.gguf` | Path to the GGUF weights            |
| `LLM_MAX_TOKENS`   | `512`                    | Caps generation length per ask      |
| `LLM_TEMPERATURE`  | `0.2`                    | Low for deterministic CFO narration |
| `LLM_TOP_P`        | `0.9`                    | Nucleus sampling                    |
| `LLM_SEED`         | `42`                     | Fixes sampler (-1 disables)         |
| `LLM_CONTEXT_SIZE` | `4096`                   | Model context window                |
| `LLM_TIMEOUT_SEC`  | `120`                    | Hard wall-clock limit per ask       |
| `LLM_THREADS`      | `0`                      | CPU threads (0 = auto)              |

Override from the host for local dev:

```bash
export LLAMA_CPP_BINARY=./llama.cpp/main
export MODEL_PATH=./backend/models/gemma.gguf
```

---

## 4. Run

### Docker Compose (recommended)

```bash
docker compose up --build
```

Includes the backend and frontend. Qdrant is opt-in:

```bash
docker compose --profile qdrant up --build
```

### Bare metal (backend only)

```bash
cd backend
LLAMA_CPP_BINARY=../llama.cpp/main \
MODEL_PATH=../backend/models/gemma.gguf \
go run ./cmd/server
```

---

## 5. Verify the LLM wiring

Once the server is up:

```bash
curl -s -X POST http://localhost:8080/ask \
  -H 'Content-Type: application/json' \
  -d '{"question":"What was profit last quarter?"}' | jq .
```

On a healthy setup you should see a `summary` + `explanation` pair
along with `confidence`, `conflicts`, and `evidence` fields. If the
LLM fails, the API returns a `summary` of:

> Unable to generate explanation. Please try again.

and a populated `error` field. This is the documented fallback — the
deterministic metrics and evidence fields are still returned.

---

## 6. Troubleshooting

| Symptom                                    | Likely cause                            | Fix                                              |
| ------------------------------------------ | --------------------------------------- | ------------------------------------------------ |
| `llm: binary not found at …`               | `LLAMA_CPP_BINARY` path wrong           | Verify with `ls` and re-export                   |
| `llm: model file unreadable at …`          | `MODEL_PATH` wrong / download incomplete | Check `du -h` of the file                        |
| Process killed during generation (OOM)     | Quantization too heavy for host RAM     | Use a smaller quant (e.g. `Q4_0` / `Q3_K_M`)     |
| Timeout after 120s                         | CPU too slow for 7B Q4                  | Lower `LLM_MAX_TOKENS`; try a 2B model           |
| Garbled `<end_of_turn>` text in output     | Older llama.cpp build                   | Rebuild llama.cpp from `main`                    |

---

## 7. Constraints recap

- No cloud APIs.
- No Python in the runtime path.
- LLM is used ONLY to explain numbers that the backend has already
  computed from SQL. Calculations live in `internal/service/financial_logic.go`.
- Prompt + sampler are deterministic so the ask-audit log is reproducible.

TODO: Performance — replace per-request `exec.Command` with a
long-lived `llama-server` process and talk to it over local HTTP.
Deferred until baseline latency is measured.
