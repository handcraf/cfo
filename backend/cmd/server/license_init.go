package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cfo/backend/internal/auth"
	"github.com/cfo/backend/internal/license"
)

// licenseStatePath resolves where the encrypted state file lives.
// Defaults to <DATA_DIR>/state/license.state.enc.
func licenseStatePath(dataDir string) string {
	if p := os.Getenv("LICENSE_STATE"); p != "" {
		return p
	}
	return filepath.Join(dataDir, "state", "license.state.enc")
}

// licenseFilePath resolves the path to license.lic.
// Defaults to ./license.lic (sibling of run.sh).
func licenseFilePath() string {
	if p := os.Getenv("LICENSE_FILE"); p != "" {
		return p
	}
	return "license.lic"
}

// licenseStateStore is the Store adapter the auth package needs.
// It re-loads / re-saves the encrypted state on every call so concurrent
// writes from the license CLI don't get clobbered.
type licenseStateStore struct {
	path string
}

func (s *licenseStateStore) load() *license.State {
	st, err := license.LoadState(s.path)
	if err != nil {
		log.Printf("[auth-store] load state: %v (using empty state)", err)
		return &license.State{}
	}
	return st
}

func (s *licenseStateStore) GetAuth() auth.AuthBlob {
	st := s.load()
	return auth.AuthBlob{
		PasswordHash:    st.Auth.PasswordHash,
		ServerSecretB64: st.Auth.ServerSecretB64,
		CreatedAt:       st.Auth.CreatedAt,
	}
}

func (s *licenseStateStore) SetAuth(b auth.AuthBlob) error {
	st := s.load()
	st.Auth = license.AuthBlob{
		PasswordHash:    b.PasswordHash,
		ServerSecretB64: b.ServerSecretB64,
		CreatedAt:       b.CreatedAt,
	}
	return license.SaveState(s.path, st)
}

// licenseStartup runs the full license verification at process boot.
// Returns (verifyResult, verifier, authService, fatal). If fatal is
// non-nil, the caller MUST refuse to expose business APIs (we still
// return a usable verifyResult so the gate endpoint shows the reason
// to the UI).
func licenseStartup(dataDir string) (license.VerifyResult, *license.Verifier, *auth.Service, *license.State) {
	licPath := licenseFilePath()
	statePath := licenseStatePath(dataDir)

	v := license.NewVerifier(licPath, statePath)
	r := v.Verify(license.VerifyOptions{})

	if r.OK {
		log.Printf("[license] OK: customer=%s type=%s expires=%s (%d days remaining)",
			r.Payload.CustomerName, r.Payload.LicenseType,
			r.Payload.Expiry.Format("2006-01-02"), r.DaysRemaining)
		if r.WarningDays > 0 {
			log.Printf("[license] ⚠ WARNING: license expires in %d days — run `cfo-license export-request`", r.WarningDays)
		}
	} else {
		log.Printf("[license] INVALID: reason=%s message=%s", r.Reason, r.Message)
		log.Printf("[license] business APIs will be disabled until the license is fixed")
		log.Printf("[license] only these endpoints will respond: /health, /license/status, /auth/*")
	}

	// Build the auth service. It can load even when the license is
	// invalid — it just means the user won't get past the LicenseError
	// page anyway.
	store := &licenseStateStore{path: statePath}
	authSvc := auth.NewService(store)

	// Reload state (Verify may have updated it on a successful path).
	state, err := license.LoadState(statePath)
	if err != nil {
		log.Printf("[license] state reload: %v", err)
		state = &license.State{}
	}
	return r, v, authSvc, state
}

// emergencyHandler is the last-resort http.Handler used when the public
// key is fundamentally unresolvable (no embed, no env). It serves a
// fixed JSON error on every route so an operator can hit /health and
// see what's wrong.
func emergencyHandler(msg string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"error":"license_misconfigured","message":%q}`, msg)
	})
}
