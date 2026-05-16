// Package config — environment-driven settings.
//
// Defaults are chosen so `docker-compose up` works out of the box with no
// env vars set:
//   - SQLite at data/state/cfo.db
//   - In-memory vectors (no Qdrant)
//   - llama.cpp at ./llama.cpp/main, Gemma GGUF at ./models/gemma.gguf
//
// LLM runtime: the platform shells out to a locally built llama.cpp
// binary with a Gemma GGUF model. Ollama is no longer a dependency.
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the backend.
type Config struct {
	Port    string
	DataDir string

	// ---- LLM runtime (llama.cpp + Gemma) ------------------------------

	// LlamaCppBinary is the path to the compiled llama.cpp executable.
	// Typically "./llama.cpp/main" on the host or "/app/llama.cpp/main"
	// inside the Docker image.
	LlamaCppBinary string

	// ModelPath is the absolute (or cwd-relative) path to the Gemma GGUF
	// weights file, e.g. "./models/gemma-7b-q4.gguf".
	ModelPath string

	// ModelName is kept for logging/telemetry compatibility only. The
	// actual model used is whatever ModelPath points at.
	// Deprecated: retained so downstream docs/UI referring to "model
	// name" don't break. Will be removed once the UI migrates.
	ModelName string

	// LLMMaxTokens caps generation length per ask.
	LLMMaxTokens int

	// LLMTemperature and LLMTopP tune sampling. Defaults are low to keep
	// the CFO narration deterministic.
	LLMTemperature float32
	LLMTopP        float32

	// LLMSeed fixes the sampler. -1 disables. Default 42.
	LLMSeed int

	// LLMContextSize is the model's context window in tokens.
	LLMContextSize int

	// LLMTimeoutSec is the hard wall-clock limit for one generation.
	LLMTimeoutSec int

	// LLMThreads passes through to llama.cpp `-t`. 0 = llama.cpp decides.
	LLMThreads int

	// ---- Legacy Ollama embedding fallback -----------------------------

	// OllamaHost is ONLY used by the optional langchaingo-based embedding
	// service. The LLM runtime no longer uses it. Default: "" (disabled).
	// Set this only if you deliberately run an Ollama server on the side
	// for embeddings.
	//
	// TODO: Replace with llama.cpp embedding mode (`llama-embedding`)
	// and drop this field entirely.
	OllamaHost string

	// ---- Stage 3 storage ---------------------------------------------

	SQLiteEnabled    bool
	SQLitePath       string
	VectorBackend    string
	QdrantURL        string
	QdrantCollection string
	QdrantAPIKey     string
	EmbeddingDim     int
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	dataDir := getEnv("DATA_DIR", "./data")
	return &Config{
		Port:    getEnv("PORT", "8080"),
		DataDir: dataDir,

		LlamaCppBinary: getEnv("LLAMA_CPP_BINARY", "./llama.cpp/main"),
		ModelPath:      getEnv("MODEL_PATH", "./models/gemma.gguf"),
		ModelName:      getEnv("MODEL_NAME", "gemma"),
		LLMMaxTokens:   getEnvInt("LLM_MAX_TOKENS", 512),
		LLMTemperature: getEnvFloat("LLM_TEMPERATURE", 0.2),
		LLMTopP:        getEnvFloat("LLM_TOP_P", 0.9),
		LLMSeed:        getEnvInt("LLM_SEED", 42),
		LLMContextSize: getEnvInt("LLM_CONTEXT_SIZE", 4096),
		LLMTimeoutSec:  getEnvInt("LLM_TIMEOUT_SEC", 120),
		LLMThreads:     getEnvInt("LLM_THREADS", 0),

		OllamaHost: getEnv("OLLAMA_HOST", ""),

		SQLiteEnabled:    getEnvBool("SQLITE_ENABLED", true),
		SQLitePath:       getEnv("SQLITE_PATH", dataDir+"/state/cfo.db"),
		VectorBackend:    strings.ToLower(getEnv("VECTOR_BACKEND", "memory")),
		QdrantURL:        getEnv("QDRANT_URL", "http://qdrant:6333"),
		QdrantCollection: getEnv("QDRANT_COLLECTION", "cfo_chunks"),
		QdrantAPIKey:     getEnv("QDRANT_API_KEY", ""),
		EmbeddingDim:     getEnvInt("EMBEDDING_DIM", 768),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func getEnvInt(key string, defaultValue int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return n
}

func getEnvFloat(key string, defaultValue float32) float32 {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return defaultValue
	}
	return float32(f)
}
