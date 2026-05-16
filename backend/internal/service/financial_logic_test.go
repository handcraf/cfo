package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// Helper to create a temporary test directory
func setupTestStorage(t *testing.T) (*storage.FileStore, string, func()) {
	tmpDir, err := os.MkdirTemp("", "cfo_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	err = storage.InitDirectories(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to init directories: %v", err)
	}

	store := storage.NewFileStore(tmpDir)

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	return store, tmpDir, cleanup
}

func TestFinancialLogic_CalculateCash(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	fl := NewFinancialLogic(store)

	tests := []struct {
		name     string
		data     map[string]float64
		expected *float64
	}{
		{
			name:     "Cash present",
			data:     map[string]float64{"cash": 500000},
			expected: floatPtr(500000),
		},
		{
			name:     "Cash missing",
			data:     map[string]float64{"revenue": 1000000},
			expected: nil,
		},
		{
			name:     "Zero cash",
			data:     map[string]float64{"cash": 0},
			expected: floatPtr(0),
		},
		{
			name:     "Negative cash",
			data:     map[string]float64{"cash": -50000},
			expected: floatPtr(-50000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fl.CalculateCash(tt.data)
			if tt.expected == nil && result != nil {
				t.Errorf("Expected nil, got %f", *result)
			} else if tt.expected != nil && result == nil {
				t.Errorf("Expected %f, got nil", *tt.expected)
			} else if tt.expected != nil && *result != *tt.expected {
				t.Errorf("Expected %f, got %f", *tt.expected, *result)
			}
		})
	}
}

func TestFinancialLogic_CalculateMonthlyBurn(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	fl := NewFinancialLogic(store)

	tests := []struct {
		name     string
		data     map[string]float64
		expected *float64
	}{
		{
			name:     "Expenses and revenue - burning cash",
			data:     map[string]float64{"expenses": 100000, "revenue": 80000},
			expected: floatPtr(20000), // Burning 20k/month
		},
		{
			name:     "Expenses only",
			data:     map[string]float64{"expenses": 100000},
			expected: floatPtr(100000),
		},
		{
			name:     "Profitable - negative burn",
			data:     map[string]float64{"expenses": 80000, "revenue": 120000},
			expected: floatPtr(-40000), // Profitable!
		},
		{
			name:     "No expenses",
			data:     map[string]float64{"revenue": 100000},
			expected: nil,
		},
		{
			name:     "Break even",
			data:     map[string]float64{"expenses": 100000, "revenue": 100000},
			expected: floatPtr(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fl.CalculateMonthlyBurn(tt.data)
			if tt.expected == nil && result != nil {
				t.Errorf("Expected nil, got %f", *result)
			} else if tt.expected != nil && result == nil {
				t.Errorf("Expected %f, got nil", *tt.expected)
			} else if tt.expected != nil && *result != *tt.expected {
				t.Errorf("Expected %f, got %f", *tt.expected, *result)
			}
		})
	}
}

func TestFinancialLogic_CalculateRunway(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	fl := NewFinancialLogic(store)

	tests := []struct {
		name        string
		cash        *float64
		monthlyBurn *float64
		expected    *float64
	}{
		{
			name:        "Normal runway",
			cash:        floatPtr(1000000),
			monthlyBurn: floatPtr(100000),
			expected:    floatPtr(10), // 10 months
		},
		{
			name:        "Short runway",
			cash:        floatPtr(150000),
			monthlyBurn: floatPtr(50000),
			expected:    floatPtr(3), // 3 months
		},
		{
			name:        "Profitable - infinite runway",
			cash:        floatPtr(500000),
			monthlyBurn: floatPtr(-20000),
			expected:    floatPtr(999), // Effectively infinite
		},
		{
			name:        "Zero burn - sustainable",
			cash:        floatPtr(500000),
			monthlyBurn: floatPtr(0),
			expected:    floatPtr(999),
		},
		{
			name:        "No cash data",
			cash:        nil,
			monthlyBurn: floatPtr(50000),
			expected:    nil,
		},
		{
			name:        "No burn data",
			cash:        floatPtr(500000),
			monthlyBurn: nil,
			expected:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fl.CalculateRunway(tt.cash, tt.monthlyBurn)
			if tt.expected == nil && result != nil {
				t.Errorf("Expected nil, got %f", *result)
			} else if tt.expected != nil && result == nil {
				t.Errorf("Expected %f, got nil", *tt.expected)
			} else if tt.expected != nil && *result != *tt.expected {
				t.Errorf("Expected %f, got %f", *tt.expected, *result)
			}
		})
	}
}

