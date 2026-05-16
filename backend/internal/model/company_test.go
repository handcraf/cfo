package model

import "testing"

func TestIsValidIndustryType(t *testing.T) {
	tests := []struct {
		name         string
		industryType IndustryType
		want         bool
	}{
		{
			name:         "Generic is valid",
			industryType: IndustryGeneric,
			want:         true,
		},
		{
			name:         "Education is valid",
			industryType: IndustryEducation,
			want:         true,
		},
		{
			name:         "Ecommerce is valid",
			industryType: IndustryEcommerce,
			want:         true,
		},
		{
			name:         "Pharma is valid",
			industryType: IndustryPharma,
			want:         true,
		},
		{
			name:         "Empty string is invalid",
			industryType: IndustryType(""),
			want:         false,
		},
		{
			name:         "Unknown type is invalid",
			industryType: IndustryType("unknown"),
			want:         false,
		},
		{
			name:         "Typo is invalid",
			industryType: IndustryType("educaton"),
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidIndustryType(tt.industryType); got != tt.want {
				t.Errorf("IsValidIndustryType(%q) = %v, want %v", tt.industryType, got, tt.want)
			}
		})
	}
}

func TestValidIndustryTypes(t *testing.T) {
	types := ValidIndustryTypes()

	// Should return exactly 4 types
	if len(types) != 4 {
		t.Errorf("ValidIndustryTypes() returned %d types, want 4", len(types))
	}

	// Verify all expected types are present
	expected := map[IndustryType]bool{
		IndustryGeneric:   false,
		IndustryEducation: false,
		IndustryEcommerce: false,
		IndustryPharma:    false,
	}

	for _, it := range types {
		if _, ok := expected[it]; !ok {
			t.Errorf("ValidIndustryTypes() returned unexpected type: %q", it)
		}
		expected[it] = true
	}

	for it, found := range expected {
		if !found {
			t.Errorf("ValidIndustryTypes() missing expected type: %q", it)
		}
	}
}

func TestIndustryTypeConstants(t *testing.T) {
	// Verify the string values of constants match expected values
	if string(IndustryGeneric) != "generic" {
		t.Errorf("IndustryGeneric = %q, want %q", IndustryGeneric, "generic")
	}
	if string(IndustryEducation) != "education" {
		t.Errorf("IndustryEducation = %q, want %q", IndustryEducation, "education")
	}
	if string(IndustryEcommerce) != "ecommerce" {
		t.Errorf("IndustryEcommerce = %q, want %q", IndustryEcommerce, "ecommerce")
	}
	if string(IndustryPharma) != "pharma" {
		t.Errorf("IndustryPharma = %q, want %q", IndustryPharma, "pharma")
	}
}

func TestCompanyStruct_IndustryTypeField(t *testing.T) {
	company := Company{
		Name:         "Test Corp",
		Industry:     "Technology",
		IndustryType: IndustryEducation,
		Currency:     "USD",
	}

	if company.IndustryType != IndustryEducation {
		t.Errorf("Company.IndustryType = %q, want %q", company.IndustryType, IndustryEducation)
	}

	// Test default value
	defaultCompany := Company{}
	if defaultCompany.IndustryType != "" {
		t.Errorf("Default Company.IndustryType = %q, want empty string", defaultCompany.IndustryType)
	}
}

