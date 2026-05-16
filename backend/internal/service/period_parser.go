package service

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParsedPeriod represents a time period extracted from a query
type ParsedPeriod struct {
	Start     string // YYYY-MM-DD
	End       string // YYYY-MM-DD
	Label     string // Human-readable label (e.g., "Q4 2024")
	Detected  bool   // Whether a period was detected
	YearOnly  bool   // Only year was specified
	QuarterNo int    // Quarter number (1-4) if detected
	Year      int    // Year if detected
}

// PeriodParser extracts time periods from natural language questions
type PeriodParser struct{}

// NewPeriodParser creates a new PeriodParser
func NewPeriodParser() *PeriodParser {
	return &PeriodParser{}
}

// Parse extracts a time period from a question
func (p *PeriodParser) Parse(question string) ParsedPeriod {
	q := strings.ToLower(question)
	now := time.Now()
	currentYear := now.Year()

	// Try different patterns in order of specificity

	// Pattern 1: Explicit quarter (Q1, Q2, Q3, Q4) with year
	if period := p.parseQuarterWithYear(q); period.Detected {
		return period
	}

	// Pattern 2: Quarter without year (assumes current or previous year)
	if period := p.parseQuarterOnly(q, currentYear); period.Detected {
		return period
	}

	// Pattern 3: Month with year (e.g., "January 2024", "Jan 2024")
	if period := p.parseMonthYear(q); period.Detected {
		return period
	}

	// Pattern 4: Year only (e.g., "2024", "FY2024", "fiscal year 2024")
	if period := p.parseYearOnly(q); period.Detected {
		return period
	}

	// Pattern 5: Relative periods (e.g., "last quarter", "this year", "last month")
	if period := p.parseRelativePeriod(q, now); period.Detected {
		return period
	}

	// Pattern 6: Date range (e.g., "from January to March 2024")
	if period := p.parseDateRange(q); period.Detected {
		return period
	}

	// No period detected
	return ParsedPeriod{Detected: false}
}

// parseQuarterWithYear handles "Q4 2024", "quarter 4 2024", "4th quarter 2024"
func (p *PeriodParser) parseQuarterWithYear(q string) ParsedPeriod {
	patterns := []string{
		`q([1-4])\s*(\d{4})`,                                    // Q4 2024, Q4-2024
		`quarter\s*([1-4])\s*(\d{4})`,                           // quarter 4 2024
		`([1-4])(?:st|nd|rd|th)?\s*quarter\s*(?:of\s*)?(\d{4})`, // 4th quarter 2024
		`(\d{4})\s*q([1-4])`,                                    // 2024 Q4
	}

	for i, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(q); matches != nil {
			var quarter, year int
			if i == 3 { // Year first pattern
				year, _ = strconv.Atoi(matches[1])
				quarter, _ = strconv.Atoi(matches[2])
			} else {
				quarter, _ = strconv.Atoi(matches[1])
				year, _ = strconv.Atoi(matches[2])
			}
			return p.quarterToPeriod(quarter, year)
		}
	}

	return ParsedPeriod{Detected: false}
}

// parseQuarterOnly handles "Q4", "quarter 4" without year
func (p *PeriodParser) parseQuarterOnly(q string, currentYear int) ParsedPeriod {
	patterns := []string{
		`\bq([1-4])\b`,
		`\bquarter\s*([1-4])\b`,
		`([1-4])(?:st|nd|rd|th)\s*quarter\b`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(q); matches != nil {
			quarter, _ := strconv.Atoi(matches[1])
			// Use current year, or previous year if asking about a future quarter
			year := currentYear
			currentQuarter := (int(time.Now().Month()) + 2) / 3
			if quarter > currentQuarter {
				year = currentYear - 1
			}
			return p.quarterToPeriod(quarter, year)
		}
	}

	return ParsedPeriod{Detected: false}
}

