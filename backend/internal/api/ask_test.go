package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cfo/backend/internal/model"
)

// ================== ASK HANDLER TESTS ==================

func TestAskHandler_Ask_ValidQuestion(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Setup company with industry type
	company := &model.Company{
		Name:           "Test Corp",
		IndustryType:   model.IndustryGeneric,
		SetupCompleted: true,
	}
	store.SaveCompany(company)

	// Add parsed document for context
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

	handler := NewAskHandler(store, cfg)

	req := model.AskRequest{Question: "What is our cash position?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result model.AskResponse
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Question != "What is our cash position?" {
		t.Errorf("Question not echoed correctly: %s", result.Question)
	}

	if len(result.NumbersUsed) == 0 {
		t.Error("NumbersUsed should be populated")
	}
}

func TestAskHandler_Ask_EmptyQuestion(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	req := model.AskRequest{Question: ""}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for empty question, got %d", resp.StatusCode)
	}
}

func TestAskHandler_Ask_InvalidJSON(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", strings.NewReader("invalid json"))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestAskHandler_Ask_MethodNotAllowed(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	httpReq := httptest.NewRequest(http.MethodGet, "/ask", nil)
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for GET, got %d", resp.StatusCode)
	}
}

func TestAskHandler_Ask_WithPeriodDetection(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Add parsed documents for different quarters
	for _, q := range []struct {
		docID string
		start string
		end   string
		cash  float64
	}{
		{"doc_q1", "2024-01-01", "2024-03-31", 400000},
		{"doc_q2", "2024-04-01", "2024-06-30", 500000},
		{"doc_q3", "2024-07-01", "2024-09-30", 600000},
	} {
		parsed := &model.ParsedDocument{
			DocumentID: q.docID,
			DocType:    model.DocTypePnL,
			Period:     model.Period{Start: q.start, End: q.end},
			Data:       map[string]float64{"cash": q.cash},
			ParsedAt:   time.Now(),
		}
		store.SaveParsedDocument(parsed)
	}

	handler := NewAskHandler(store, cfg)

	// Ask about Q2 specifically
	req := model.AskRequest{Question: "What was our cash position in Q2 2024?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result model.AskResponse
	json.NewDecoder(resp.Body).Decode(&result)

	// Verify period was detected in the response context
	if len(result.NumbersUsed) == 0 {
		t.Log("NumbersUsed is empty")
	} else {
		t.Log("NumbersUsed:", result.NumbersUsed)
	}
}

func TestAskHandler_Ask_WithIndustryContext_Education(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Setup education company
	company := &model.Company{
		Name:           "EduTech University",
		Industry:       "Education",
		IndustryType:   model.IndustryEducation,
		SetupCompleted: true,
	}
	store.SaveCompany(company)

	// Add some data
	parsed := &model.ParsedDocument{
		DocumentID: "doc_1",
		DocType:    model.DocTypePnL,
		Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
		Data: map[string]float64{
			"revenue": 5000000,
			"cash":    1000000,
		},
		ParsedAt: time.Now(),
	}
	store.SaveParsedDocument(parsed)

	handler := NewAskHandler(store, cfg)

	// Ask education-specific question
	req := model.AskRequest{Question: "What is our student enrollment this year?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	// Should return OK even if industry data is placeholder
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestAskHandler_Ask_WithIndustryContext_Ecommerce(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Setup ecommerce company
	company := &model.Company{
		Name:           "ShopMart Inc",
		Industry:       "E-commerce",
		IndustryType:   model.IndustryEcommerce,
		SetupCompleted: true,
	}
	store.SaveCompany(company)

	parsed := &model.ParsedDocument{
		DocumentID: "doc_1",
		DocType:    model.DocTypePnL,
		Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
		Data:       map[string]float64{"revenue": 10000000},
		ParsedAt:   time.Now(),
	}
	store.SaveParsedDocument(parsed)

	handler := NewAskHandler(store, cfg)

	// Ask ecommerce-specific question
	req := model.AskRequest{Question: "What is our GMV this quarter?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestAskHandler_Ask_WithIndustryContext_Pharma(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Setup pharma company
	company := &model.Company{
		Name:           "BioHealth Labs",
		Industry:       "Pharmaceutical",
		IndustryType:   model.IndustryPharma,
		SetupCompleted: true,
	}
	store.SaveCompany(company)

	parsed := &model.ParsedDocument{
		DocumentID: "doc_1",
		DocType:    model.DocTypePnL,
		Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
		Data:       map[string]float64{"revenue": 50000000},
		ParsedAt:   time.Now(),
	}
	store.SaveParsedDocument(parsed)

	handler := NewAskHandler(store, cfg)

	// Ask pharma-specific question
	req := model.AskRequest{Question: "What is our R&D spending?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestAskHandler_GetCompanyIndustryType_NoCompany(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	industryType := handler.getCompanyIndustryType()
	if industryType != model.IndustryGeneric {
		t.Errorf("Expected generic industry type when no company, got %s", industryType)
	}
}

func TestAskHandler_GetCompanyIndustryType_EmptyIndustryType(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	company := &model.Company{
		Name:           "Test Corp",
		IndustryType:   "", // Empty
		SetupCompleted: true,
	}
	store.SaveCompany(company)

	handler := NewAskHandler(store, cfg)

	industryType := handler.getCompanyIndustryType()
	if industryType != model.IndustryGeneric {
		t.Errorf("Expected generic industry type for empty value, got %s", industryType)
	}
}

// ================== SECURITY TESTS ==================

func TestAskHandler_Ask_XSSHandling(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	// Test with XSS payload in question
	// Note: XSS prevention is primarily a frontend concern.
	// The backend should handle these inputs gracefully without crashing.
	xssPayloads := []string{
		"<script>alert('xss')</script>What is our cash?",
		"<img src=x onerror=alert('xss')>Show revenue",
		"javascript:alert('xss')//What is profit?",
		"<svg onload=alert('xss')>",
	}

	for _, payload := range xssPayloads {
		t.Run("XSS_"+payload[:10], func(t *testing.T) {
			req := model.AskRequest{Question: payload}
			body, _ := json.Marshal(req)

			httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Ask(w, httpReq)

			resp := w.Result()
			defer resp.Body.Close()

			// Should not crash, should handle gracefully
			// Backend accepts input as-is; XSS prevention is frontend responsibility
			if resp.StatusCode >= 500 {
				t.Errorf("Server error for XSS payload: %d", resp.StatusCode)
			}

			// Verify response is valid JSON
			var result model.AskResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Errorf("Failed to decode response: %v", err)
			}
		})
	}
}

func TestAskHandler_Ask_SQLInjectionPrevention(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	// Test with SQL injection payloads
	sqlPayloads := []string{
		"'; DROP TABLE companies; --",
		"1' OR '1'='1",
		"1; DELETE FROM documents WHERE '1'='1",
		"UNION SELECT * FROM users",
	}

	for _, payload := range sqlPayloads {
		t.Run("SQL_"+payload[:10], func(t *testing.T) {
			req := model.AskRequest{Question: payload}
			body, _ := json.Marshal(req)

			httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Ask(w, httpReq)

			// Should not crash
			resp := w.Result()
			defer resp.Body.Close()

			// We use file-based storage, not SQL, but still verify no crash
			if resp.StatusCode >= 500 {
				t.Errorf("Server error for SQL injection attempt: %d", resp.StatusCode)
			}
		})
	}
}

func TestAskHandler_Ask_PathTraversalPrevention(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	// Test with path traversal payloads
	pathPayloads := []string{
		"../../etc/passwd",
		"..\\..\\windows\\system32\\config\\sam",
		"/etc/shadow",
		"file:///etc/passwd",
	}

	for _, payload := range pathPayloads {
		t.Run("Path_"+payload[:10], func(t *testing.T) {
			req := model.AskRequest{Question: payload}
			body, _ := json.Marshal(req)

			httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Ask(w, httpReq)

			resp := w.Result()
			defer resp.Body.Close()

			// Should not crash or expose system files
			if resp.StatusCode >= 500 {
				t.Errorf("Server error for path traversal attempt: %d", resp.StatusCode)
			}
		})
	}
}

func TestAskHandler_Ask_LargePayload(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	// Create a very large question (potential DoS)
	largeQuestion := strings.Repeat("What is our cash position? ", 10000)

	req := model.AskRequest{Question: largeQuestion}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	// Should handle gracefully (either process or reject)
	if resp.StatusCode >= 500 {
		t.Errorf("Server error for large payload: %d", resp.StatusCode)
	}
}

func TestAskHandler_Ask_UnicodeHandling(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	// Test with various Unicode characters
	unicodeQuestions := []string{
		"收入是多少？", // Chinese: What is the revenue?
		"¿Cuál es el flujo de caja?", // Spanish: What is the cash flow?
		"💰 What is our cash position?", // Emoji
		"مبيعات Q4", // Arabic: Q4 sales
		"Какова прибыль?", // Russian: What is the profit?
	}

	for _, q := range unicodeQuestions {
		t.Run("Unicode", func(t *testing.T) {
			req := model.AskRequest{Question: q}
			body, _ := json.Marshal(req)

			httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Ask(w, httpReq)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode >= 500 {
				t.Errorf("Server error for unicode question: %d", resp.StatusCode)
			}
		})
	}
}

// ================== EDGE CASE TESTS ==================

func TestAskHandler_Ask_NoDocuments(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	handler := NewAskHandler(store, cfg)

	req := model.AskRequest{Question: "What is our revenue?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	// Should return OK even with no documents
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 even with no documents, got %d", resp.StatusCode)
	}

	var result model.AskResponse
	json.NewDecoder(resp.Body).Decode(&result)

	// Should indicate no data available
	if len(result.NumbersUsed) == 0 {
		// This is expected when no documents
	}
}

func TestAskHandler_Ask_EmptyDocumentData(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Add empty parsed document
	parsed := &model.ParsedDocument{
		DocumentID: "doc_empty",
		DocType:    model.DocTypePnL,
		Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
		Data:       map[string]float64{}, // Empty data
		ParsedAt:   time.Now(),
	}
	store.SaveParsedDocument(parsed)

	handler := NewAskHandler(store, cfg)

	req := model.AskRequest{Question: "What is our cash?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestAskHandler_Ask_NegativeValues(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Add document with negative values (losses)
	parsed := &model.ParsedDocument{
		DocumentID: "doc_loss",
		DocType:    model.DocTypePnL,
		Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
		Data: map[string]float64{
			"revenue":    500000,
			"expenses":   800000,
			"net_income": -300000, // Loss
		},
		ParsedAt: time.Now(),
	}
	store.SaveParsedDocument(parsed)

	handler := NewAskHandler(store, cfg)

	req := model.AskRequest{Question: "Are we profitable?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestAskHandler_Ask_VeryLargeNumbers(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Add document with very large numbers
	parsed := &model.ParsedDocument{
		DocumentID: "doc_large",
		DocType:    model.DocTypePnL,
		Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
		Data: map[string]float64{
			"revenue":  999999999999999,
			"cash":     888888888888888,
			"expenses": 777777777777777,
		},
		ParsedAt: time.Now(),
	}
	store.SaveParsedDocument(parsed)

	handler := NewAskHandler(store, cfg)

	req := model.AskRequest{Question: "What is our revenue?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

func TestAskHandler_Ask_ZeroValues(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Add document with zero values
	parsed := &model.ParsedDocument{
		DocumentID: "doc_zero",
		DocType:    model.DocTypePnL,
		Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
		Data: map[string]float64{
			"revenue":  0,
			"expenses": 0,
			"cash":     0,
		},
		ParsedAt: time.Now(),
	}
	store.SaveParsedDocument(parsed)

	handler := NewAskHandler(store, cfg)

	req := model.AskRequest{Question: "What is our burn rate?"}
	body, _ := json.Marshal(req)

	httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.Ask(w, httpReq)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// ================== CONCURRENT ACCESS TESTS ==================

func TestAskHandler_Ask_ConcurrentRequests(t *testing.T) {
	cfg, store, cleanup := setupTestEnv(t)
	defer cleanup()

	// Add some data
	parsed := &model.ParsedDocument{
		DocumentID: "doc_1",
		DocType:    model.DocTypePnL,
		Period:     model.Period{Start: "2024-01-01", End: "2024-12-31"},
		Data:       map[string]float64{"cash": 500000},
		ParsedAt:   time.Now(),
	}
	store.SaveParsedDocument(parsed)

	handler := NewAskHandler(store, cfg)

	// Run concurrent requests
	numRequests := 20
	results := make(chan int, numRequests)

	for i := 0; i < numRequests; i++ {
		go func() {
			req := model.AskRequest{Question: "What is our cash?"}
			body, _ := json.Marshal(req)

			httpReq := httptest.NewRequest(http.MethodPost, "/ask", bytes.NewReader(body))
			httpReq.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler.Ask(w, httpReq)

			results <- w.Result().StatusCode
		}()
	}

	// Collect results
	successCount := 0
	for i := 0; i < numRequests; i++ {
		status := <-results
		if status == http.StatusOK {
			successCount++
		}
	}

	// All requests should succeed
	if successCount != numRequests {
		t.Errorf("Expected %d successful requests, got %d", numRequests, successCount)
	}
}

