package service

import (
	"fmt"
	"sort"
	"time"

	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/storage"
)

// FinancialLogic handles all deterministic financial calculations
// IMPORTANT: All calculations are done in CODE, NOT by LLM
type FinancialLogic struct {
	store *storage.FileStore
}

// NewFinancialLogic creates a new FinancialLogic service
func NewFinancialLogic(store *storage.FileStore) *FinancialLogic {
	return &FinancialLogic{store: store}
}

// CalculateCurrentMetrics calculates all current financial metrics
func (f *FinancialLogic) CalculateCurrentMetrics() (*model.FinancialMetrics, error) {
	// Load all parsed documents
	docs, err := f.store.LoadAllParsedDocuments()
	if err != nil {
		return nil, fmt.Errorf("failed to load parsed documents: %w", err)
	}

	if len(docs) == 0 {
		return &model.FinancialMetrics{
			Errors:      []string{"No documents uploaded yet"},
			DataSources: []string{},
		}, nil
	}

	// Sort documents by period end date (most recent first)
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Period.End > docs[j].Period.End
	})

	metrics := &model.FinancialMetrics{
		DataSources: make([]string, 0),
	}

	// Aggregate data from all documents
	// For MVP, we take the most recent value for each metric
	aggregated := make(map[string]float64)
	for _, doc := range docs {
		metrics.DataSources = append(metrics.DataSources, doc.DocumentID)
		for key, value := range doc.Data {
			// Only set if not already set (prefer most recent)
			if _, exists := aggregated[key]; !exists {
				aggregated[key] = value
			}
		}
		// Set period from most recent doc
		if metrics.PeriodEnd == "" {
			metrics.PeriodStart = doc.Period.Start
			metrics.PeriodEnd = doc.Period.End
		}
	}

	// Calculate metrics from aggregated data
	if cash, ok := aggregated["cash"]; ok {
		metrics.Cash = &cash
	}

	if revenue, ok := aggregated["revenue"]; ok {
		metrics.Revenue = &revenue
	}

	if expenses, ok := aggregated["expenses"]; ok {
		metrics.Expenses = &expenses
	}

	if netIncome, ok := aggregated["net_income"]; ok {
		metrics.NetIncome = &netIncome
	}

	if totalAssets, ok := aggregated["total_assets"]; ok {
		metrics.TotalAssets = &totalAssets
	}

	if totalLiab, ok := aggregated["total_liabilities"]; ok {
		metrics.TotalLiab = &totalLiab
	}

	if equity, ok := aggregated["equity"]; ok {
		metrics.Equity = &equity
	}

	// Calculate derived metrics
	metrics.MonthlyBurn = f.CalculateMonthlyBurn(aggregated)
	metrics.RunwayMonths = f.CalculateRunway(metrics.Cash, metrics.MonthlyBurn)

	// Calculate trends if we have multiple periods
	if len(docs) >= 2 {
		metrics.Trends = f.ComparePeriods(docs[0], docs[1])
	}

	return metrics, nil
}

// CalculateCash returns the current cash position
func (f *FinancialLogic) CalculateCash(data map[string]float64) *float64 {
	if cash, ok := data["cash"]; ok {
		return &cash
	}
	return nil
}

// CalculateMonthlyBurn calculates the monthly burn rate
// Burn = Expenses - Revenue (if negative, company is profitable)
func (f *FinancialLogic) CalculateMonthlyBurn(data map[string]float64) *float64 {
	expenses, hasExpenses := data["expenses"]
	revenue, hasRevenue := data["revenue"]

	if !hasExpenses {
		return nil
	}

	var burn float64
	if hasRevenue {
		burn = expenses - revenue
	} else {
		burn = expenses
	}

	// Convert to monthly if we have annual data
	// TODO: Detect if data is annual, quarterly, or monthly
	// For now, assume data is already monthly

	return &burn
}

// CalculateRunway calculates runway in months
// Runway = Cash / Monthly Burn
func (f *FinancialLogic) CalculateRunway(cash *float64, monthlyBurn *float64) *float64 {
	if cash == nil || monthlyBurn == nil {
		return nil
	}

	// If burn is zero or negative (profitable), runway is effectively infinite
	if *monthlyBurn <= 0 {
		// Return a large number to indicate profitable/sustainable
		infinite := 999.0
		return &infinite
	}

	runway := *cash / *monthlyBurn
	return &runway
}