func TestFinancialLogic_ComparePeriods(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	fl := NewFinancialLogic(store)

	tests := []struct {
		name                string
		current             *model.ParsedDocument
		previous            *model.ParsedDocument
		expectedRevChange   *float64
		expectedExpChange   *float64
		expectedCashChange  *float64
	}{
		{
			name: "Revenue growth",
			current: &model.ParsedDocument{
				Data: map[string]float64{"revenue": 1200000},
			},
			previous: &model.ParsedDocument{
				Data: map[string]float64{"revenue": 1000000},
			},
			expectedRevChange: floatPtr(20), // 20% growth
		},
		{
			name: "Revenue decline",
			current: &model.ParsedDocument{
				Data: map[string]float64{"revenue": 800000},
			},
			previous: &model.ParsedDocument{
				Data: map[string]float64{"revenue": 1000000},
			},
			expectedRevChange: floatPtr(-20), // 20% decline
		},
		{
			name: "Multiple metrics change",
			current: &model.ParsedDocument{
				Data: map[string]float64{
					"revenue":  1100000,
					"expenses": 900000,
					"cash":     600000,
				},
			},
			previous: &model.ParsedDocument{
				Data: map[string]float64{
					"revenue":  1000000,
					"expenses": 800000,
					"cash":     500000,
				},
			},
			expectedRevChange:  floatPtr(10),   // 10% revenue growth
			expectedExpChange:  floatPtr(12.5), // 12.5% expense increase
			expectedCashChange: floatPtr(20),   // 20% cash increase
		},
		{
			name:     "Nil documents",
			current:  nil,
			previous: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fl.ComparePeriods(tt.current, tt.previous)

			if tt.current == nil || tt.previous == nil {
				if result != nil {
					t.Error("Expected nil result for nil documents")
				}
				return
			}

			if result == nil {
				t.Fatal("Expected non-nil result")
			}

			if tt.expectedRevChange != nil {
				if result.RevenueChange == nil {
					t.Error("Expected revenue change")
				} else if *result.RevenueChange != *tt.expectedRevChange {
					t.Errorf("Revenue change: expected %f, got %f", *tt.expectedRevChange, *result.RevenueChange)
				}
			}

			if tt.expectedExpChange != nil {
				if result.ExpenseChange == nil {
					t.Error("Expected expense change")
				} else if *result.ExpenseChange != *tt.expectedExpChange {
					t.Errorf("Expense change: expected %f, got %f", *tt.expectedExpChange, *result.ExpenseChange)
				}
			}

			if tt.expectedCashChange != nil {
				if result.CashChange == nil {
					t.Error("Expected cash change")
				} else if *result.CashChange != *tt.expectedCashChange {
					t.Errorf("Cash change: expected %f, got %f", *tt.expectedCashChange, *result.CashChange)
				}
			}
		})
	}
}

func TestFinancialLogic_CalculateCurrentMetrics_NoDocuments(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	fl := NewFinancialLogic(store)

	metrics, err := fl.CalculateCurrentMetrics()
	if err != nil {
		t.Fatalf("Should not error with no documents: %v", err)
	}

	if len(metrics.Errors) == 0 {
		t.Error("Should have error message about no documents")
	}

	if len(metrics.DataSources) != 0 {
		t.Error("Should have no data sources")
	}
}

