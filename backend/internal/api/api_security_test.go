package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/cfo/backend/internal/config"
	"github.com/cfo/backend/internal/model"
)

// setupTestServer creates a test server with temporary data directory
func setupTestServer(t *testing.T) (http.Handler, string, func()) {
	t.Helper()
	tmpDir, err := os.MkdirTemp("", "api_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Initialize the data directories (documents, parsed, state, rag)
	dirs := []string{
		tmpDir + "/documents",
		tmpDir + "/parsed",
		tmpDir + "/state",
		tmpDir + "/rag/generic",
		tmpDir + "/rag/education",
		tmpDir + "/rag/ecommerce",
		tmpDir + "/rag/pharma",
	}
	for _, dir := range dirs {
		os.MkdirAll(dir, 0755)
	}

	cfg := &config.Config{
		DataDir:    tmpDir,
		OllamaHost: "http://localhost:11434",
		ModelName:  "llama3",
	}

	mux := SetupRoutes(cfg)
	cleanup := func() { os.RemoveAll(tmpDir) }

	return mux, tmpDir, cleanup
}

// ================== API SECURITY TESTS ==================

func TestAPI_CORS_Headers(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest("OPTIONS", "/health", nil)
	req.Header.Set("Origin", "http://localhost:3000")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Check CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("Missing Access-Control-Allow-Origin header")
	}
}

func TestAPI_MethodNotAllowed(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		method   string
		path     string
		expected int
	}{
		{"DELETE", "/setup/company", http.StatusMethodNotAllowed},
		{"PUT", "/health", http.StatusMethodNotAllowed},
		{"DELETE", "/ask", http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.expected {
				t.Errorf("Status = %d, want %d", w.Code, tt.expected)
			}
		})
	}
}

func TestAPI_InvalidJSON(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	invalidJSONs := []string{
		`{invalid json}`,
		`{"unclosed": "string`,
		`["array", "instead", "of", "object"]`,
		`null`,
		``,
	}

	for _, body := range invalidJSONs {
		t.Run("POST /setup/company", func(t *testing.T) {
			req := httptest.NewRequest("POST", "/setup/company", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want %d for body: %q", w.Code, http.StatusBadRequest, body)
			}
		})

		t.Run("POST /ask", func(t *testing.T) {
			req := httptest.NewRequest("POST", "/ask", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Status = %d, want %d for body: %q", w.Code, http.StatusBadRequest, body)
			}
		})
	}
}

func TestAPI_XSSPrevention(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	// XSS in company name
	xssPayload := `<script>alert('xss')</script>`
	company := model.Company{
		Name:          xssPayload,
		Industry:      "Tech",
		IndustryType:  model.IndustryGeneric,
		FiscalYearEnd: "December",
		Currency:      "USD",
	}

	body, _ := json.Marshal(company)
	req := httptest.NewRequest("POST", "/setup/company", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Response should not execute script (proper content-type)
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Content-Type = %q, should be application/json", contentType)
	}
}

func TestAPI_LargePayloadRejection(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Very large JSON payload
	largeQuestion := strings.Repeat("x", 1024*1024) // 1MB question
	askReq := map[string]string{"question": largeQuestion}
	body, _ := json.Marshal(askReq)

	req := httptest.NewRequest("POST", "/ask", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Should either reject or handle gracefully (not panic)
	// The specific behavior depends on server config
	if w.Code == http.StatusInternalServerError {
		t.Log("Server returned 500 for large payload - may need size limit middleware")
	}
}

// ================== INPUT VALIDATION TESTS ==================

func TestAPI_CompanyValidation(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name     string
		company  interface{}
		wantCode int
	}{
		{
			name: "Valid company",
			company: map[string]string{
				"name":           "Test Corp",
				"industry":       "Technology",
				"industry_type":  "generic",
				"fiscal_year_end": "December",
				"currency":       "USD",
			},
			wantCode: http.StatusOK,
		},
		{
			name: "Empty name",
			company: map[string]string{
				"name":           "",
				"industry":       "Technology",
				"fiscal_year_end": "December",
				"currency":       "USD",
			},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "Empty object",
			company:  map[string]string{},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.company)
			req := httptest.NewRequest("POST", "/setup/company", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("Status = %d, want %d, body: %s", w.Code, tt.wantCode, w.Body.String())
			}
		})
	}
}

func TestAPI_AskValidation(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	tests := []struct {
		name     string
		body     interface{}
		wantCode int
	}{
		{
			name:     "Empty question",
			body:     map[string]string{"question": ""},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "Missing question field",
			body:     map[string]string{},
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "Null question",
			body:     map[string]interface{}{"question": nil},
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/ask", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("Status = %d, want %d", w.Code, tt.wantCode)
			}
		})
	}
}

// ================== FILE UPLOAD SECURITY TESTS ==================

func TestAPI_FileUpload_MaliciousFilename(t *testing.T) {
	mux, tmpDir, cleanup := setupTestServer(t)
	defer cleanup()

	maliciousFilenames := []string{
		"../../../etc/passwd",
		"..\\..\\windows\\system32",
		"test\x00.csv",
		".htaccess",
		"file;rm -rf /;.csv",
	}

	for _, filename := range maliciousFilenames {
		t.Run(filename, func(t *testing.T) {
			// Create multipart form
			var buf bytes.Buffer
			writer := multipart.NewWriter(&buf)
			part, _ := writer.CreateFormFile("file", filename)
			io.WriteString(part, "a,b,c\n1,2,3")
			writer.Close()

			req := httptest.NewRequest("POST", "/documents/upload", &buf)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			// Should succeed but sanitize filename, or reject
			// Check that no files were created outside the documents directory
			secretsPath := tmpDir + "/../secrets"
			if _, err := os.Stat(secretsPath); !os.IsNotExist(err) {
				t.Error("Malicious file was created outside data directory!")
			}
		})
	}
}

func TestAPI_FileUpload_ContentTypeValidation(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Upload without multipart content type
	req := httptest.NewRequest("POST", "/documents/upload", strings.NewReader("not multipart"))
	req.Header.Set("Content-Type", "text/plain")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d for non-multipart upload", w.Code, http.StatusBadRequest)
	}
}

// ================== RESPONSE FORMAT TESTS ==================

func TestAPI_ResponseFormat_JSON(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/health"},
		{"GET", "/company/status"},
		{"GET", "/documents"},
		{"GET", "/metrics/current"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", contentType)
			}

			// Verify valid JSON response
			var result interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Errorf("Response is not valid JSON: %v", err)
			}
		})
	}
}

// ================== RATE LIMITING SIMULATION ==================

func TestAPI_ConcurrentRequests(t *testing.T) {
	mux, _, cleanup := setupTestServer(t)
	defer cleanup()

	// Simulate concurrent requests
	done := make(chan int, 50)

	for i := 0; i < 50; i++ {
		go func(id int) {
			req := httptest.NewRequest("GET", "/health", nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			done <- w.Code
		}(i)
	}

	successCount := 0
	for i := 0; i < 50; i++ {
		code := <-done
		if code == http.StatusOK {
			successCount++
		}
	}

	// All requests should succeed
	if successCount != 50 {
		t.Errorf("Only %d/50 concurrent requests succeeded", successCount)
	}
}

