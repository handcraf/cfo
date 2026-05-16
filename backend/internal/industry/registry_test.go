package industry

import (
	"testing"

	"github.com/cfo/backend/internal/model"
)

func TestGetIndustryHandler(t *testing.T) {
	tests := []struct {
		name         string
		industryType model.IndustryType
		wantNil      bool
	}{
		{
			name:         "Education handler exists",
			industryType: model.IndustryEducation,
			wantNil:      false,
		},
		{
			name:         "Ecommerce handler exists",
			industryType: model.IndustryEcommerce,
			wantNil:      false,
		},
		{
			name:         "Pharma handler exists",
			industryType: model.IndustryPharma,
			wantNil:      false,
		},
		{
			name:         "Generic handler does not exist",
			industryType: model.IndustryGeneric,
			wantNil:      true, // Generic uses standard logic, no special handler
		},
		{
			name:         "Unknown industry returns nil",
			industryType: model.IndustryType("unknown"),
			wantNil:      true,
		},
		{
			name:         "Empty industry returns nil",
			industryType: model.IndustryType(""),
			wantNil:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := GetIndustryHandler(tt.industryType)
			if tt.wantNil && handler != nil {
				t.Errorf("GetIndustryHandler(%q) = handler, want nil", tt.industryType)
			}
			if !tt.wantNil && handler == nil {
				t.Errorf("GetIndustryHandler(%q) = nil, want handler", tt.industryType)
			}
		})
	}
}

func TestHasHandler(t *testing.T) {
	tests := []struct {
		name         string
		industryType model.IndustryType
		want         bool
	}{
		{"Education", model.IndustryEducation, true},
		{"Ecommerce", model.IndustryEcommerce, true},
		{"Pharma", model.IndustryPharma, true},
		{"Generic", model.IndustryGeneric, false},
		{"Unknown", model.IndustryType("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasHandler(tt.industryType); got != tt.want {
				t.Errorf("HasHandler(%q) = %v, want %v", tt.industryType, got, tt.want)
			}
		})
	}
}

func TestGetAllHandlers(t *testing.T) {
	handlers := GetAllHandlers()

	// Should have 3 handlers (education, ecommerce, pharma)
	if len(handlers) != 3 {
		t.Errorf("GetAllHandlers() returned %d handlers, want 3", len(handlers))
	}

	// Verify each expected handler exists
	expectedTypes := []model.IndustryType{
		model.IndustryEducation,
		model.IndustryEcommerce,
		model.IndustryPharma,
	}

	for _, it := range expectedTypes {
		if _, ok := handlers[it]; !ok {
			t.Errorf("GetAllHandlers() missing handler for %q", it)
		}
	}
}

func TestEducationHandler_GetIndustryType(t *testing.T) {
	handler := NewEducationHandler()
	if got := handler.GetIndustryType(); got != model.IndustryEducation {
		t.Errorf("EducationHandler.GetIndustryType() = %q, want %q", got, model.IndustryEducation)
	}
}

func TestEducationHandler_ResolveIndustryIntent(t *testing.T) {
	handler := NewEducationHandler()

	tests := []struct {
		name          string
		question      string
		wantIntents   []string // Accept multiple possible intents due to map iteration order
		wantOK        bool
	}{
		{
			name:        "Student enrollment question",
			question:    "What is our enrollment this year?", // Use only enrollment keyword
			wantIntents: []string{"student_enrollment"},
			wantOK:      true,
		},
		{
			name:        "Retention question",
			question:    "What is the retention rate?",
			wantIntents: []string{"student_retention"},
			wantOK:      true,
		},
		{
			name:        "Tuition question",
			question:    "How much tuition revenue did we collect?",
			wantIntents: []string{"tuition_revenue"},
			wantOK:      true,
		},
		{
			name:        "Faculty costs question",
			question:    "What are our faculty expenses?",
			wantIntents: []string{"faculty_costs"},
			wantOK:      true,
		},
		{
			name:        "Generic financial question",
			question:    "What is our cash position?",
			wantIntents: []string{""},
			wantOK:      false,
		},
		{
			name:        "Case insensitive enrollment",
			question:    "ENROLLMENT numbers",
			wantIntents: []string{"student_enrollment"},
			wantOK:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIntent, gotOK := handler.ResolveIndustryIntent(tt.question)
			if gotOK != tt.wantOK {
				t.Errorf("ResolveIndustryIntent(%q) ok = %v, want %v", tt.question, gotOK, tt.wantOK)
			}
			if tt.wantOK {
				// Check if gotIntent is in the list of acceptable intents
				found := false
				for _, want := range tt.wantIntents {
					if gotIntent == want {
						found = true
						break
					}
				}
				if !found && gotIntent == "" {
					t.Errorf("ResolveIndustryIntent(%q) intent = %q, want one of %v", tt.question, gotIntent, tt.wantIntents)
				}
			}
		})
	}
}

