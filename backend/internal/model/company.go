package model

import "time"

// IndustryType represents the type of industry for specialized handling
// TODO: Extend with more industries as skill-packs are developed
type IndustryType string

const (
	IndustryGeneric   IndustryType = "generic"   // Default: standard CFO logic
	IndustryEducation IndustryType = "education" // Education sector (schools, universities, edtech)
	IndustryEcommerce IndustryType = "ecommerce" // E-commerce and retail
	IndustryPharma    IndustryType = "pharma"    // Pharmaceutical and healthcare
)

// ValidIndustryTypes returns all valid industry types
func ValidIndustryTypes() []IndustryType {
	return []IndustryType{
		IndustryGeneric,
		IndustryEducation,
		IndustryEcommerce,
		IndustryPharma,
	}
}

// IsValidIndustryType checks if the given industry type is valid
func IsValidIndustryType(it IndustryType) bool {
	for _, valid := range ValidIndustryTypes() {
		if it == valid {
			return true
		}
	}
	return false
}

// Company represents the company setup information
type Company struct {
	Name           string       `json:"name"`
	Industry       string       `json:"industry"`        // Free-form industry description
	IndustryType   IndustryType `json:"industry_type"`   // Enum for specialized handling (generic, education, ecommerce, pharma)
	FiscalYearEnd  string       `json:"fiscal_year_end"` // e.g., "December"
	Currency       string       `json:"currency"`        // e.g., "USD"
	SetupCompleted bool         `json:"setup_completed"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}

// CompanyStatus represents the status response for company setup
type CompanyStatus struct {
	SetupCompleted bool     `json:"setup_completed"`
	Company        *Company `json:"company,omitempty"`
}
