//go:build integration
// +build integration

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/cfo/backend/internal/api"
	"github.com/cfo/backend/internal/config"
	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// Integration test that tests the full flow:
// 1. Setup company
// 2. Upload documents
// 3. Get metrics
// 4. Ask questions

func setupIntegrationTest(t *testing.T) (*httptest.Server, string, func()) {
	tmpDir, err := os.MkdirTemp("", "cfo_integration_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	err = storage.InitDirectories(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init directories: %v", err)
	}

	cfg := &config.Config{
		Port:       "0",
		DataDir:    tmpDir,
		OllamaHost: "http://0.0.0.0:11434", // May not be running in test
		ModelName:  "llama3",
	}

	mux := api.SetupRoutes(cfg)
	server := httptest.NewServer(mux)

	cleanup := func() {
		server.Close()
		os.RemoveAll(tmpDir)
	}

	return server, tmpDir, cleanup
}

func TestIntegration_FullWorkflow(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	client := server.Client()
	baseURL := server.URL

	// Step 1: Check health
	t.Run("Health Check", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("Health check failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if result["status"] != "ok" {
			t.Error("Health status should be 'ok'")
		}
	})

	// Step 2: Check company status (should not be setup)
	t.Run("Company Not Setup Initially", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/company/status")
		if err != nil {
			t.Fatalf("Company status check failed: %v", err)
		}
		defer resp.Body.Close()

		var result model.CompanyStatus
		json.NewDecoder(resp.Body).Decode(&result)
		if result.SetupCompleted {
			t.Error("Company should not be setup initially")
		}
	})

	// Step 3: Setup company
	t.Run("Setup Company", func(t *testing.T) {
		company := model.Company{
			Name:          "Test Integration Corp",
			Industry:      "Technology",
			FiscalYearEnd: "December",
			Currency:      "USD",
		}
		body, _ := json.Marshal(company)

		resp, err := client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Company setup failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var result model.Company
		json.NewDecoder(resp.Body).Decode(&result)
		if !result.SetupCompleted {
			t.Error("Setup should be completed")
		}
	})

	// Step 4: Verify company is now setup
	t.Run("Company Setup Verified", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/company/status")
		if err != nil {
			t.Fatalf("Company status check failed: %v", err)
		}
		defer resp.Body.Close()

		var result model.CompanyStatus
		json.NewDecoder(resp.Body).Decode(&result)
		if !result.SetupCompleted {
			t.Error("Company should be setup now")
		}
		if result.Company.Name != "Test Integration Corp" {
			t.Errorf("Company name mismatch: %s", result.Company.Name)
		}
	})

	// Step 5: Check documents (should be empty)
	t.Run("Documents Empty Initially", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/documents")
		if err != nil {
			t.Fatalf("Documents list failed: %v", err)
		}
		defer resp.Body.Close()

		var result model.DocumentList
		json.NewDecoder(resp.Body).Decode(&result)
		if len(result.Documents) != 0 {
			t.Errorf("Expected 0 documents, got %d", len(result.Documents))
		}
	})

	// Step 6: Upload a document
	t.Run("Upload Document", func(t *testing.T) {
		csvContent := `Metric,Value
Revenue,1000000
Expenses,800000
Cash,500000
Net Income,200000
Total Assets,2000000
Total Liabilities,800000
Equity,1200000`

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		part, _ := writer.CreateFormFile("file", "q1_financials.csv")
		part.Write([]byte(csvContent))
		writer.WriteField("doc_type", "P&L")
		writer.WriteField("period_start", "2024-01-01")
		writer.WriteField("period_end", "2024-03-31")
		writer.Close()

		req, _ := http.NewRequest("POST", baseURL+"/documents/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var result model.Document
		json.NewDecoder(resp.Body).Decode(&result)
		if result.ID == "" {
			t.Error("Document should have an ID")
		}
		if result.DocType != model.DocTypePnL {
			t.Errorf("DocType mismatch: %s", result.DocType)
		}
	})

	// Step 7: Verify document appears in list
	t.Run("Documents List Updated", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/documents")
		if err != nil {
			t.Fatalf("Documents list failed: %v", err)
		}
		defer resp.Body.Close()

		var result model.DocumentList
		json.NewDecoder(resp.Body).Decode(&result)
		if len(result.Documents) != 1 {
			t.Errorf("Expected 1 document, got %d", len(result.Documents))
		}
	})

	// Step 8: Get metrics
	t.Run("Get Metrics", func(t *testing.T) {
		resp, err := client.Get(baseURL + "/metrics/current")
		if err != nil {
			t.Fatalf("Metrics fetch failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}

		var result model.FinancialMetrics
		json.NewDecoder(resp.Body).Decode(&result)

		// Verify metrics are populated
		if result.Cash == nil || *result.Cash != 500000 {
			t.Errorf("Cash: expected 500000, got %v", result.Cash)
		}
		if result.Revenue == nil || *result.Revenue != 1000000 {
			t.Errorf("Revenue: expected 1000000, got %v", result.Revenue)
		}
		if result.Expenses == nil || *result.Expenses != 800000 {
			t.Errorf("Expenses: expected 800000, got %v", result.Expenses)
		}
		if result.MonthlyBurn == nil {
			t.Error("MonthlyBurn should be calculated")
		}
		if len(result.DataSources) != 1 {
			t.Errorf("Expected 1 data source, got %d", len(result.DataSources))
		}
	})

	// Step 9: Ask a question (may fail if Ollama not running, which is OK)
	t.Run("Ask CFO", func(t *testing.T) {
		question := model.AskRequest{
			Question: "What is our cash position?",
		}
		body, _ := json.Marshal(question)

		resp, err := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Ask failed: %v", err)
		}
		defer resp.Body.Close()

		// We accept both success (if Ollama running) and error (if not)
		var result model.AskResponse
		json.NewDecoder(resp.Body).Decode(&result)

		if result.Question != "What is our cash position?" {
			t.Errorf("Question not echoed: %s", result.Question)
		}

		// If LLM is not running, we should still have numbers
		if len(result.NumbersUsed) == 0 {
			t.Error("NumbersUsed should be populated even if LLM fails")
		}
	})
}

