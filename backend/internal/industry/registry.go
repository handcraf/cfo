// Package industry provides pluggable industry-specific intelligence for the CFO system.
// This allows the CFO to understand domain-specific terminology, metrics, and data sources
// for different industries (education, ecommerce, pharma, etc.).
//
// TODO: This is the extensibility layer. Each industry skill-pack should:
// 1. Implement the IndustryHandler interface
// 2. Register itself in the registry
// 3. Provide vocabulary, intent resolution, and data fetching capabilities
package industry

import (
	"log"
	"sync"

	"github.com/cfo/backend/internal/model"
)

// Chunk represents a contextual data chunk from industry-specific RAG
type Chunk struct {
	Text       string                 `json:"text"`
	Source     string                 `json:"source"`
	DocumentID string                 `json:"document_id,omitempty"`
	Score      float64                `json:"score,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// IndustryHandler defines the interface for industry-specific intelligence modules.
// Each industry (education, ecommerce, pharma) implements this interface to provide
// specialized handling of domain-specific questions and data.
type IndustryHandler interface {
	// GetIndustryType returns the industry type this handler supports
	GetIndustryType() model.IndustryType

	// ResolveIndustryIntent attempts to understand industry-specific questions.
	// Returns the resolved intent string and whether resolution was successful.
	// Example: "What is our student retention rate?" -> ("student_retention", true)
	//
	// TODO: Implement actual NLP-based intent resolution for each industry
	ResolveIndustryIntent(question string) (industryIntent string, ok bool)

	// FetchIndustryData retrieves industry-specific context chunks for the given intent.
	// This data will be injected into the LLM prompt for industry-aware responses.
	//
	// TODO: Implement actual data fetching from industry-specific RAG storage
	FetchIndustryData(industryIntent string, period model.Period) (contextChunks []Chunk, err error)

	// GetIndustryVocabulary returns domain-specific terms and their definitions.
	// These terms help the LLM understand industry jargon and acronyms.
	GetIndustryVocabulary() []string
}

// Registry manages all registered industry handlers
type Registry struct {
	mu       sync.RWMutex
	handlers map[model.IndustryType]IndustryHandler
}

// globalRegistry is the singleton registry instance
var globalRegistry = &Registry{
	handlers: make(map[model.IndustryType]IndustryHandler),
}

// init registers all industry handlers at startup
func init() {
	// Register all industry handlers
	// Each handler is a stub that will be fully implemented later
	RegisterHandler(NewEducationHandler())
	RegisterHandler(NewEcommerceHandler())
	RegisterHandler(NewPharmaHandler())

	log.Printf("[Industry Registry] Initialized with %d industry handlers", len(globalRegistry.handlers))
}

// RegisterHandler registers an industry handler in the global registry
func RegisterHandler(handler IndustryHandler) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	industryType := handler.GetIndustryType()
	globalRegistry.handlers[industryType] = handler
	log.Printf("[Industry Registry] Registered handler for: %s", industryType)
}

// GetIndustryHandler returns the handler for the specified industry type.
// Returns nil if no handler is registered for the industry.
func GetIndustryHandler(industryType model.IndustryType) IndustryHandler {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	handler, exists := globalRegistry.handlers[industryType]
	if !exists {
		return nil
	}
	return handler
}

// GetAllHandlers returns all registered industry handlers
func GetAllHandlers() map[model.IndustryType]IndustryHandler {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[model.IndustryType]IndustryHandler, len(globalRegistry.handlers))
	for k, v := range globalRegistry.handlers {
		result[k] = v
	}
	return result
}

// HasHandler checks if a handler is registered for the given industry type
func HasHandler(industryType model.IndustryType) bool {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	_, exists := globalRegistry.handlers[industryType]
	return exists
}

