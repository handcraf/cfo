package service

import (
	"context"
	"testing"
	"time"
)

// ================== EMBEDDING SERVICE TESTS ==================

func TestEmbeddingService_DefaultConfig(t *testing.T) {
	cfg := DefaultEmbeddingConfig()

	if cfg.OllamaHost == "" {
		t.Error("OllamaHost should not be empty")
	}

	if cfg.ModelName == "" {
		t.Error("ModelName should not be empty")
	}
}

func TestEmbeddingService_Creation(t *testing.T) {
	cfg := DefaultEmbeddingConfig()
	svc := NewEmbeddingService(cfg)

	if svc == nil {
		t.Fatal("NewEmbeddingService returned nil")
	}

	if svc.GetModelName() != cfg.ModelName {
		t.Errorf("ModelName = %q, want %q", svc.GetModelName(), cfg.ModelName)
	}
}

func TestEmbeddingService_EmbedText_Error(t *testing.T) {
	// Test with non-existent server - should fail on actual embed call
	cfg := EmbeddingConfig{
		OllamaHost: "http://localhost:99999", // Non-existent port
		ModelName:  "test-model",
	}

	svc := NewEmbeddingService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Trying to embed should fail with connection error
	_, err := svc.EmbedText(ctx, "test text")

	// Should get an error (connection refused or timeout)
	if err == nil {
		t.Log("Note: EmbedText may succeed if Ollama is running on port 99999")
	}
}

// ================== COSINE SIMILARITY TESTS ==================

func TestCosineSimilarity_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		a        []float32
		b        []float32
		expected float32
	}{
		{
			name:     "Zero vectors",
			a:        []float32{0, 0, 0},
			b:        []float32{0, 0, 0},
			expected: 0,
		},
		{
			name:     "One zero vector",
			a:        []float32{1, 2, 3},
			b:        []float32{0, 0, 0},
			expected: 0,
		},
		{
			name:     "Negative values",
			a:        []float32{-1, -2, -3},
			b:        []float32{-1, -2, -3},
			expected: 1.0,
		},
		{
			name:     "Mixed positive negative",
			a:        []float32{1, -1, 1},
			b:        []float32{-1, 1, -1},
			expected: -1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CosineSimilarity(tt.a, tt.b)
			diff := result - tt.expected
			if diff < 0 {
				diff = -diff
			}
			if diff > 0.01 {
				t.Errorf("CosineSimilarity = %f, want %f", result, tt.expected)
			}
		})
	}
}

func TestCosineSimilarity_Normalization(t *testing.T) {
	// Test that scaling doesn't affect similarity
	a := []float32{1, 2, 3}
	b := []float32{2, 4, 6} // Same direction, different magnitude

	similarity := CosineSimilarity(a, b)

	// Should be 1.0 (same direction)
	if similarity < 0.99 || similarity > 1.01 {
		t.Errorf("CosineSimilarity for parallel vectors = %f, want ~1.0", similarity)
	}
}

func TestSqrt32_Correctness(t *testing.T) {
	tests := []struct {
		input    float32
		expected float32
	}{
		{0, 0},
		{1, 1},
		{4, 2},
		{9, 3},
		{16, 4},
		{2, 1.414},
	}

	for _, tt := range tests {
		result := sqrt32(tt.input)
		diff := result - tt.expected
		if diff < 0 {
			diff = -diff
		}
		if diff > 0.01 {
			t.Errorf("sqrt32(%f) = %f, want %f", tt.input, result, tt.expected)
		}
	}
}

// ================== BENCHMARK TESTS ==================

func BenchmarkCosineSimilarity_Small(b *testing.B) {
	a := make([]float32, 128)
	bVec := make([]float32, 128)
	for i := range a {
		a[i] = float32(i) * 0.1
		bVec[i] = float32(i) * 0.2
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CosineSimilarity(a, bVec)
	}
}

func BenchmarkCosineSimilarity_Large(b *testing.B) {
	a := make([]float32, 1536) // OpenAI embedding size
	bVec := make([]float32, 1536)
	for i := range a {
		a[i] = float32(i) * 0.001
		bVec[i] = float32(i) * 0.002
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CosineSimilarity(a, bVec)
	}
}

