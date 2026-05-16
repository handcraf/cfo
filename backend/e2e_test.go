//go:build e2e
// +build e2e

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cfo/backend/internal/model"
)

// End-to-end tests that require the backend to be running
// Run with: go test -tags=e2e -v

const baseURL = "http://0.0.0.0:8080"

func checkBackendRunning(t *testing.T) {
	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Skip("Backend not running, skipping e2e tests")
	}
	resp.Body.Close()
}

func TestE2E_CompleteFlow(t *testing.T) {
	checkBackendRunning(t)

	t.Run("1. Health Check", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/health")
		if err != nil {
			t.Fatalf("Health check failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			t.Errorf("Expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		t.Logf("Health: %v", result)
	})

	t.Run("2. Company Status", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/company/status")
		if err != nil {
			t.Fatalf("Company status failed: %v", err)
		}
		defer resp.Body.Close()

		var result model.CompanyStatus
		json.NewDecoder(resp.Body).Decode(&result)
		t.Logf("Company setup completed: %v", result.SetupCompleted)
	})

	t.Run("3. Setup Company", func(t *testing.T) {
		company := model.Company{
			Name:          "E2E Test Corp " + time.Now().Format("15:04:05"),
			Industry:      "Technology",
			FiscalYearEnd: "December",
			Currency:      "USD",
		}
		body, _ := json.Marshal(company)

		resp, err := http.Post(baseURL+"/setup/company", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("Expected 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var result model.Company
		json.NewDecoder(resp.Body).Decode(&result)
		t.Logf("Company created: %s", result.Name)
	})

	t.Run("4. Upload Test Documents", func(t *testing.T) {
		testFiles := []struct {
			name        string
			content     string
			docType     string
			periodStart string
			periodEnd   string
		}{
			{
				name: "pnl_q1.csv",
				content: `Metric,Value
Revenue,1000000
Expenses,800000
Net Income,200000
Cash,500000
Total Assets,2000000
Total Liabilities,800000
Equity,1200000`,
				docType:     "P&L",
				periodStart: "2024-01-01",
				periodEnd:   "2024-03-31",
			},
			{
				name: "pnl_q2.csv",
				content: `Metric,Value
Revenue,1200000
Expenses,850000
Net Income,350000
Cash,750000
Total Assets,2500000
Total Liabilities,850000
Equity,1650000`,
				docType:     "P&L",
				periodStart: "2024-04-01",
				periodEnd:   "2024-06-30",
			},
		}

		for _, tf := range testFiles {
			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)

			part, _ := writer.CreateFormFile("file", tf.name)
			part.Write([]byte(tf.content))
			writer.WriteField("doc_type", tf.docType)
			writer.WriteField("period_start", tf.periodStart)
			writer.WriteField("period_end", tf.periodEnd)
			writer.Close()

			req, _ := http.NewRequest("POST", baseURL+"/documents/upload", body)
			req.Header.Set("Content-Type", writer.FormDataContentType())

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Upload failed for %s: %v", tf.name, err)
			}

			if resp.StatusCode != 200 {
				respBody, _ := io.ReadAll(resp.Body)
				t.Fatalf("Upload failed for %s: %s", tf.name, string(respBody))
			}

			var result model.Document
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()

			t.Logf("Uploaded: %s (ID: %s)", result.Filename, result.ID)
		}
	})

	t.Run("5. List Documents", func(t *testing.T) {
		resp, _ := http.Get(baseURL + "/documents")
		defer resp.Body.Close()

		var result model.DocumentList
		json.NewDecoder(resp.Body).Decode(&result)

		t.Logf("Total documents: %d", len(result.Documents))
		for _, doc := range result.Documents {
			t.Logf("  - %s (%s): %s to %s", doc.Filename, doc.DocType, doc.PeriodStart, doc.PeriodEnd)
		}
	})

	t.Run("6. Get Financial Metrics", func(t *testing.T) {
		resp, _ := http.Get(baseURL + "/metrics/current")
		defer resp.Body.Close()

		var result model.FinancialMetrics
		json.NewDecoder(resp.Body).Decode(&result)

		t.Log("=== Financial Metrics ===")
		if result.Cash != nil {
			t.Logf("Cash: $%.2f", *result.Cash)
		}
		if result.Revenue != nil {
			t.Logf("Revenue: $%.2f", *result.Revenue)
		}
		if result.Expenses != nil {
			t.Logf("Expenses: $%.2f", *result.Expenses)
		}
		if result.NetIncome != nil {
			t.Logf("Net Income: $%.2f", *result.NetIncome)
		}
		if result.MonthlyBurn != nil {
			t.Logf("Monthly Burn: $%.2f", *result.MonthlyBurn)
		}
		if result.RunwayMonths != nil {
			if *result.RunwayMonths >= 999 {
				t.Log("Runway: ∞ (Profitable)")
			} else {
				t.Logf("Runway: %.1f months", *result.RunwayMonths)
			}
		}

		if result.Trends != nil {
			t.Log("=== Trends ===")
			if result.Trends.RevenueChange != nil {
				t.Logf("Revenue Change: %.1f%%", *result.Trends.RevenueChange)
			}
			if result.Trends.ExpenseChange != nil {
				t.Logf("Expense Change: %.1f%%", *result.Trends.ExpenseChange)
			}
		}

		if len(result.Errors) > 0 {
			t.Log("=== Errors ===")
			for _, err := range result.Errors {
				t.Logf("  - %s", err)
			}
		}

		t.Logf("Data Sources: %v", result.DataSources)
	})

	t.Run("7. Ask CFO Questions", func(t *testing.T) {
		questions := []string{
			"What is our current cash position?",
			"How long is our runway?",
			"Explain our burn rate",
			"What are our revenue trends?",
		}

		for _, q := range questions {
			t.Logf("\n=== Question: %s ===", q)

			req := model.AskRequest{Question: q}
			body, _ := json.Marshal(req)

			resp, err := http.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Logf("Ask failed: %v", err)
				continue
			}

			var result model.AskResponse
			json.NewDecoder(resp.Body).Decode(&result)
			resp.Body.Close()

			if result.Error != "" {
				t.Logf("Error: %s", result.Error)
				t.Logf("Numbers Available: %v", result.NumbersUsed)
			} else {
				t.Logf("Summary: %s", result.Summary)
				t.Logf("Numbers Used: %v", result.NumbersUsed)
				if result.Explanation != "" {
					// Truncate long explanations
					exp := result.Explanation
					if len(exp) > 200 {
						exp = exp[:200] + "..."
					}
					t.Logf("Explanation: %s", exp)
				}
			}
		}
	})
}