// ComparePeriods compares two periods and returns trend data
func (f *FinancialLogic) ComparePeriods(current, previous *model.ParsedDocument) *model.TrendData {
	if current == nil || previous == nil {
		return nil
	}

	trends := &model.TrendData{}

	// Calculate percentage changes
	if currRev, ok := current.Data["revenue"]; ok {
		if prevRev, ok := previous.Data["revenue"]; ok && prevRev != 0 {
			change := ((currRev - prevRev) / prevRev) * 100
			trends.RevenueChange = &change
		}
	}

	if currExp, ok := current.Data["expenses"]; ok {
		if prevExp, ok := previous.Data["expenses"]; ok && prevExp != 0 {
			change := ((currExp - prevExp) / prevExp) * 100
			trends.ExpenseChange = &change
		}
	}

	if currCash, ok := current.Data["cash"]; ok {
		if prevCash, ok := previous.Data["cash"]; ok && prevCash != 0 {
			change := ((currCash - prevCash) / prevCash) * 100
			trends.CashChange = &change
		}
	}

	return trends
}

// FormatMetricsForPrompt formats metrics as strings for LLM prompts
func (f *FinancialLogic) FormatMetricsForPrompt(metrics *model.FinancialMetrics) []string {
	var numbers []string

	if metrics.Cash != nil {
		numbers = append(numbers, fmt.Sprintf("Cash: $%.2f", *metrics.Cash))
	}
	if metrics.MonthlyBurn != nil {
		numbers = append(numbers, fmt.Sprintf("Monthly Burn: $%.2f", *metrics.MonthlyBurn))
	}
	if metrics.RunwayMonths != nil {
		if *metrics.RunwayMonths >= 999 {
			numbers = append(numbers, "Runway: Profitable (sustainable)")
		} else {
			numbers = append(numbers, fmt.Sprintf("Runway: %.1f months", *metrics.RunwayMonths))
		}
	}
	if metrics.Revenue != nil {
		numbers = append(numbers, fmt.Sprintf("Revenue: $%.2f", *metrics.Revenue))
	}
	if metrics.Expenses != nil {
		numbers = append(numbers, fmt.Sprintf("Expenses: $%.2f", *metrics.Expenses))
	}
	if metrics.NetIncome != nil {
		numbers = append(numbers, fmt.Sprintf("Net Income: $%.2f", *metrics.NetIncome))
	}
	if metrics.TotalAssets != nil {
		numbers = append(numbers, fmt.Sprintf("Total Assets: $%.2f", *metrics.TotalAssets))
	}
	if metrics.TotalLiab != nil {
		numbers = append(numbers, fmt.Sprintf("Total Liabilities: $%.2f", *metrics.TotalLiab))
	}
	if metrics.Equity != nil {
		numbers = append(numbers, fmt.Sprintf("Equity: $%.2f", *metrics.Equity))
	}

	// Add trend information
	if metrics.Trends != nil {
		if metrics.Trends.RevenueChange != nil {
			numbers = append(numbers, fmt.Sprintf("Revenue Change: %.1f%%", *metrics.Trends.RevenueChange))
		}
		if metrics.Trends.ExpenseChange != nil {
			numbers = append(numbers, fmt.Sprintf("Expense Change: %.1f%%", *metrics.Trends.ExpenseChange))
		}
		if metrics.Trends.CashChange != nil {
			numbers = append(numbers, fmt.Sprintf("Cash Change: %.1f%%", *metrics.Trends.CashChange))
		}
	}

	if len(numbers) == 0 {
		numbers = append(numbers, "No financial data available yet")
	}

	// Add period info
	if metrics.PeriodEnd != "" {
		numbers = append(numbers, fmt.Sprintf("Period: %s to %s", metrics.PeriodStart, metrics.PeriodEnd))
	}

	return numbers
}

// GetLatestPeriodEnd returns the end date of the most recent period
func (f *FinancialLogic) GetLatestPeriodEnd() (time.Time, error) {
	docs, err := f.store.LoadAllParsedDocuments()
	if err != nil {
		return time.Time{}, err
	}

	if len(docs) == 0 {
		return time.Time{}, fmt.Errorf("no documents available")
	}

	// Find the latest period end
	var latest time.Time
	for _, doc := range docs {
		if doc.Period.End != "" {
			t, err := time.Parse("2006-01-02", doc.Period.End)
			if err == nil && t.After(latest) {
				latest = t
			}
		}
	}

	return latest, nil
}

