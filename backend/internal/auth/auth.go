// Package auth — single-password login + cookie-session for the
// single-tenant, single-machine AI CFO deployment.
//
// Design (matching the user's spec: "just match password, no users"):
//   - One password per install. Stored as a bcrypt hash inside the
//     encrypted license state (so a stolen state file alone doesn't
//     reveal the password and a stolen state file from another host
//     won't decrypt at all).
//   - First-time setup: if no password is set, /auth/status reports
//     needs_setup=true, and POST /auth/setup accepts the chosen
//     password (any length ≥ 6).
//   - Login: POST /auth/login with {"password":...} → 200 + Set-Cookie
//     (HTTP-only, SameSite=Lax). Wrong password → 401, with a small
//     constant-time delay (~250 ms) to discourage online brute force.
//   - Session token: HMAC-SHA256( server_secret , user_id || exp )
//     stored in cookie. Validated on every protected request.
//   - Logout: clear cookie.
//
// Server-secret comes from the encrypted state (auto-generated on first
// boot). That means restarting the server invalidates all existing
// cookies if the state was wiped — good behavior on a single-tenant box.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cookieName     = "cfo_session"
	sessionTTL     = 24 * time.Hour
	minPasswordLen = 6
	loginDelay     = 250 * time.Millisecond // throttle wrong-password
)

// State is the persisted auth blob. The state.go in the license package
// is the canonical home for ALL on-disk secrets so we don't sprinkle
// encrypted files around the project; auth piggybacks on it via the
// AuthBlob field in license.State. To keep the packages decoupled, this
// file accepts a Store interface.
type Store interface {
	GetAuth() AuthBlob
	SetAuth(AuthBlob) error
}

// AuthBlob is what we persist. Empty PasswordHash means "not yet set
// up". ServerSecret is generated on first boot and used to sign session
// cookies.
type AuthBlob struct {
	PasswordHash    string `json:"password_hash"`
	ServerSecretB64 string `json:"server_secret_b64"`
	CreatedAt       string `json:"created_at"`
}

// Service owns the password + session logic. Construct with NewService.
type Service struct {
	store Store
}

// NewService wires an auth service to its persistent store.
func NewService(s Store) *Service { return &Service{store: s} }

// NeedsSetup reports whether the first-run password setup is pending.
func (s *Service) NeedsSetup() bool {
	return s.store.GetAuth().PasswordHash == ""
}

// SetupPassword establishes the password on first run. Refuses if a
// password is already set (callers should require explicit "reset"
// flow for that).
func (s *Service) SetupPassword(pw string) error {
	if len(pw) < minPasswordLen {
		return fmt.Errorf("password must be at least %d characters", minPasswordLen)
	}
	blob := s.store.GetAuth()
	if blob.PasswordHash != "" {
		return errors.New("password already set — reset is not supported in v1")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash: %w", err)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("server secret: %w", err)
	}
	blob.PasswordHash = string(hash)
	blob.ServerSecretB64 = base64.StdEncoding.EncodeToString(secret)
	blob.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	return s.store.SetAuth(blob)
}

// VerifyPassword compares an attempt against the stored hash. Always
// sleeps for `loginDelay` first (so timing tells the attacker nothing).
func (s *Service) VerifyPassword(pw string) bool {
	time.Sleep(loginDelay)
	blob := s.store.GetAuth()
	if blob.PasswordHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(blob.PasswordHash), []byte(pw)) == nil
}

// IssueSession mints a session token (HMAC-signed, with embedded expiry)
// and writes it as an HTTP-only cookie.
func (s *Service) IssueSession(w http.ResponseWriter) error {
	blob := s.store.GetAuth()
	secret, err := base64.StdEncoding.DecodeString(blob.ServerSecretB64)
	if err != nil {
		return fmt.Errorf("server secret: %w", err)
	}
	exp := time.Now().Add(sessionTTL).Unix()
	token := signToken(secret, exp)
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(sessionTTL),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// Secure should be true when serving over HTTPS. In dev (http
		// on localhost) we leave it false so the cookie is accepted.
		// TODO: drive this from cfg.SecureCookies when TLS lands.
	})
	return nil
}

// ClearSession nukes the session cookie.
func (s *Service) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// IsAuthenticated reports whether the request carries a valid session
// cookie. Returns false (without error) when no cookie, malformed
// cookie, expired, or bad HMAC.
func (s *Service) IsAuthenticated(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return false
	}
	blob := s.store.GetAuth()
	secret, err := base64.StdEncoding.DecodeString(blob.ServerSecretB64)
	if err != nil {
		return false
	}
	return verifyToken(secret, c.Value)
}

// ---------------------------------------------------------------------
// Token format:  base64( exp_unix.le64 ) "." base64( hmac_sha256(secret, exp_unix.le64) )
// ---------------------------------------------------------------------

func signToken(secret []byte, exp int64) string {
	expBytes := []byte(fmt.Sprintf("%d", exp))
	mac := hmac.New(sha256.New, secret)
	mac.Write(expBytes)
	tag := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(expBytes) + "." +
		hex.EncodeToString(tag)
}

func verifyToken(secret []byte, token string) bool {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return false
	}
	expBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	tagGot, err := hex.DecodeString(parts[1])
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write(expBytes)
	tagWant := mac.Sum(nil)
	if !hmac.Equal(tagGot, tagWant) {
		return false
	}
	// Parse exp and check expiry.
	var exp int64
	_, err = fmt.Sscanf(string(expBytes), "%d", &exp)
	if err != nil {
		return false
	}
	return time.Now().Unix() < exp
}