// parseMonthYear handles "January 2024", "Jan 2024", "march of 2024"
func (p *PeriodParser) parseMonthYear(q string) ParsedPeriod {
	months := map[string]int{
		"january": 1, "jan": 1, "february": 2, "feb": 2, "march": 3, "mar": 3,
		"april": 4, "apr": 4, "may": 5, "june": 6, "jun": 6,
		"july": 7, "jul": 7, "august": 8, "aug": 8, "september": 9, "sep": 9, "sept": 9,
		"october": 10, "oct": 10, "november": 11, "nov": 11, "december": 12, "dec": 12,
	}

	for monthName, monthNum := range months {
		pattern := regexp.MustCompile(monthName + `\s*(?:of\s*)?(\d{4})`)
		if matches := pattern.FindStringSubmatch(q); matches != nil {
			year, _ := strconv.Atoi(matches[1])
			start := time.Date(year, time.Month(monthNum), 1, 0, 0, 0, 0, time.UTC)
			end := start.AddDate(0, 1, -1)
			return ParsedPeriod{
				Start:    start.Format("2006-01-02"),
				End:      end.Format("2006-01-02"),
				Label:    start.Format("January 2006"),
				Detected: true,
				Year:     year,
			}
		}
	}

	return ParsedPeriod{Detected: false}
}

// parseYearOnly handles "2024", "FY2024", "fiscal year 2024", "year 2024"
func (p *PeriodParser) parseYearOnly(q string) ParsedPeriod {
	patterns := []string{
		`\bfy\s*(\d{4})\b`,        // FY2024
		`fiscal\s*year\s*(\d{4})`, // fiscal year 2024
		`\byear\s*(\d{4})\b`,      // year 2024
		`\bin\s*(\d{4})\b`,        // in 2024
		`\bfor\s*(\d{4})\b`,       // for 2024
		`\b(20\d{2})\b`,           // Just 2024 (only 20xx years)
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(q); matches != nil {
			year, _ := strconv.Atoi(matches[1])
			if year >= 2000 && year <= 2100 {
				return ParsedPeriod{
					Start:    strconv.Itoa(year) + "-01-01",
					End:      strconv.Itoa(year) + "-12-31",
					Label:    "FY" + strconv.Itoa(year),
					Detected: true,
					YearOnly: true,
					Year:     year,
				}
			}
		}
	}

	return ParsedPeriod{Detected: false}
}

// parseRelativePeriod handles "last quarter", "this year", "last month", "previous quarter"
func (p *PeriodParser) parseRelativePeriod(q string, now time.Time) ParsedPeriod {
	currentQuarter := (int(now.Month()) + 2) / 3
	currentYear := now.Year()

	// Last quarter / previous quarter
	if strings.Contains(q, "last quarter") || strings.Contains(q, "previous quarter") {
		quarter := currentQuarter - 1
		year := currentYear
		if quarter < 1 {
			quarter = 4
			year--
		}
		return p.quarterToPeriod(quarter, year)
	}

	// This quarter / current quarter
	if strings.Contains(q, "this quarter") || strings.Contains(q, "current quarter") {
		return p.quarterToPeriod(currentQuarter, currentYear)
	}

	// Last year / previous year
	if strings.Contains(q, "last year") || strings.Contains(q, "previous year") {
		year := currentYear - 1
		return ParsedPeriod{
			Start:    strconv.Itoa(year) + "-01-01",
			End:      strconv.Itoa(year) + "-12-31",
			Label:    "FY" + strconv.Itoa(year),
			Detected: true,
			YearOnly: true,
			Year:     year,
		}
	}

	// This year / current year
	if strings.Contains(q, "this year") || strings.Contains(q, "current year") {
		return ParsedPeriod{
			Start:    strconv.Itoa(currentYear) + "-01-01",
			End:      strconv.Itoa(currentYear) + "-12-31",
			Label:    "FY" + strconv.Itoa(currentYear),
			Detected: true,
			YearOnly: true,
			Year:     currentYear,
		}
	}

	// Last month / previous month
	if strings.Contains(q, "last month") || strings.Contains(q, "previous month") {
		lastMonth := now.AddDate(0, -1, 0)
		start := time.Date(lastMonth.Year(), lastMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, -1)
		return ParsedPeriod{
			Start:    start.Format("2006-01-02"),
			End:      end.Format("2006-01-02"),
			Label:    start.Format("January 2006"),
			Detected: true,
			Year:     start.Year(),
		}
	}

	return ParsedPeriod{Detected: false}
}

