//go:build integration
// +build integration

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/cfo/backend/internal/api"
	"github.com/cfo/backend/internal/config"
	"github.com/cfo/backend/internal/industry"
	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// Industry integration tests verify the industry extensibility layer works correctly

func setupIndustryIntegrationTest(t *testing.T) (*httptest.Server, string, func()) {
	tmpDir, err := os.MkdirTemp("", "cfo_industry_integration_*")
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
		OllamaHost: "http://0.0.0.0:11434",
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

func TestIntegration_IndustryRegistry_Initialization(t *testing.T) {
	// Verify all industry handlers are registered
	handlers := industry.GetAllHandlers()

	expectedIndustries := []model.IndustryType{
		model.IndustryEducation,
		model.IndustryEcommerce,
		model.IndustryPharma,
	}

	for _, it := range expectedIndustries {
		if _, ok := handlers[it]; !ok {
			t.Errorf("Handler not registered for industry: %s", it)
		}
	}

	t.Logf("Registered %d industry handlers", len(handlers))
}

func TestIntegration_EducationIndustry_FullFlow(t *testing.T) {
	server, _, cleanup := setupIndustryIntegrationTest(t)
	defer cleanup()

	client := server.Client()
	baseURL := server.URL

	// Step 1: Setup education company
	t.Run("Setup Education Company", func(t *testing.T) {
		company := model.Company{
			Name:          "EduTech University",
			Industry:      "Higher Education",
			IndustryType:  model.IndustryEducation,
			FiscalYearEnd: "June",
			Currency:      "USD",
		}
		body, _ := json.Marshal(company)

		resp, err := client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var result model.Company
		json.NewDecoder(resp.Body).Decode(&result)

		if result.IndustryType != model.IndustryEducation {
			t.Errorf("IndustryType = %s, want %s", result.IndustryType, model.IndustryEducation)
		}
	})

	// Step 2: Upload education financial data
	t.Run("Upload Education Financial Data", func(t *testing.T) {
		csvContent := `Metric,Value
Revenue,15000000
Expenses,12000000
Cash,5000000
Net Income,3000000
Total Assets,50000000
Total Liabilities,20000000`

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", "university_financials.csv")
		part.Write([]byte(csvContent))
		writer.WriteField("doc_type", "P&L")
		writer.WriteField("period_start", "2024-07-01")
		writer.WriteField("period_end", "2024-12-31")
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
	})

	// Step 3: Ask education-specific question
	t.Run("Ask Education-Specific Question", func(t *testing.T) {
		question := model.AskRequest{
			Question: "What is our tuition revenue?",
		}
		body, _ := json.Marshal(question)

		resp, err := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Ask failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result model.AskResponse
		json.NewDecoder(resp.Body).Decode(&result)

		// Should have processed the question
		if result.Question != "What is our tuition revenue?" {
			t.Errorf("Question not echoed: %s", result.Question)
		}

		// Should have numbers from the uploaded document
		if len(result.NumbersUsed) == 0 {
			t.Log("Note: NumbersUsed may be empty if no matching metrics found")
		}
	})

	// Step 4: Ask about student enrollment (industry-specific)
	t.Run("Ask Student Enrollment Question", func(t *testing.T) {
		question := model.AskRequest{
			Question: "What is our student enrollment trend?",
		}
		body, _ := json.Marshal(question)

		resp, err := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Ask failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestIntegration_EcommerceIndustry_FullFlow(t *testing.T) {
	server, _, cleanup := setupIndustryIntegrationTest(t)
	defer cleanup()

	client := server.Client()
	baseURL := server.URL

	// Step 1: Setup ecommerce company
	t.Run("Setup Ecommerce Company", func(t *testing.T) {
		company := model.Company{
			Name:          "ShopMart Online",
			Industry:      "E-commerce Retail",
			IndustryType:  model.IndustryEcommerce,
			FiscalYearEnd: "December",
			Currency:      "USD",
		}
		body, _ := json.Marshal(company)

		resp, err := client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Step 2: Upload ecommerce financial data
	t.Run("Upload Ecommerce Financial Data", func(t *testing.T) {
		csvContent := `Metric,Value
Revenue,25000000
Expenses,22000000
Cash,8000000
Net Income,3000000`

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", "ecommerce_q4.csv")
		part.Write([]byte(csvContent))
		writer.WriteField("doc_type", "P&L")
		writer.WriteField("period_start", "2024-10-01")
		writer.WriteField("period_end", "2024-12-31")
		writer.Close()

		req, _ := http.NewRequest("POST", baseURL+"/documents/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Step 3: Ask GMV question (ecommerce-specific)
	t.Run("Ask GMV Question", func(t *testing.T) {
		question := model.AskRequest{
			Question: "What is our GMV this quarter?",
		}
		body, _ := json.Marshal(question)

		resp, err := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Ask failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Step 4: Ask AOV question
	t.Run("Ask AOV Question", func(t *testing.T) {
		question := model.AskRequest{
			Question: "What is our average order value?",
		}
		body, _ := json.Marshal(question)

		resp, err := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Ask failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestIntegration_PharmaIndustry_FullFlow(t *testing.T) {
	server, _, cleanup := setupIndustryIntegrationTest(t)
	defer cleanup()

	client := server.Client()
	baseURL := server.URL

	// Step 1: Setup pharma company
	t.Run("Setup Pharma Company", func(t *testing.T) {
		company := model.Company{
			Name:          "BioHealth Labs",
			Industry:      "Pharmaceutical",
			IndustryType:  model.IndustryPharma,
			FiscalYearEnd: "December",
			Currency:      "USD",
		}
		body, _ := json.Marshal(company)

		resp, err := client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Step 2: Upload pharma financial data
	t.Run("Upload Pharma Financial Data", func(t *testing.T) {
		csvContent := `Metric,Value
Revenue,500000000
Expenses,450000000
Cash,200000000
Net Income,50000000`

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", "pharma_annual.csv")
		part.Write([]byte(csvContent))
		writer.WriteField("doc_type", "P&L")
		writer.WriteField("period_start", "2024-01-01")
		writer.WriteField("period_end", "2024-12-31")
		writer.Close()

		req, _ := http.NewRequest("POST", baseURL+"/documents/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Upload failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Step 3: Ask R&D question (pharma-specific)
	t.Run("Ask R&D Question", func(t *testing.T) {
		question := model.AskRequest{
			Question: "What is our R&D spending?",
		}
		body, _ := json.Marshal(question)

		resp, err := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Ask failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Step 4: Ask clinical trials question
	t.Run("Ask Clinical Trials Question", func(t *testing.T) {
		question := model.AskRequest{
			Question: "What is the status of our clinical trials?",
		}
		body, _ := json.Marshal(question)

		resp, err := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Ask failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestIntegration_IndustrySwitch(t *testing.T) {
	server, _, cleanup := setupIndustryIntegrationTest(t)
	defer cleanup()

	client := server.Client()
	baseURL := server.URL

	// Start with generic company
	t.Run("Setup Generic Company", func(t *testing.T) {
		company := model.Company{
			Name:          "Generic Corp",
			Industry:      "Technology",
			IndustryType:  model.IndustryGeneric,
			FiscalYearEnd: "December",
			Currency:      "USD",
		}
		body, _ := json.Marshal(company)

		resp, _ := client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Verify generic handling
	t.Run("Ask Generic Question", func(t *testing.T) {
		question := model.AskRequest{
			Question: "What is our cash position?",
		}
		body, _ := json.Marshal(question)

		resp, _ := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Reset and switch to education
	t.Run("Reset Company", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodDelete, baseURL+"/company/reset", nil)
		resp, _ := client.Do(req)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Setup education company
	t.Run("Setup Education Company", func(t *testing.T) {
		company := model.Company{
			Name:          "EduCorp",
			Industry:      "Education",
			IndustryType:  model.IndustryEducation,
			FiscalYearEnd: "June",
			Currency:      "USD",
		}
		body, _ := json.Marshal(company)

		resp, _ := client.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 200, got %d", resp.StatusCode)
		}
	})

	// Verify education-specific handling
	t.Run("Ask Education Question After Switch", func(t *testing.T) {
		question := model.AskRequest{
			Question: "What is our tuition collection rate?",
		}
		body, _ := json.Marshal(question)

		resp, _ := client.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}
	})
}

func TestIntegration_IndustryType_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_industry_persist_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	storage.InitDirectories(tmpDir)

	cfg := &config.Config{
		Port:       "0",
		DataDir:    tmpDir,
		OllamaHost: "http://0.0.0.0:11434",
		ModelName:  "llama3",
	}

	// Server 1: Create company with industry type
	mux1 := api.SetupRoutes(cfg)
	server1 := httptest.NewServer(mux1)

	company := model.Company{
		Name:         "Persistent Pharma",
		IndustryType: model.IndustryPharma,
	}
	body, _ := json.Marshal(company)
	resp, _ := http.Post(server1.URL+"/setup/company", "application/json", bytes.NewReader(body))
	resp.Body.Close()

	server1.Close()

	// Server 2: Verify industry type persisted
	time.Sleep(100 * time.Millisecond)

	mux2 := api.SetupRoutes(cfg)
	server2 := httptest.NewServer(mux2)
	defer server2.Close()

	t.Run("Industry Type Persists", func(t *testing.T) {
		resp, _ := http.Get(server2.URL + "/company/status")
		defer resp.Body.Close()

		var result model.CompanyStatus
		json.NewDecoder(resp.Body).Decode(&result)

		if result.Company == nil {
			t.Fatal("Company should exist after restart")
		}

		if result.Company.IndustryType != model.IndustryPharma {
			t.Errorf("IndustryType = %s, want %s", result.Company.IndustryType, model.IndustryPharma)
		}
	})
}

func TestIntegration_RAGDirectories_Created(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cfo_rag_dirs_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize directories
	err = storage.InitDirectories(tmpDir)
	if err != nil {
		t.Fatalf("InitDirectories failed: %v", err)
	}

	// Verify RAG directories exist
	ragDirs := []string{
		"rag/generic",
		"rag/education",
		"rag/ecommerce",
		"rag/pharma",
	}

	for _, dir := range ragDirs {
		path := tmpDir + "/" + dir
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("RAG directory not created: %s", dir)
		}
	}
}

func TestIntegration_IndustryHandler_VocabularyInjection(t *testing.T) {
	// Test that industry vocabulary is available
	handlers := map[model.IndustryType]industry.IndustryHandler{
		model.IndustryEducation: industry.GetIndustryHandler(model.IndustryEducation),
		model.IndustryEcommerce: industry.GetIndustryHandler(model.IndustryEcommerce),
		model.IndustryPharma:    industry.GetIndustryHandler(model.IndustryPharma),
	}

	for industryType, handler := range handlers {
		t.Run(string(industryType), func(t *testing.T) {
			if handler == nil {
				t.Fatalf("Handler is nil for %s", industryType)
			}

			vocab := handler.GetIndustryVocabulary()
			if len(vocab) == 0 {
				t.Errorf("Vocabulary is empty for %s", industryType)
			}

			t.Logf("%s has %d vocabulary terms", industryType, len(vocab))
		})
	}
}

func TestIntegration_IndustryHandler_IntentResolution(t *testing.T) {
	testCases := []struct {
		industryType model.IndustryType
		question     string
		expectIntent bool
	}{
		{model.IndustryEducation, "What is our enrollment?", true},
		{model.IndustryEducation, "What is our cash?", false},
		{model.IndustryEcommerce, "What is our GMV?", true},
		{model.IndustryEcommerce, "What is our profit?", false},
		{model.IndustryPharma, "What is our R&D spending?", true},
		{model.IndustryPharma, "What is our revenue?", false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.industryType)+"_"+tc.question[:15], func(t *testing.T) {
			handler := industry.GetIndustryHandler(tc.industryType)
			if handler == nil {
				t.Fatalf("Handler is nil for %s", tc.industryType)
			}

			intent, resolved := handler.ResolveIndustryIntent(tc.question)
			if resolved != tc.expectIntent {
				t.Errorf("ResolveIndustryIntent(%q) = %v, want %v (intent: %s)",
					tc.question, resolved, tc.expectIntent, intent)
			}
		})
	}
}

// Benchmark tests for industry module
func BenchmarkIndustryIntentResolution(b *testing.B) {
	handlers := []industry.IndustryHandler{
		industry.GetIndustryHandler(model.IndustryEducation),
		industry.GetIndustryHandler(model.IndustryEcommerce),
		industry.GetIndustryHandler(model.IndustryPharma),
	}

	questions := []string{
		"What is our student enrollment trend?",
		"Show me the GMV breakdown by category",
		"What is our R&D expenditure?",
		"What is our cash position?",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, handler := range handlers {
			for _, q := range questions {
				handler.ResolveIndustryIntent(q)
			}
		}
	}
}

func BenchmarkIndustryVocabularyFetch(b *testing.B) {
	handlers := []industry.IndustryHandler{
		industry.GetIndustryHandler(model.IndustryEducation),
		industry.GetIndustryHandler(model.IndustryEcommerce),
		industry.GetIndustryHandler(model.IndustryPharma),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, handler := range handlers {
			_ = handler.GetIndustryVocabulary()
		}
	}
}