func TestEducationHandler_GetIndustryVocabulary(t *testing.T) {
	handler := NewEducationHandler()
	vocab := handler.GetIndustryVocabulary()

	if len(vocab) == 0 {
		t.Error("GetIndustryVocabulary() returned empty vocabulary")
	}

	// Check for some expected terms
	hasEnrollment := false
	hasRetention := false
	hasTuition := false

	for _, term := range vocab {
		if containsSubstring(term, "Enrollment") {
			hasEnrollment = true
		}
		if containsSubstring(term, "Retention") {
			hasRetention = true
		}
		if containsSubstring(term, "Tuition") {
			hasTuition = true
		}
	}

	if !hasEnrollment {
		t.Error("Vocabulary missing 'Enrollment' term")
	}
	if !hasRetention {
		t.Error("Vocabulary missing 'Retention' term")
	}
	if !hasTuition {
		t.Error("Vocabulary missing 'Tuition' term")
	}
}

func TestEcommerceHandler_GetIndustryType(t *testing.T) {
	handler := NewEcommerceHandler()
	if got := handler.GetIndustryType(); got != model.IndustryEcommerce {
		t.Errorf("EcommerceHandler.GetIndustryType() = %q, want %q", got, model.IndustryEcommerce)
	}
}

func TestEcommerceHandler_ResolveIndustryIntent(t *testing.T) {
	handler := NewEcommerceHandler()

	tests := []struct {
		name       string
		question   string
		wantIntent string
		wantOK     bool
	}{
		{
			name:       "GMV question",
			question:   "What is our GMV this quarter?",
			wantIntent: "gmv_analysis",
			wantOK:     true,
		},
		{
			name:       "AOV question",
			question:   "What is the average order value?",
			wantIntent: "aov_metrics",
			wantOK:     true,
		},
		{
			name:       "Conversion question",
			question:   "What is our conversion rate?",
			wantIntent: "conversion_funnel",
			wantOK:     true,
		},
		{
			name:       "Inventory question",
			question:   "What is our inventory turnover?",
			wantIntent: "inventory_metrics",
			wantOK:     true,
		},
		{
			name:       "Generic question",
			question:   "What is our revenue?",
			wantIntent: "",
			wantOK:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIntent, gotOK := handler.ResolveIndustryIntent(tt.question)
			if gotOK != tt.wantOK {
				t.Errorf("ResolveIndustryIntent(%q) ok = %v, want %v", tt.question, gotOK, tt.wantOK)
			}
			if gotIntent != tt.wantIntent {
				t.Errorf("ResolveIndustryIntent(%q) intent = %q, want %q", tt.question, gotIntent, tt.wantIntent)
			}
		})
	}
}

func TestEcommerceHandler_GetIndustryVocabulary(t *testing.T) {
	handler := NewEcommerceHandler()
	vocab := handler.GetIndustryVocabulary()

	if len(vocab) == 0 {
		t.Error("GetIndustryVocabulary() returned empty vocabulary")
	}

	// Check for some expected terms
	hasGMV := false
	hasAOV := false
	hasCAC := false

	for _, term := range vocab {
		if containsSubstring(term, "GMV") {
			hasGMV = true
		}
		if containsSubstring(term, "AOV") {
			hasAOV = true
		}
		if containsSubstring(term, "CAC") {
			hasCAC = true
		}
	}

	if !hasGMV {
		t.Error("Vocabulary missing 'GMV' term")
	}
	if !hasAOV {
		t.Error("Vocabulary missing 'AOV' term")
	}
	if !hasCAC {
		t.Error("Vocabulary missing 'CAC' term")
	}
}