func TestFinancialLogic_CalculateCurrentMetrics_WithDocuments(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	// Create a parsed document
	parsed := &model.ParsedDocument{
		DocumentID: "test_doc_1",
		DocType:    model.DocTypePnL,
		Period: model.Period{
			Start: "2024-01-01",
			End:   "2024-12-31",
		},
		Data: map[string]float64{
			"revenue":           1000000,
			"expenses":          800000,
			"cash":              500000,
			"net_income":        200000,
			"total_assets":      2000000,
			"total_liabilities": 1000000,
			"equity":            1000000,
		},
		ParsedAt: time.Now(),
	}

	err := store.SaveParsedDocument(parsed)
	if err != nil {
		t.Fatalf("Failed to save parsed document: %v", err)
	}

	fl := NewFinancialLogic(store)
	metrics, err := fl.CalculateCurrentMetrics()
	if err != nil {
		t.Fatalf("Failed to calculate metrics: %v", err)
	}

	// Verify all metrics are populated
	if metrics.Cash == nil || *metrics.Cash != 500000 {
		t.Errorf("Cash: expected 500000, got %v", metrics.Cash)
	}

	if metrics.Revenue == nil || *metrics.Revenue != 1000000 {
		t.Errorf("Revenue: expected 1000000, got %v", metrics.Revenue)
	}

	if metrics.Expenses == nil || *metrics.Expenses != 800000 {
		t.Errorf("Expenses: expected 800000, got %v", metrics.Expenses)
	}

	if metrics.MonthlyBurn == nil || *metrics.MonthlyBurn != -200000 {
		t.Errorf("MonthlyBurn: expected -200000, got %v", metrics.MonthlyBurn)
	}

	// Profitable company should have "infinite" runway
	if metrics.RunwayMonths == nil || *metrics.RunwayMonths != 999 {
		t.Errorf("RunwayMonths: expected 999 (infinite), got %v", metrics.RunwayMonths)
	}

	if len(metrics.DataSources) != 1 {
		t.Errorf("Expected 1 data source, got %d", len(metrics.DataSources))
	}
}

func TestFinancialLogic_FormatMetricsForPrompt(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	fl := NewFinancialLogic(store)

	cash := 500000.0
	burn := 50000.0
	runway := 10.0
	revenue := 1000000.0

	metrics := &model.FinancialMetrics{
		Cash:         &cash,
		MonthlyBurn:  &burn,
		RunwayMonths: &runway,
		Revenue:      &revenue,
		PeriodStart:  "2024-01-01",
		PeriodEnd:    "2024-12-31",
	}

	formatted := fl.FormatMetricsForPrompt(metrics)

	if len(formatted) == 0 {
		t.Error("Should return formatted strings")
	}

	// Check that formatted strings contain expected values
	hasFound := map[string]bool{
		"Cash":    false,
		"Burn":    false,
		"Runway":  false,
		"Revenue": false,
	}

	for _, s := range formatted {
		for key := range hasFound {
			if contains(s, key) {
				hasFound[key] = true
			}
		}
	}

	for key, found := range hasFound {
		if !found {
			t.Errorf("Missing %s in formatted output", key)
		}
	}
}

func TestFinancialLogic_FormatMetricsForPrompt_NoData(t *testing.T) {
	store, _, cleanup := setupTestStorage(t)
	defer cleanup()

	fl := NewFinancialLogic(store)

	metrics := &model.FinancialMetrics{}

	formatted := fl.FormatMetricsForPrompt(metrics)

	if len(formatted) == 0 {
		t.Error("Should return at least a 'no data' message")
	}
}

// Helper functions
func floatPtr(f float64) *float64 {
	return &f
}

func contains(s, substr string) bool {
	return filepath.Base(s) != "" && len(s) > 0 && len(substr) > 0 && 
		(s == substr || len(s) >= len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