// parseDateRange handles "from January to March 2024"
func (p *PeriodParser) parseDateRange(q string) ParsedPeriod {
	// Simple pattern: from X to Y year
	pattern := regexp.MustCompile(`from\s+(\w+)\s+to\s+(\w+)\s+(\d{4})`)
	if matches := pattern.FindStringSubmatch(q); matches != nil {
		months := map[string]int{
			"january": 1, "jan": 1, "february": 2, "feb": 2, "march": 3, "mar": 3,
			"april": 4, "apr": 4, "may": 5, "june": 6, "jun": 6,
			"july": 7, "jul": 7, "august": 8, "aug": 8, "september": 9, "sep": 9,
			"october": 10, "oct": 10, "november": 11, "nov": 11, "december": 12, "dec": 12,
		}

		startMonth, ok1 := months[strings.ToLower(matches[1])]
		endMonth, ok2 := months[strings.ToLower(matches[2])]
		year, _ := strconv.Atoi(matches[3])

		if ok1 && ok2 {
			start := time.Date(year, time.Month(startMonth), 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(year, time.Month(endMonth)+1, 0, 0, 0, 0, 0, time.UTC)
			return ParsedPeriod{
				Start:    start.Format("2006-01-02"),
				End:      end.Format("2006-01-02"),
				Label:    start.Format("Jan") + " - " + end.Format("Jan 2006"),
				Detected: true,
				Year:     year,
			}
		}
	}

	return ParsedPeriod{Detected: false}
}

// quarterToPeriod converts a quarter number and year to a ParsedPeriod
func (p *PeriodParser) quarterToPeriod(quarter, year int) ParsedPeriod {
	var start, end time.Time

	switch quarter {
	case 1:
		start = time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Date(year, 3, 31, 0, 0, 0, 0, time.UTC)
	case 2:
		start = time.Date(year, 4, 1, 0, 0, 0, 0, time.UTC)
		end = time.Date(year, 6, 30, 0, 0, 0, 0, time.UTC)
	case 3:
		start = time.Date(year, 7, 1, 0, 0, 0, 0, time.UTC)
		end = time.Date(year, 9, 30, 0, 0, 0, 0, time.UTC)
	case 4:
		start = time.Date(year, 10, 1, 0, 0, 0, 0, time.UTC)
		end = time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	}

	return ParsedPeriod{
		Start:     start.Format("2006-01-02"),
		End:       end.Format("2006-01-02"),
		Label:     "Q" + strconv.Itoa(quarter) + " " + strconv.Itoa(year),
		Detected:  true,
		QuarterNo: quarter,
		Year:      year,
	}
}

// IsPeriodMatch checks if a document period overlaps with the parsed period
func (p *PeriodParser) IsPeriodMatch(docStart, docEnd string, period ParsedPeriod) bool {
	if !period.Detected {
		return true // No period filter, match all
	}

	// Parse dates
	pStart, err1 := time.Parse("2006-01-02", period.Start)
	pEnd, err2 := time.Parse("2006-01-02", period.End)

	// Try parsing document dates
	dStart, err3 := time.Parse("2006-01-02", docStart)
	dEnd, err4 := time.Parse("2006-01-02", docEnd)

	// If we can't parse dates, be lenient and match
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		// Try year-only matching
		if period.Year > 0 && (strings.Contains(docStart, strconv.Itoa(period.Year)) ||
			strings.Contains(docEnd, strconv.Itoa(period.Year))) {
			return true
		}
		return true // Be lenient on unparseable dates
	}

	// Check for overlap: periods overlap if one doesn't end before the other starts
	return !(pEnd.Before(dStart) || dEnd.Before(pStart))
}
