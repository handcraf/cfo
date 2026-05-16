package api

import (
	"encoding/json"
	"net/http"

	"github.com/cfo/backend/internal/config"
	"github.com/cfo/backend/internal/model"
	"github.com/cfo/backend/internal/service"
	"github.com/cfo/backend/internal/storage"
	"github.com/cfo/backend/internal/storage/sqlstore"
)

// Deps bundles optional wire-in dependencies for SetupRoutes. Any field may
// be nil; the router wires up what it's given and degrades gracefully.
//
// This struct exists so main() owns the lifecycle of persistent resources
// (DB connections, HTTP clients) rather than the api package.
type Deps struct {
	// SQL is the SQLite source-of-truth. When non-nil, the ask handler
	// will write audit rows; other handlers may later adopt SQL reads.
	SQL *sqlstore.Store

	// VectorStore is the backend for semantic retrieval. Usually an
	// in-memory store (service.NewInMemoryStore) or a *service.QdrantStore.
	// When nil, retrieval falls back to keyword-only RAG.
	VectorStore service.Store

	// Gate carries the license verifier + auth service. When non-nil,
	// every business route is wrapped in two middlewares: LicenseGate
	// (503 on bad license) and AuthGate (401 when not logged in).
	// Health checks and the gate endpoints themselves are exempt.
	Gate *LicenseAuthHandler
}

// SetupRoutes configures all API routes with no extra wiring. Retained
// for tests and backward-compatible main() callers.
func SetupRoutes(cfg *config.Config) http.Handler {
	return SetupRoutesWithDeps(cfg, Deps{})
}

// SetupRoutesWithDeps is the production entry point: main() builds the
// sqlstore + vector store, passes them in here. Tests that don't need
// them keep calling SetupRoutes(cfg).
func SetupRoutesWithDeps(cfg *config.Config, deps Deps) http.Handler {
	mux := http.NewServeMux()
	store := storage.NewFileStore(cfg.DataDir)

	// Create handlers
	healthHandler := NewHealthHandler()
	companyHandler := NewCompanyHandler(store, cfg)
	documentsHandler := NewDocumentsHandler(store, cfg)
	metricsHandler := NewMetricsHandler(store)
	askHandler := NewAskHandler(store, cfg)
	if deps.VectorStore != nil {
		askHandler = askHandler.WithVectorStore(deps.VectorStore)
	}
	if sink := NewSQLiteAuditSink(deps.SQL); sink != nil {
		askHandler = askHandler.WithAudit(sink)
	}

	// Register routes
	mux.HandleFunc("/health", healthHandler.Health)
	mux.HandleFunc("/setup/company", companyHandler.SetupCompany)
	mux.HandleFunc("/company/status", companyHandler.GetStatus)
	mux.HandleFunc("/company/reset", companyHandler.ResetCompany)
	mux.HandleFunc("/documents/upload", documentsHandler.Upload)
	mux.HandleFunc("/documents", documentsHandler.List)
	mux.HandleFunc("/documents/reset", documentsHandler.ResetDocuments)
	mux.HandleFunc("/metrics/current", metricsHandler.GetCurrent)
	mux.HandleFunc("/ask", askHandler.Ask)

	// License + auth endpoints (always reachable, even when the gate
	// blocks everything else).
	if deps.Gate != nil {
		mux.HandleFunc("/license/status", deps.Gate.LicenseStatus)
		mux.HandleFunc("/auth/status", deps.Gate.AuthStatus)
		mux.HandleFunc("/auth/setup", deps.Gate.AuthSetup)
		mux.HandleFunc("/auth/login", deps.Gate.AuthLogin)
		mux.HandleFunc("/auth/logout", deps.Gate.AuthLogout)
	}

	// Compose middleware chain. Order matters:
	//   request → CORS → LicenseGate → AuthGate → mux
	// License problems take precedence over auth problems so the UI
	// can show the LicenseError page instead of a Login page when the
	// license is bad.
	var handler http.Handler = mux
	if deps.Gate != nil {
		handler = deps.Gate.AuthGate(handler)
		handler = deps.Gate.LicenseGate(handler)
	}
	return corsMiddleware(handler)
}

// corsMiddleware adds CORS headers for local development. Cookies
// (session auth) require Allow-Credentials AND a concrete (non-wildcard)
// origin, so we echo back the request Origin when it's localhost.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" || isLocalOrigin(origin) {
			if origin == "" {
				origin = "*"
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			if origin != "*" {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isLocalOrigin accepts the dev origins we trust: http://localhost:*,
// http://127.0.0.1:*, http://0.0.0.0:*. Anything else falls through to
// a wildcard origin without credentials.
func isLocalOrigin(o string) bool {
	for _, p := range []string{
		"http://localhost:", "http://127.0.0.1:", "http://0.0.0.0:",
		"https://localhost:", "https://127.0.0.1:",
	} {
		if len(o) >= len(p) && o[:len(p)] == p {
			return true
		}
	}
	return false
}

// CompanyHandler handles company setup endpoints
type CompanyHandler struct {
	store *storage.FileStore
	cfg   *config.Config
}

// NewCompanyHandler creates a new CompanyHandler
func NewCompanyHandler(store *storage.FileStore, cfg *config.Config) *CompanyHandler {
	return &CompanyHandler{store: store, cfg: cfg}
}

// SetupCompany handles POST /setup/company
func (h *CompanyHandler) SetupCompany(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var company model.Company
	if err := json.NewDecoder(r.Body).Decode(&company); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if company.Name == "" {
		writeError(w, "Company name is required", http.StatusBadRequest)
		return
	}

	company.SetupCompleted = true
	if err := h.store.SaveCompany(&company); err != nil {
		writeError(w, "Failed to save company", http.StatusInternalServerError)
		return
	}

	writeJSON(w, company)
}

// GetStatus handles GET /company/status
func (h *CompanyHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	company, err := h.store.LoadCompany()
	if err != nil {
		writeError(w, "Failed to load company status", http.StatusInternalServerError)
		return
	}

	status := model.CompanyStatus{
		SetupCompleted: company != nil && company.SetupCompleted,
		Company:        company,
	}

	writeJSON(w, status)
}

// ResetCompany handles DELETE /company/reset - resets all data and starts fresh
func (h *CompanyHandler) ResetCompany(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Reset all data using the store
	if err := h.store.ResetAllData(); err != nil {
		writeError(w, "Failed to reset company data: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"success": true,
		"message": "Company data reset successfully. Please set up a new company.",
	})
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response
func writeError(w http.ResponseWriter, message string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
