package config

import (
	"os"
	"testing"
)

// ensureCleanEnv clears every env var Load reads so defaults win.
func ensureCleanEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"PORT", "DATA_DIR",
		"LLAMA_CPP_BINARY", "MODEL_PATH", "MODEL_NAME",
		"LLM_MAX_TOKENS", "LLM_TEMPERATURE", "LLM_TOP_P",
		"LLM_SEED", "LLM_CONTEXT_SIZE", "LLM_TIMEOUT_SEC", "LLM_THREADS",
		"OLLAMA_HOST",
		"SQLITE_ENABLED", "SQLITE_PATH",
		"VECTOR_BACKEND", "QDRANT_URL", "QDRANT_COLLECTION", "QDRANT_API_KEY", "EMBEDDING_DIM",
	}
	for _, k := range keys {
		os.Unsetenv(k)
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	ensureCleanEnv(t)

	cfg := Load()

	stringCases := []struct {
		name     string
		got      string
		expected string
	}{
		{"Port", cfg.Port, "8080"},
		{"DataDir", cfg.DataDir, "./data"},
		{"LlamaCppBinary", cfg.LlamaCppBinary, "./llama.cpp/main"},
		{"ModelPath", cfg.ModelPath, "./models/gemma.gguf"},
		{"ModelName", cfg.ModelName, "gemma"},
		// Ollama is deprecated; default is empty (embeddings disabled).
		{"OllamaHost", cfg.OllamaHost, ""},
	}
	for _, tt := range stringCases {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}

	if cfg.LLMMaxTokens != 512 {
		t.Errorf("LLMMaxTokens = %d, want 512", cfg.LLMMaxTokens)
	}
	if cfg.LLMTemperature != 0.2 {
		t.Errorf("LLMTemperature = %v, want 0.2", cfg.LLMTemperature)
	}
	if cfg.LLMTopP != 0.9 {
		t.Errorf("LLMTopP = %v, want 0.9", cfg.LLMTopP)
	}
	if cfg.LLMSeed != 42 {
		t.Errorf("LLMSeed = %d, want 42", cfg.LLMSeed)
	}
	if cfg.LLMContextSize != 4096 {
		t.Errorf("LLMContextSize = %d, want 4096", cfg.LLMContextSize)
	}
	if cfg.LLMTimeoutSec != 120 {
		t.Errorf("LLMTimeoutSec = %d, want 120", cfg.LLMTimeoutSec)
	}
}

func TestLoad_FromEnvironment(t *testing.T) {
	ensureCleanEnv(t)

	os.Setenv("PORT", "9090")
	os.Setenv("DATA_DIR", "/custom/data")
	os.Setenv("LLAMA_CPP_BINARY", "/opt/llama/main")
	os.Setenv("MODEL_PATH", "/opt/models/gemma-7b.gguf")
	os.Setenv("MODEL_NAME", "gemma-7b")
	os.Setenv("LLM_MAX_TOKENS", "256")
	os.Setenv("LLM_TEMPERATURE", "0.1")
	os.Setenv("LLM_TOP_P", "0.8")
	os.Setenv("OLLAMA_HOST", "http://localhost:11434")
	defer ensureCleanEnv(t)

	cfg := Load()

	if cfg.Port != "9090" {
		t.Errorf("Port = %q, want 9090", cfg.Port)
	}
	if cfg.DataDir != "/custom/data" {
		t.Errorf("DataDir = %q, want /custom/data", cfg.DataDir)
	}
	if cfg.LlamaCppBinary != "/opt/llama/main" {
		t.Errorf("LlamaCppBinary = %q, want /opt/llama/main", cfg.LlamaCppBinary)
	}
	if cfg.ModelPath != "/opt/models/gemma-7b.gguf" {
		t.Errorf("ModelPath = %q, want /opt/models/gemma-7b.gguf", cfg.ModelPath)
	}
	if cfg.ModelName != "gemma-7b" {
		t.Errorf("ModelName = %q, want gemma-7b", cfg.ModelName)
	}
	if cfg.LLMMaxTokens != 256 {
		t.Errorf("LLMMaxTokens = %d, want 256", cfg.LLMMaxTokens)
	}
	if cfg.LLMTemperature != 0.1 {
		t.Errorf("LLMTemperature = %v, want 0.1", cfg.LLMTemperature)
	}
	if cfg.LLMTopP != 0.8 {
		t.Errorf("LLMTopP = %v, want 0.8", cfg.LLMTopP)
	}
	if cfg.OllamaHost != "http://localhost:11434" {
		t.Errorf("OllamaHost = %q, want http://localhost:11434", cfg.OllamaHost)
	}
}