func TestIntegration_MultipleDocuments(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	client := server.Client()
	baseURL := server.URL

	// Setup company first
	company := model.Company{Name: "Multi-Doc Corp"}
	body, _ := json.Marshal(company)
	client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))

	// Upload multiple documents
	documents := []struct {
		filename    string
		content     string
		periodStart string
		periodEnd   string
	}{
		{
			filename:    "q1.csv",
			content:     "Metric,Value\nRevenue,100000\nExpenses,80000\nCash,200000",
			periodStart: "2024-01-01",
			periodEnd:   "2024-03-31",
		},
		{
			filename:    "q2.csv",
			content:     "Metric,Value\nRevenue,120000\nExpenses,85000\nCash,235000",
			periodStart: "2024-04-01",
			periodEnd:   "2024-06-30",
		},
		{
			filename:    "q3.csv",
			content:     "Metric,Value\nRevenue,150000\nExpenses,90000\nCash,295000",
			periodStart: "2024-07-01",
			periodEnd:   "2024-09-30",
		},
	}

	for _, doc := range documents {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", doc.filename)
		part.Write([]byte(doc.content))
		writer.WriteField("doc_type", "P&L")
		writer.WriteField("period_start", doc.periodStart)
		writer.WriteField("period_end", doc.periodEnd)
		writer.Close()

		req, _ := http.NewRequest("POST", baseURL+"/documents/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, _ := client.Do(req)
		resp.Body.Close()
	}

	// Verify all documents uploaded
	t.Run("All Documents Uploaded", func(t *testing.T) {
		resp, _ := client.Get(baseURL + "/documents")
		defer resp.Body.Close()

		var result model.DocumentList
		json.NewDecoder(resp.Body).Decode(&result)

		if len(result.Documents) != 3 {
			t.Errorf("Expected 3 documents, got %d", len(result.Documents))
		}
	})

	// Verify metrics use most recent data
	t.Run("Metrics Use Latest Data", func(t *testing.T) {
		resp, _ := client.Get(baseURL + "/metrics/current")
		defer resp.Body.Close()

		var result model.FinancialMetrics
		json.NewDecoder(resp.Body).Decode(&result)

		// Should have trend data from multiple periods
		if len(result.DataSources) != 3 {
			t.Errorf("Expected 3 data sources, got %d", len(result.DataSources))
		}
	})
}

