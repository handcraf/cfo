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
	"time"

	"github.com/cfo/backend/internal/config"
	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// Test setup helpers
func setupTestEnv(t *testing.T) (*config.Config, *storage.FileStore, func()) {
	tmpDir, err := os.MkdirTemp("", "cfo_api_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	err = storage.InitDirectories(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init directories: %v", err)
	}

	cfg := &config.Config{
		Port:       "8080",
		DataDir:    tmpDir,
		OllamaHost: "http://0.0.0.0:11434",
		ModelName:  "llama3",
	}

	store := storage.NewFileStore(tmpDir)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return cfg, store, cleanup
}

// ================== HEALTH HANDLER TESTS ==================

func TestHealthHandler_Health(t *testing.T) {
	handler := NewHealthHandler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("Expected status 'ok', got %v", result["status"])
	}
}

func TestHealthHandler_MethodNotAllowed(t *testing.T) {
	handler := NewHealthHandler()

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	w := httptest.NewRecorder()

	handler.Health(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", resp.StatusCode)
	}
}

// ================== COMPANY HANDLER TESTS ==================

func TestCompanyHandler_SetupCompany(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewCompanyHandler(store, cfg)

	company := model.Company{
		Name:          "Test Corp",
		Industry:      "Technology",
		FiscalYearEnd: "December",
		Currency:      "USD",
	}

	body, _ := json.Marshal(company)
	req := httptest.NewRequest(http.MethodPost, "/setup/company", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.SetupCompany(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result model.Company
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Name != "Test Corp" {
		t.Errorf("Expected name 'Test Corp', got %s", result.Name)
	}

	if !result.SetupCompleted {
		t.Error("SetupCompleted should be true")
	}
}

func TestCompanyHandler_SetupCompany_WithIndustryType(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewCompanyHandler(store, cfg)

	tests := []struct {
		name         string
		industryType model.IndustryType
	}{
		{"Generic", model.IndustryGeneric},
		{"Education", model.IndustryEducation},
		{"Ecommerce", model.IndustryEcommerce},
		{"Pharma", model.IndustryPharma},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			company := model.Company{
				Name:         "Test " + tt.name + " Corp",
				IndustryType: tt.industryType,
			}

			body, _ := json.Marshal(company)
			req := httptest.NewRequest(http.MethodPost, "/setup/company", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.SetupCompany(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status 200, got %d", resp.StatusCode)
				return
			}

			var result model.Company
			json.NewDecoder(resp.Body).Decode(&result)

			if result.IndustryType != tt.industryType {
				t.Errorf("IndustryType = %s, want %s", result.IndustryType, tt.industryType)
			}
		})
	}
}

func TestCompanyHandler_SetupCompany_SecurityXSS(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewCompanyHandler(store, cfg)

	// Test XSS in company name
	company := model.Company{
		Name:     "<script>alert('xss')</script>Test Corp",
		Industry: "<img src=x onerror=alert('xss')>",
	}

	body, _ := json.Marshal(company)
	req := httptest.NewRequest(http.MethodPost, "/setup/company", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.SetupCompany(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Should accept but store as-is (XSS prevention is a frontend concern)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestCompanyHandler_SetupCompany_VeryLongName(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewCompanyHandler(store, cfg)

	// Create a very long company name
	longName := strings.Repeat("A", 10000)
	company := model.Company{
		Name: longName,
	}

	body, _ := json.Marshal(company)
	req := httptest.NewRequest(http.MethodPost, "/setup/company", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.SetupCompany(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Should handle gracefully (accept or reject, but not crash)
	if resp.StatusCode >= 500 {
		t.Errorf("Server error for long name: %d", resp.StatusCode)
	}
}

func TestCompanyHandler_SetupCompany_MissingName(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewCompanyHandler(store, cfg)

	company := model.Company{
		Industry: "Technology",
	}

	body, _ := json.Marshal(company)
	req := httptest.NewRequest(http.MethodPost, "/setup/company", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.SetupCompany(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestCompanyHandler_SetupCompany_InvalidJSON(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewCompanyHandler(store, cfg)

	req := httptest.NewRequest(http.MethodPost, "/setup/company", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.SetupCompany(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", resp.StatusCode)
	}
}

func TestCompanyHandler_GetStatus_NotSetup(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewCompanyHandler(store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/company/status", nil)
	w := httptest.NewRecorder()

	handler.GetStatus(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result model.CompanyStatus
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.SetupCompleted {
		t.Error("SetupCompleted should be false before setup")
	}
}

func TestCompanyHandler_GetStatus_AfterSetup(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Setup company first
	company := &model.Company{
		Name:           "Test Corp",
		SetupCompleted: true,
	}
	store.SaveCompany(company)

	handler := NewCompanyHandler(store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/company/status", nil)
	w := httptest.NewRecorder()

	handler.GetStatus(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var result model.CompanyStatus
	json.NewDecoder(resp.Body).Decode(&result)

	if !result.SetupCompleted {
		t.Error("SetupCompleted should be true after setup")
	}

	if result.Company == nil || result.Company.Name != "Test Corp" {
		t.Error("Company data should be included")
	}
}

// ================== DOCUMENTS HANDLER TESTS ==================

func TestDocumentsHandler_Upload_CSV(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewDocumentsHandler(store, cfg)

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file
	csvContent := "Metric,Value\nRevenue,1000000\nExpenses,800000"
	part, err := writer.CreateFormFile("file", "test.csv")
	if err != nil {
		t.Fatalf("Failed to create form file: %v", err)
	}
	part.Write([]byte(csvContent))

	// Add metadata
	writer.WriteField("doc_type", "P&L")
	writer.WriteField("period_start", "2024-01-01")
	writer.WriteField("period_end", "2024-12-31")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/documents/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.Upload(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status 200, got %d: %s", resp.StatusCode, string(respBody))
	}

	var result model.Document
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if result.Filename != "test.csv" {
		t.Errorf("Expected filename 'test.csv', got %s", result.Filename)
	}

	if result.DocType != model.DocTypePnL {
		t.Errorf("Expected doc type 'P&L', got %s", result.DocType)
	}
}

func TestDocumentsHandler_Upload_InvalidFileType(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewDocumentsHandler(store, cfg)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.txt")
	part.Write([]byte("text content"))
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/documents/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.Upload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid file type, got %d", resp.StatusCode)
	}
}

func TestDocumentsHandler_Upload_NoFile(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewDocumentsHandler(store, cfg)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	writer.WriteField("doc_type", "P&L")
	writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/documents/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w := httptest.NewRecorder()

	handler.Upload(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing file, got %d", resp.StatusCode)
	}
}

func TestDocumentsHandler_List_Empty(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewDocumentsHandler(store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/documents", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result model.DocumentList
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Documents) != 0 {
		t.Errorf("Expected 0 documents, got %d", len(result.Documents))
	}
}

func TestDocumentsHandler_List_WithDocuments(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Add some documents
	docList := &model.DocumentList{
		Documents: []model.Document{
			{ID: "doc_1", Filename: "test1.csv", DocType: model.DocTypePnL},
			{ID: "doc_2", Filename: "test2.csv", DocType: model.DocTypeBalanceSheet},
		},
	}
	store.SaveDocumentList(docList)

	handler := NewDocumentsHandler(store, cfg)

	req := httptest.NewRequest(http.MethodGet, "/documents", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var result model.DocumentList
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Documents) != 2 {
		t.Errorf("Expected 2 documents, got %d", len(result.Documents))
	}
}

// ================== METRICS HANDLER TESTS ==================

func TestMetricsHandler_GetCurrent_NoData(t *testing.T) {
	_, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewMetricsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/metrics/current", nil)
	w := httptest.NewRecorder()

	handler.GetCurrent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result model.FinancialMetrics
	json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Errors) == 0 {
		t.Error("Should have error about no documents")
	}
}

func TestMetricsHandler_GetCurrent_WithData(t *testing.T) {
	_, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Add parsed document
	parsed := &model.ParsedDocument{
		DocumentID: "doc_1",
		DocType:    model.DocTypePnL,
		Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
		Data: map[string]float64{
			"cash":     500000,
			"revenue":  1000000,
			"expenses": 800000,
		},
		ParsedAt: time.Now(),
	}
	store.SaveParsedDocument(parsed)

	handler := NewMetricsHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/metrics/current", nil)
	w := httptest.NewRecorder()

	handler.GetCurrent(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	var result model.FinancialMetrics
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Cash == nil || *result.Cash != 500000 {
		t.Errorf("Expected cash 500000, got %v", result.Cash)
	}

	if result.Revenue == nil || *result.Revenue != 1000000 {
		t.Errorf("Expected revenue 1000000, got %v", result.Revenue)
	}
}

// ================== CORS MIDDLEWARE TESTS ==================

func TestCORSMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := corsMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()

	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS header missing")
	}

	if resp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Error("CORS methods header missing")
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called for OPTIONS")
	})

	wrapped := corsMiddleware(handler)

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	w := httptest.NewRecorder()

	wrapped.ServeHTTP(w, req)

	resp := w.Result()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for OPTIONS, got %d", resp.StatusCode)
	}
}

// ================== SETUP ROUTES TESTS ==================

func TestSetupRoutes(t *testing.T) {
	cfg, _, cleanup := setupTestEnv(t)
	defer cleanup()

	mux := SetupRoutes(cfg)

	if mux == nil {
		t.Fatal("SetupRoutes returned nil")
	}

	// Test that routes are registered
	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
		{http.MethodGet, "/company/status"},
		{http.MethodGet, "/documents"},
		{http.MethodGet, "/metrics/current"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		if w.Code == http.StatusNotFound {
			t.Errorf("Route %s %s not found", ep.method, ep.path)
		}
	}
}