func TestLoad_PartialEnvironment(t *testing.T) {
	ensureCleanEnv(t)
	os.Setenv("PORT", "3000")
	os.Setenv("MODEL_PATH", "/tmp/phi.gguf")
	defer ensureCleanEnv(t)

	cfg := Load()

	if cfg.Port != "3000" {
		t.Errorf("Port = %q, want 3000", cfg.Port)
	}
	if cfg.ModelPath != "/tmp/phi.gguf" {
		t.Errorf("ModelPath = %q, want /tmp/phi.gguf", cfg.ModelPath)
	}
	// Unspecified values stay at defaults.
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data", cfg.DataDir)
	}
	if cfg.LlamaCppBinary != "./llama.cpp/main" {
		t.Errorf("LlamaCppBinary = %q, want ./llama.cpp/main", cfg.LlamaCppBinary)
	}
	if cfg.OllamaHost != "" {
		t.Errorf("OllamaHost = %q, want empty default", cfg.OllamaHost)
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		defaultVal string
		envValue   string
		setEnv     bool
		expected   string
	}{
		{name: "Returns default when not set", key: "TEST_VAR_UNSET", defaultVal: "default", setEnv: false, expected: "default"},
		{name: "Returns env value when set", key: "TEST_VAR_SET", defaultVal: "default", envValue: "custom", setEnv: true, expected: "custom"},
		{name: "Returns default when empty string", key: "TEST_VAR_EMPTY", defaultVal: "default", envValue: "", setEnv: true, expected: "default"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setEnv {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}
			if got := getEnv(tt.key, tt.defaultVal); got != tt.expected {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.defaultVal, got, tt.expected)
			}
		})
	}
}

func TestGetEnvInt_DefaultAndOverride(t *testing.T) {
	os.Unsetenv("CFO_TEST_INT")
	if got := getEnvInt("CFO_TEST_INT", 7); got != 7 {
		t.Errorf("default path = %d, want 7", got)
	}
	os.Setenv("CFO_TEST_INT", "11")
	defer os.Unsetenv("CFO_TEST_INT")
	if got := getEnvInt("CFO_TEST_INT", 7); got != 11 {
		t.Errorf("override path = %d, want 11", got)
	}
	os.Setenv("CFO_TEST_INT", "not-a-number")
	if got := getEnvInt("CFO_TEST_INT", 7); got != 7 {
		t.Errorf("bad parse fallback = %d, want 7", got)
	}
}

func TestGetEnvFloat_DefaultAndOverride(t *testing.T) {
	os.Unsetenv("CFO_TEST_FLOAT")
	if got := getEnvFloat("CFO_TEST_FLOAT", 0.5); got != 0.5 {
		t.Errorf("default = %v, want 0.5", got)
	}
	os.Setenv("CFO_TEST_FLOAT", "0.123")
	defer os.Unsetenv("CFO_TEST_FLOAT")
	if got := getEnvFloat("CFO_TEST_FLOAT", 0.5); got < 0.122 || got > 0.124 {
		t.Errorf("override = %v, want ~0.123", got)
	}
}

func TestConfig_NotNil(t *testing.T) {
	cfg := Load()
	if cfg == nil {
		t.Fatal("Load() returned nil")
	}
}
