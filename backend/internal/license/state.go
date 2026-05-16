// Package license — encrypted local activation state.
//
// What's stored:
//   - The machine_id the license was activated on (set once, then
//     compared against the live fingerprint on every boot).
//   - An activation keypair: at first-activation we generate an
//     Ed25519 keypair, persist the PRIVATE half encrypted on disk,
//     and include the PUBLIC half in any MigrationFile we issue.
//     When the customer later activates on a new machine, the new
//     host re-verifies the migration file using the public key
//     contained inside the migration file itself (yes, it's a
//     trust-on-first-use within the migration chain).
//   - A set of used nonces — to refuse replay of migration files.
//   - The vendor's license payload bytes (for offline diagnostics
//     even when the active filesystem license file is gone).
//
// Storage format:  AES-256-GCM with a key derived from
//     SHA256("ai-cfo-license/v1" || machine_fingerprint).
// The fingerprint is itself derived from the host, so the encrypted
// file CANNOT be copied to another machine and decrypted there —
// which is exactly the property we want.
//
// SECURITY HONESTY: This is defense against the casual attacker who
// opens the file in a text editor. A determined attacker with shell
// access on the host can re-derive the key from a static prefix +
// machine_id. The REAL anti-tamper is the Ed25519 signature on the
// license.lic itself, which is unforgeable without the vendor's
// private key.
package license

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is the customer-side persisted activation record.
type State struct {
	// CustomerID copied from the active license. Used for sanity
	// checks on import.
	CustomerID string `json:"customer_id"`

	// MachineID the license is bound to. Equal to MachineID() when
	// the local fingerprint matches. Different machine = bad bind.
	MachineID string `json:"machine_id"`

	// ActivatedAt is when we first bound this license here.
	ActivatedAt time.Time `json:"activated_at"`

	// LastValidatedAt records every successful startup verification.
	LastValidatedAt time.Time `json:"last_validated_at"`

	// LicensePayloadBytes is the raw, base64'd canonical payload from
	// the license.lic that was activated. Lets us produce migration
	// digests / status output even if license.lic has been moved.
	LicensePayloadB64 string `json:"license_payload_b64"`

	// ActivationPubKeyB64 is the public half of the activation keypair
	// generated when this license was first activated. Included in any
	// MigrationFile we emit; lets the destination machine verify the
	// migration without contacting the vendor.
	ActivationPubKeyB64 string `json:"activation_pub_key_b64"`

	// ActivationPrivKeyB64 is the matching private half. Kept ONLY
	// inside the encrypted state file. Used to sign MigrationFiles
	// when the user runs `cfo-license deactivate`.
	ActivationPrivKeyB64 string `json:"activation_priv_key_b64"`

	// UsedNonces records every migration nonce we have ever accepted
	// as inbound — refused on second use to prevent replay. We also
	// add outbound nonces here so a single state file can't double-
	// issue a migration with the same nonce.
	UsedNonces []string `json:"used_nonces"`

	// Deactivated flips to true after `cfo-license deactivate` runs;
	// the backend startup will refuse to start while deactivated until
	// `cfo-license activate` re-binds.
	Deactivated bool `json:"deactivated"`

	// Auth is the persisted single-password + server-secret blob used
	// by the auth package. Stored here so the license state file is
	// the ONE encrypted secrets vault on disk (no scattered cred files).
	// Empty PasswordHash means "first-run setup pending".
	Auth AuthBlob `json:"auth,omitempty"`
}

// AuthBlob mirrors auth.AuthBlob. Duplicated here to avoid a package
// import cycle (auth imports license? license imports auth?). Both
// packages instead share the same shape; the auth Store adapter
// in api/auth_handler.go is the bridge.
type AuthBlob struct {
	PasswordHash    string `json:"password_hash"`
	ServerSecretB64 string `json:"server_secret_b64"`
	CreatedAt       string `json:"created_at"`
}

// stateStoreMu guards Load/Save to the same file. Each store instance
// keeps its own mutex; calls from inside the backend are funneled
// through Load/Save which take the package-level lock.
var stateStoreMu sync.Mutex