func TestIntegration_ErrorCases(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	client := server.Client()
	baseURL := server.URL

	t.Run("Setup Without Name", func(t *testing.T) {
		company := model.Company{Industry: "Technology"}
		body, _ := json.Marshal(company)

		resp, _ := client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("Upload Invalid File Type", func(t *testing.T) {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", "test.txt")
		part.Write([]byte("text content"))
		writer.Close()

		req, _ := http.NewRequest("POST", baseURL+"/documents/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, _ := client.Do(req)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("Ask Without Question", func(t *testing.T) {
		// Setup company first
		company := model.Company{Name: "Error Test Corp"}
		body, _ := json.Marshal(company)
		client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))

		// Ask without question
		question := model.AskRequest{Question: ""}
		body2, _ := json.Marshal(question)

		resp, _ := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body2))
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400, got %d", resp.StatusCode)
		}
	})
}

func TestIntegration_BurnRateCalculation(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	client := server.Client()
	baseURL := server.URL

	// Setup company
	company := model.Company{Name: "Startup Inc"}
	body, _ := json.Marshal(company)
	client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))

	// Upload startup data (burning cash)
	csvContent := `Metric,Value
Cash,300000
Revenue,50000
Expenses,100000
Net Income,-50000`

	uploadBody := &bytes.Buffer{}
	writer := multipart.NewWriter(uploadBody)
	part, _ := writer.CreateFormFile("file", "startup.csv")
	part.Write([]byte(csvContent))
	writer.WriteField("doc_type", "P&L")
	writer.WriteField("period_start", "2024-01-01")
	writer.WriteField("period_end", "2024-01-31")
	writer.Close()

	req, _ := http.NewRequest("POST", baseURL+"/documents/upload", uploadBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, _ := client.Do(req)
	resp.Body.Close()

	// Check burn rate and runway
	t.Run("Burn Rate Calculation", func(t *testing.T) {
		resp, _ := client.Get(baseURL + "/metrics/current")
		defer resp.Body.Close()

		var result model.FinancialMetrics
		json.NewDecoder(resp.Body).Decode(&result)

		// Burn = Expenses - Revenue = 100000 - 50000 = 50000
		if result.MonthlyBurn == nil {
			t.Fatal("MonthlyBurn should be calculated")
		}
		if *result.MonthlyBurn != 50000 {
			t.Errorf("Expected burn 50000, got %f", *result.MonthlyBurn)
		}

		// Runway = Cash / Burn = 300000 / 50000 = 6 months
		if result.RunwayMonths == nil {
			t.Fatal("RunwayMonths should be calculated")
		}
		if *result.RunwayMonths != 6 {
			t.Errorf("Expected runway 6 months, got %f", *result.RunwayMonths)
		}
	})
}

func TestIntegration_ProfitableCompany(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	client := server.Client()
	baseURL := server.URL

	// Setup company
	company := model.Company{Name: "Profitable Corp"}
	body, _ := json.Marshal(company)
	client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))

	// Upload profitable company data
	csvContent := `Metric,Value
Cash,2000000
Revenue,500000
Expenses,350000
Net Income,150000`

	uploadBody := &bytes.Buffer{}
	writer := multipart.NewWriter(uploadBody)
	part, _ := writer.CreateFormFile("file", "profitable.csv")
	part.Write([]byte(csvContent))
	writer.WriteField("doc_type", "P&L")
	writer.WriteField("period_start", "2024-01-01")
	writer.WriteField("period_end", "2024-01-31")
	writer.Close()

	req, _ := http.NewRequest("POST", baseURL+"/documents/upload", uploadBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, _ := client.Do(req)
	resp.Body.Close()

	t.Run("Infinite Runway for Profitable Company", func(t *testing.T) {
		resp, _ := client.Get(baseURL + "/metrics/current")
		defer resp.Body.Close()

		var result model.FinancialMetrics
		json.NewDecoder(resp.Body).Decode(&result)

		// Burn = 350000 - 500000 = -150000 (negative = profitable)
		if result.MonthlyBurn == nil {
			t.Fatal("MonthlyBurn should be calculated")
		}
		if *result.MonthlyBurn >= 0 {
			t.Errorf("Burn should be negative for profitable company: %f", *result.MonthlyBurn)
		}

		// Runway should be "infinite" (999)
		if result.RunwayMonths == nil {
			t.Fatal("RunwayMonths should be calculated")
		}
		if *result.RunwayMonths != 999 {
			t.Errorf("Expected infinite runway (999), got %f", *result.RunwayMonths)
		}
	})
}