// CalculateMetricsForPeriod calculates financial metrics for a specific period
func (f *FinancialLogic) CalculateMetricsForPeriod(startDate, endDate string) (*model.FinancialMetrics, error) {
	// Load all parsed documents
	docs, err := f.store.LoadAllParsedDocuments()
	if err != nil {
		return nil, fmt.Errorf("failed to load parsed documents: %w", err)
	}

	if len(docs) == 0 {
		return &model.FinancialMetrics{
			Errors:      []string{"No documents uploaded yet"},
			DataSources: []string{},
		}, nil
	}

	// Parse the target period
	targetStart, err1 := time.Parse("2006-01-02", startDate)
	targetEnd, err2 := time.Parse("2006-01-02", endDate)
	if err1 != nil || err2 != nil {
		// Fall back to current metrics if date parsing fails
		return f.CalculateCurrentMetrics()
	}

	// Filter documents that overlap with the target period
	var matchingDocs []*model.ParsedDocument
	for _, doc := range docs {
		if f.periodOverlaps(doc.Period.Start, doc.Period.End, targetStart, targetEnd) {
			matchingDocs = append(matchingDocs, doc)
		}
	}

	if len(matchingDocs) == 0 {
		return &model.FinancialMetrics{
			PeriodStart: startDate,
			PeriodEnd:   endDate,
			Errors:      []string{fmt.Sprintf("No data available for period %s to %s", startDate, endDate)},
			DataSources: []string{},
		}, nil
	}

	// Sort by period end (most recent first within the filtered set)
	sort.Slice(matchingDocs, func(i, j int) bool {
		return matchingDocs[i].Period.End > matchingDocs[j].Period.End
	})

	metrics := &model.FinancialMetrics{
		PeriodStart: startDate,
		PeriodEnd:   endDate,
		DataSources: make([]string, 0),
	}

	// Aggregate data from matching documents
	aggregated := make(map[string]float64)
	for _, doc := range matchingDocs {
		metrics.DataSources = append(metrics.DataSources, doc.DocumentID)
		for key, value := range doc.Data {
			if _, exists := aggregated[key]; !exists {
				aggregated[key] = value
			}
		}
	}

	// Calculate metrics from aggregated data
	if cash, ok := aggregated["cash"]; ok {
		metrics.Cash = &cash
	}
	if revenue, ok := aggregated["revenue"]; ok {
		metrics.Revenue = &revenue
	}
	if expenses, ok := aggregated["expenses"]; ok {
		metrics.Expenses = &expenses
	}
	if netIncome, ok := aggregated["net_income"]; ok {
		metrics.NetIncome = &netIncome
	}
	if totalAssets, ok := aggregated["total_assets"]; ok {
		metrics.TotalAssets = &totalAssets
	}
	if totalLiab, ok := aggregated["total_liabilities"]; ok {
		metrics.TotalLiab = &totalLiab
	}
	if equity, ok := aggregated["equity"]; ok {
		metrics.Equity = &equity
	}

	// Calculate derived metrics
	metrics.MonthlyBurn = f.CalculateMonthlyBurn(aggregated)
	metrics.RunwayMonths = f.CalculateRunway(metrics.Cash, metrics.MonthlyBurn)

	return metrics, nil
}

// periodOverlaps checks if a document's period overlaps with the target period
func (f *FinancialLogic) periodOverlaps(docStart, docEnd string, targetStart, targetEnd time.Time) bool {
	// Parse document dates
	dStart, err1 := time.Parse("2006-01-02", docStart)
	dEnd, err2 := time.Parse("2006-01-02", docEnd)

	if err1 != nil || err2 != nil {
		// If we can't parse dates, try year matching
		targetYear := targetStart.Year()
		if containsYear(docStart, targetYear) || containsYear(docEnd, targetYear) {
			return true
		}
		return false
	}

	// Periods overlap if one doesn't end before the other starts
	return !(targetEnd.Before(dStart) || dEnd.Before(targetStart))
}

// containsYear checks if a date string contains a specific year
func containsYear(dateStr string, year int) bool {
	yearStr := fmt.Sprintf("%d", year)
	return len(dateStr) >= 4 && dateStr[:4] == yearStr
}
