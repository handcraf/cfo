// Package api — HTTP endpoints + middleware for the license-and-login
// gate that protects every other route.
//
// Endpoints exposed here:
//
//	GET  /auth/status     — { authenticated, needs_setup }
//	POST /auth/setup      — { password }   first-time only
//	POST /auth/login      — { password }   → sets cookie
//	POST /auth/logout     — clears cookie
//	GET  /license/status  — full VerifyResult, safe to surface to UI
//
// Middleware:
//
//	LicenseGate  — blocks every /api/* route except the four /auth/*
//	               endpoints and /license/status when license is invalid
//	AuthGate     — blocks every /api/* route when not logged in
package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cfo/backend/internal/auth"
	"github.com/cfo/backend/internal/license"
)

// LicenseAuthHandler holds shared state for the gate endpoints. The
// underlying license verifier and auth service are constructed in
// main.go and injected.
type LicenseAuthHandler struct {
	Verifier *license.Verifier
	Auth     *auth.Service
	State    *license.State

	mu sync.RWMutex
	// Cached last verify result so we don't re-run the pipeline on
	// every request. Refreshed every minute.
	last        license.VerifyResult
	lastChecked time.Time
}

// NewLicenseAuthHandler wires the gate. The caller must have already
// performed an initial Verify and persisted the State.
func NewLicenseAuthHandler(v *license.Verifier, a *auth.Service, initial license.VerifyResult, st *license.State) *LicenseAuthHandler {
	return &LicenseAuthHandler{
		Verifier:    v,
		Auth:        a,
		State:       st,
		last:        initial,
		lastChecked: time.Now(),
	}
}

// CurrentStatus returns the cached verify result, refreshing if older
// than ttl.
func (h *LicenseAuthHandler) CurrentStatus() license.VerifyResult {
	const ttl = time.Minute
	h.mu.RLock()
	r, t := h.last, h.lastChecked
	h.mu.RUnlock()
	if time.Since(t) < ttl {
		return r
	}
	r = h.Verifier.Verify(license.VerifyOptions{})
	h.mu.Lock()
	h.last = r
	h.lastChecked = time.Now()
	h.mu.Unlock()
	return r
}

// ---------------------------------------------------------------------
// HTTP handlers
// ---------------------------------------------------------------------

// LicenseStatus returns the full VerifyResult to the UI. Safe even
// when the license is invalid — that's the whole point.
func (h *LicenseAuthHandler) LicenseStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, h.CurrentStatus())
}

// AuthStatus reports whether a session cookie is valid and whether
// first-run setup is still pending.
func (h *LicenseAuthHandler) AuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := map[string]any{
		"authenticated": h.Auth.IsAuthenticated(r),
		"needs_setup":   h.Auth.NeedsSetup(),
	}
	writeJSON(w, resp)
}

// AuthSetup performs first-time password setup.
func (h *LicenseAuthHandler) AuthSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.Auth.SetupPassword(req.Password); err != nil {
		writeError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.Auth.IssueSession(w); err != nil {
		writeError(w, "session issue: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// AuthLogin verifies a password and issues a session cookie.
func (h *LicenseAuthHandler) AuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.Auth.NeedsSetup() {
		writeError(w, "password not set — call /auth/setup first", http.StatusConflict)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if !h.Auth.VerifyPassword(req.Password) {
		writeError(w, "wrong password", http.StatusUnauthorized)
		return
	}
	if err := h.Auth.IssueSession(w); err != nil {
		writeError(w, "session issue: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// AuthLogout clears the session cookie.
func (h *LicenseAuthHandler) AuthLogout(w http.ResponseWriter, r *http.Request) {
	h.Auth.ClearSession(w)
	writeJSON(w, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------------

// Always-allowed prefixes — health checks, the gate endpoints
// themselves, and static frontend assets.
var alwaysAllowed = []string{
	"/health",
	"/auth/status",
	"/auth/setup",
	"/auth/login",
	"/auth/logout",
	"/license/status",
}

func isAlwaysAllowed(path string) bool {
	for _, p := range alwaysAllowed {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// LicenseGate blocks every protected route when the license is invalid.
// Returns 503 with a structured error body. The frontend reads the
// body's `reason` and `message` to render the LicenseError screen.
func (h *LicenseAuthHandler) LicenseGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAlwaysAllowed(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		st := h.CurrentStatus()
		if !st.OK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":   "license_invalid",
				"reason":  st.Reason,
				"message": st.Message,
				// Hint the UI on what to show next.
				"action": licenseAction(st.Reason),
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// AuthGate blocks every protected route when not authenticated. Runs
// AFTER LicenseGate (license problems take precedence).
func (h *LicenseAuthHandler) AuthGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAlwaysAllowed(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !h.Auth.IsAuthenticated(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":  "not_authenticated",
				"action": "login_required",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// licenseAction maps the verifier's Reason to a UI hint.
func licenseAction(r license.Reason) string {
	switch r {
	case license.ReasonFileMissing:
		return "request_license"
	case license.ReasonExpired:
		return "renew_license"
	case license.ReasonMachineMismatch:
		return "reactivate"
	case license.ReasonBadSignature:
		return "contact_vendor"
	default:
		return "contact_vendor"
	}
}