// LoadState reads and decrypts the state file at path. Returns a
// fresh, empty State if the file does not exist (first run).
func LoadState(path string) (*State, error) {
	stateStoreMu.Lock()
	defer stateStoreMu.Unlock()

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &State{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("state read: %w", err)
	}
	plain, err := decryptStateBlob(raw)
	if err != nil {
		return nil, fmt.Errorf("state decrypt: %w (was the file tampered with or copied from a different machine?)", err)
	}
	var s State
	if err := json.Unmarshal(plain, &s); err != nil {
		return nil, fmt.Errorf("state unmarshal: %w", err)
	}
	return &s, nil
}

// SaveState marshals + encrypts + writes the state to disk atomically.
func SaveState(path string, s *State) error {
	stateStoreMu.Lock()
	defer stateStoreMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("state mkdir: %w", err)
	}
	plain, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("state marshal: %w", err)
	}
	enc, err := encryptStateBlob(plain)
	if err != nil {
		return fmt.Errorf("state encrypt: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, enc, 0o600); err != nil {
		return fmt.Errorf("state write: %w", err)
	}
	return os.Rename(tmp, path)
}

// NonceUsed reports whether a migration nonce has been seen before.
func (s *State) NonceUsed(nonce string) bool {
	for _, n := range s.UsedNonces {
		if n == nonce {
			return true
		}
	}
	return false
}

// AddNonce appends a nonce. Caller should SaveState afterwards.
func (s *State) AddNonce(nonce string) {
	if !s.NonceUsed(nonce) {
		s.UsedNonces = append(s.UsedNonces, nonce)
	}
}

// ActivationPrivateKey returns the deserialized ed25519 private key
// (or error if the state was never activated).
func (s *State) ActivationPrivateKey() (ed25519.PrivateKey, error) {
	if s.ActivationPrivKeyB64 == "" {
		return nil, New(ReasonUnknown, "state has no activation keypair (license never activated)")
	}
	raw, err := base64.StdEncoding.DecodeString(s.ActivationPrivKeyB64)
	if err != nil {
		return nil, fmt.Errorf("activation priv key: bad base64: %w", err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("activation priv key: bad length %d", len(raw))
	}
	return ed25519.PrivateKey(raw), nil
}

// ActivationPublicKey returns the deserialized ed25519 public key.
func (s *State) ActivationPublicKey() (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(s.ActivationPubKeyB64)
	if err != nil {
		return nil, fmt.Errorf("activation pub key: bad base64: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("activation pub key: bad length %d", len(raw))
	}
	return ed25519.PublicKey(raw), nil
}

// GenerateActivationKeypair creates and stores a fresh keypair in
// the state. Idempotent — if a keypair already exists, it's left
// alone (so deactivate's signing key matches what's in the wild).
func (s *State) GenerateActivationKeypair() error {
	if s.ActivationPubKeyB64 != "" && s.ActivationPrivKeyB64 != "" {
		return nil
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("activation keygen: %w", err)
	}
	s.ActivationPubKeyB64 = base64.StdEncoding.EncodeToString(pub)
	s.ActivationPrivKeyB64 = base64.StdEncoding.EncodeToString(priv)
	return nil
}

// ---------------------------------------------------------------------
// AES-256-GCM helpers
// ---------------------------------------------------------------------

// stateKey derives the encryption key from a static prefix + the
// machine fingerprint. The fingerprint is read live, so an attacker
// who copies the encrypted state file to another machine cannot
// decrypt it there (their MachineID() differs).
func stateKey() ([]byte, error) {
	mid, err := MachineID()
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write([]byte("ai-cfo-license/v1\x00"))
	h.Write([]byte(mid))
	return h.Sum(nil), nil
}

func encryptStateBlob(plain []byte) ([]byte, error) {
	key, err := stateKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return g.Seal(nonce, nonce, plain, []byte("ai-cfo-state")), nil
}

func decryptStateBlob(blob []byte) ([]byte, error) {
	key, err := stateKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < g.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := blob[:g.NonceSize()], blob[g.NonceSize():]
	return g.Open(nil, nonce, ct, []byte("ai-cfo-state"))
}