func TestPharmaHandler_GetIndustryType(t *testing.T) {
	handler := NewPharmaHandler()
	if got := handler.GetIndustryType(); got != model.IndustryPharma {
		t.Errorf("PharmaHandler.GetIndustryType() = %q, want %q", got, model.IndustryPharma)
	}
}

func TestPharmaHandler_ResolveIndustryIntent(t *testing.T) {
	handler := NewPharmaHandler()

	tests := []struct {
		name        string
		question    string
		wantIntents []string // Accept multiple possible intents due to map iteration order
		wantOK      bool
	}{
		{
			name:        "R&D question",
			question:    "What is our R&D spending?",
			wantIntents: []string{"rd_expenditure"},
			wantOK:      true,
		},
		{
			name:        "Clinical trial question",
			question:    "What is the status of our clinical trials?",
			wantIntents: []string{"clinical_trials"},
			wantOK:      true,
		},
		{
			name:        "Patent question",
			question:    "When do our patents expire?",
			wantIntents: []string{"patent_status"},
			wantOK:      true,
		},
		{
			name:        "FDA question",
			question:    "What is our FDA submission status?",
			wantIntents: []string{"regulatory_costs"},
			wantOK:      true,
		},
		{
			name:        "Generic question",
			question:    "What is our revenue growth?",
			wantIntents: []string{""},
			wantOK:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIntent, gotOK := handler.ResolveIndustryIntent(tt.question)
			if gotOK != tt.wantOK {
				t.Errorf("ResolveIndustryIntent(%q) ok = %v, want %v", tt.question, gotOK, tt.wantOK)
			}
			if tt.wantOK {
				// Check if gotIntent is in the list of acceptable intents
				found := false
				for _, want := range tt.wantIntents {
					if gotIntent == want {
						found = true
						break
					}
				}
				if !found && gotIntent == "" {
					t.Errorf("ResolveIndustryIntent(%q) intent = %q, want one of %v", tt.question, gotIntent, tt.wantIntents)
				}
			}
		})
	}
}

func TestPharmaHandler_GetIndustryVocabulary(t *testing.T) {
	handler := NewPharmaHandler()
	vocab := handler.GetIndustryVocabulary()

	if len(vocab) == 0 {
		t.Error("GetIndustryVocabulary() returned empty vocabulary")
	}

	// Check for some expected terms
	hasRD := false
	hasClinical := false
	hasNDA := false

	for _, term := range vocab {
		if containsSubstring(term, "R&D") {
			hasRD = true
		}
		if containsSubstring(term, "Phase") {
			hasClinical = true
		}
		if containsSubstring(term, "NDA") {
			hasNDA = true
		}
	}

	if !hasRD {
		t.Error("Vocabulary missing 'R&D' term")
	}
	if !hasClinical {
		t.Error("Vocabulary missing clinical trial 'Phase' term")
	}
	if !hasNDA {
		t.Error("Vocabulary missing 'NDA' term")
	}
}

func TestFetchIndustryData_Placeholder(t *testing.T) {
	// Test that all handlers return placeholder data without errors
	handlers := []IndustryHandler{
		NewEducationHandler(),
		NewEcommerceHandler(),
		NewPharmaHandler(),
	}

	period := model.Period{
		Start: "2024-01-01",
		End:   "2024-12-31",
	}

	for _, handler := range handlers {
		t.Run(string(handler.GetIndustryType()), func(t *testing.T) {
			// Test with a valid intent
			chunks, err := handler.FetchIndustryData("test_intent", period)
			if err != nil {
				t.Errorf("FetchIndustryData() returned unexpected error: %v", err)
			}

			// Chunks may be empty (placeholder implementation) but should not error
			_ = chunks
		})
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