func TestIntegration_DataPersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_persist_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)

	// Create first server, setup data
	cfg := &config.Config{
		Port:       "0",
		DataDir:    tmpDir,
		OllamaHost: "http://0.0.0.0:11434",
		ModelName:  "llama3",
	}

	mux1 := api.SetupRoutes(cfg)
	server1 := httptest.NewServer(mux1)

	// Setup company and upload doc
	client := server1.Client()
	company := model.Company{Name: "Persistent Corp"}
	body, _ := json.Marshal(company)
	client.Post(server1.URL+"/setup/company", "application/json", bytes.NewReader(body))

	csvContent := "Metric,Value\nCash,1000000\nRevenue,500000"
	uploadBody := &bytes.Buffer{}
	writer := multipart.NewWriter(uploadBody)
	part, _ := writer.CreateFormFile("file", "data.csv")
	part.Write([]byte(csvContent))
	writer.Close()
	req, _ := http.NewRequest("POST", server1.URL+"/documents/upload", uploadBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client.Do(req)

	server1.Close()

	// Create second server with same data dir
	time.Sleep(100 * time.Millisecond) // Small delay

	mux2 := api.SetupRoutes(cfg)
	server2 := httptest.NewServer(mux2)
	defer server2.Close()

	t.Run("Data Persists Across Restarts", func(t *testing.T) {
		resp, _ := http.Get(server2.URL + "/company/status")
		defer resp.Body.Close()

		var result model.CompanyStatus
		json.NewDecoder(resp.Body).Decode(&result)

		if !result.SetupCompleted {
			t.Error("Company setup should persist")
		}
		if result.Company == nil || result.Company.Name != "Persistent Corp" {
			t.Error("Company name should persist")
		}
	})

	t.Run("Documents Persist", func(t *testing.T) {
		resp, _ := http.Get(server2.URL + "/documents")
		defer resp.Body.Close()

		var result model.DocumentList
		json.NewDecoder(resp.Body).Decode(&result)

		if len(result.Documents) != 1 {
			t.Errorf("Expected 1 document to persist, got %d", len(result.Documents))
		}
	})
}

// Benchmark tests
func BenchmarkDocumentUpload(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "cfo_bench_*")
	defer os.RemoveAll(tmpDir)
	storage.InitDirectories(tmpDir)

	cfg := &config.Config{
		Port:       "0",
		DataDir:    tmpDir,
		OllamaHost: "http://0.0.0.0:11434",
		ModelName:  "llama3",
	}

	mux := api.SetupRoutes(cfg)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Setup company
	company := model.Company{Name: "Bench Corp"}
	body, _ := json.Marshal(company)
	http.Post(server.URL+"/setup/company", "application/json", bytes.NewReader(body))

	csvContent := "Metric,Value\nRevenue,1000000\nExpenses,800000\nCash,500000"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		uploadBody := &bytes.Buffer{}
		writer := multipart.NewWriter(uploadBody)
		part, _ := writer.CreateFormFile("file", fmt.Sprintf("bench_%d.csv", i))
		part.Write([]byte(csvContent))
		writer.Close()

		req, _ := http.NewRequest("POST", server.URL+"/documents/upload", uploadBody)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		resp, _ := http.DefaultClient.Do(req)
		resp.Body.Close()
	}
}

func BenchmarkMetricsCalculation(b *testing.B) {
	tmpDir, _ := os.MkdirTemp("", "cfo_bench_*")
	defer os.RemoveAll(tmpDir)
	storage.InitDirectories(tmpDir)

	cfg := &config.Config{
		Port:       "0",
		DataDir:    tmpDir,
		OllamaHost: "http://0.0.0.0:11434",
		ModelName:  "llama3",
	}

	store := storage.NewFileStore(tmpDir)

	// Create 10 documents
	for i := 0; i < 10; i++ {
		parsed := &model.ParsedDocument{
			DocumentID: fmt.Sprintf("doc_%d", i),
			DocType:    model.DocTypePnL,
			Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
			Data: map[string]float64{
				"revenue":  float64(1000000 + i*100000),
				"expenses": float64(800000 + i*50000),
				"cash":     float64(500000 + i*25000),
			},
			ParsedAt: time.Now(),
		}
		store.SaveParsedDocument(parsed)
	}

	mux := api.SetupRoutes(cfg)
	server := httptest.NewServer(mux)
	defer server.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, _ := http.Get(server.URL + "/metrics/current")
		resp.Body.Close()
	}
}