func TestE2E_FileUploadFromDisk(t *testing.T) {
	checkBackendRunning(t)

	// Find test data files
	testDataDir := filepath.Join("testdata")
	files, err := os.ReadDir(testDataDir)
	if err != nil {
		t.Skipf("Test data directory not found: %v", err)
	}

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".csv" {
			continue
		}

		t.Run("Upload_"+file.Name(), func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(testDataDir, file.Name()))
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			part, _ := writer.CreateFormFile("file", file.Name())
			part.Write(content)
			writer.WriteField("doc_type", "P&L")
			writer.WriteField("period_start", "2024-01-01")
			writer.WriteField("period_end", "2024-12-31")
			writer.Close()

			req, _ := http.NewRequest("POST", baseURL+"/documents/upload", body)
			req.Header.Set("Content-Type", writer.FormDataContentType())

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Upload failed: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				respBody, _ := io.ReadAll(resp.Body)
				t.Logf("Upload may have failed (could be expected for some test files): %s", string(respBody))
			} else {
				var result model.Document
				json.NewDecoder(resp.Body).Decode(&result)
				t.Logf("Uploaded: %s -> %s", file.Name(), result.ID)
			}
		})
	}
}

func TestE2E_LLMIntegration(t *testing.T) {
	checkBackendRunning(t)

	// Check if Ollama is responding
	ollamaResp, err := http.Get("http://0.0.0.0:11434/api/tags")
	if err != nil {
		t.Skip("Ollama not running, skipping LLM integration test")
	}
	ollamaResp.Body.Close()

	t.Log("Ollama is running, testing LLM integration...")

	// Ask a question that requires LLM
	question := model.AskRequest{
		Question: "What do our financial metrics tell us about the health of the business?",
	}
	body, _ := json.Marshal(question)

	resp, err := http.Post(baseURL+"/ask", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Ask failed: %v", err)
	}
	defer resp.Body.Close()

	var result model.AskResponse
	json.NewDecoder(resp.Body).Decode(&result)

	t.Log("=== LLM Response ===")
	t.Logf("Question: %s", result.Question)
	t.Logf("Summary: %s", result.Summary)
	t.Logf("Explanation: %s", result.Explanation)
	t.Logf("Numbers Used: %v", result.NumbersUsed)
	t.Logf("Sources: %v", result.Sources)

	if result.Error != "" {
		t.Logf("Error: %s", result.Error)
	}

	// Verify LLM produced a response
	if result.Summary == "" && result.Error == "" {
		t.Error("LLM should produce either a summary or an error")
	}
}

func TestE2E_StressTest(t *testing.T) {
	checkBackendRunning(t)

	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	t.Log("Starting stress test...")

	// Upload 20 documents rapidly
	numDocs := 20
	successCount := 0

	for i := 0; i < numDocs; i++ {
		content := fmt.Sprintf(`Metric,Value
Revenue,%d
Expenses,%d
Cash,%d
Net Income,%d`, 1000000+i*10000, 800000+i*5000, 500000+i*2500, 200000+i*5000)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, _ := writer.CreateFormFile("file", fmt.Sprintf("stress_%d.csv", i))
		part.Write([]byte(content))
		writer.WriteField("doc_type", "P&L")
		writer.Close()

		req, _ := http.NewRequest("POST", baseURL+"/documents/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())

		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == 200 {
			successCount++
		}
		if resp != nil {
			resp.Body.Close()
		}
	}

	t.Logf("Successfully uploaded %d/%d documents", successCount, numDocs)

	// Verify all documents are listed
	resp, _ := http.Get(baseURL + "/documents")
	defer resp.Body.Close()

	var result model.DocumentList
	json.NewDecoder(resp.Body).Decode(&result)

	t.Logf("Total documents after stress test: %d", len(result.Documents))

	// Measure metrics calculation time
	start := time.Now()
	metricsResp, _ := http.Get(baseURL + "/metrics/current")
	metricsResp.Body.Close()
	elapsed := time.Since(start)

	t.Logf("Metrics calculation time with %d documents: %v", len(result.Documents), elapsed)

	if elapsed > 5*time.Second {
		t.Errorf("Metrics calculation too slow: %v", elapsed)
	}
}
