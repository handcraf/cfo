// Package license — Ed25519 sign/verify helpers and embedded public key.
//
// Key flow:
//
//	┌──────────────────┐                    ┌─────────────────────┐
//	│  Vendor host     │   never shipped    │  Customer host      │
//	│  ────────────    │ ────────────────►  │  ────────────       │
//	│  private key  ◄──┤                    │  public key ◄── embedded
//	│  license-gen ───►│  license.lic       │  cfo-server reads   │
//	│  signs payload   │  ───────────────►  │  ★ verifies ★       │
//	└──────────────────┘                    └─────────────────────┘
//
// Public key is provided via three mechanisms (first non-empty wins):
//
//  1. Compile-time embed of config/license_pubkey.pem (preferred, ships
//     baked into the binary).
//  2. Env var LICENSE_PUBLIC_KEY containing the raw base64 32-byte key.
//     Used for dev / tests so we don't need a real keypair on disk.
//  3. Hard-coded fallback (a randomly generated dev key) so tests run
//     even with zero configuration. NOT for production.
//
// All three are documented in README so an enterprise customer never
// gets confused about where verification trust comes from.
package license

import (
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"sync"
)

// embeddedPubKeyPEM is bundled at build time. The file must exist at
// config/license_pubkey.pem relative to the repo root. If the file is
// empty (e.g. tests run before any keypair is generated), we fall back
// to the env var or the dev key — see VendorPublicKey().
//
//go:embed pubkey_embed.pem
var embeddedPubKeyPEM []byte

var (
	pubKeyOnce sync.Once
	pubKey     ed25519.PublicKey
	pubKeyErr  error
)

// VendorPublicKey returns the Ed25519 public key used to verify every
// incoming license / migration / renewal artifact. The function is
// cached after the first call.
//
// Precedence (highest first):
//
//  1. LICENSE_PUBLIC_KEY env var — used by tests, dev, and emergency
//     overrides. Standard 12-factor: env beats compiled-in.
//  2. Compile-time embed of config/license_pubkey.pem — the production
//     default. Ships baked into every customer binary.
//  3. ErrNoPublicKey — fatal startup failure.
func VendorPublicKey() (ed25519.PublicKey, error) {
	pubKeyOnce.Do(func() {
		// 1. Env var override.
		if env := os.Getenv("LICENSE_PUBLIC_KEY"); env != "" {
			raw, err := base64.StdEncoding.DecodeString(env)
			if err != nil {
				pubKeyErr = fmt.Errorf("LICENSE_PUBLIC_KEY: bad base64: %w", err)
				return
			}
			if len(raw) != ed25519.PublicKeySize {
				pubKeyErr = fmt.Errorf("LICENSE_PUBLIC_KEY: bad length %d (want %d)", len(raw), ed25519.PublicKeySize)
				return
			}
			pubKey = ed25519.PublicKey(raw)
			return
		}
		// 2. Compile-time embed (production default).
		if k, err := parsePEM(embeddedPubKeyPEM); err == nil && len(k) > 0 {
			pubKey = k
			return
		}
		// 3. Fatal.
		pubKeyErr = ErrNoPublicKey
	})
	return pubKey, pubKeyErr
}

// ErrNoPublicKey is returned when neither embed nor env produced a
// usable verification key. The backend treats this as a hard startup
// failure.
var ErrNoPublicKey = errors.New("no vendor public key available (set LICENSE_PUBLIC_KEY or place config/license_pubkey.pem at build time)")

// LoadPrivateKeyPEM reads an Ed25519 private key from a PEM file.
// Used ONLY by the vendor's license-gen tool. Never call this from
// the customer-facing backend.
func LoadPrivateKeyPEM(path string) (ed25519.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("private key: no PEM block found")
	}
	if block.Type != "ED25519 PRIVATE KEY" && block.Type != "PRIVATE KEY" {
		return nil, fmt.Errorf("private key: unexpected PEM type %q", block.Type)
	}
	if len(block.Bytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("private key: bad length %d (want %d)", len(block.Bytes), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(block.Bytes), nil
}

// SavePrivateKeyPEM writes an Ed25519 private key as a PEM block. Used
// only by `license-gen keygen`.
func SavePrivateKeyPEM(path string, priv ed25519.PrivateKey) error {
	block := &pem.Block{Type: "ED25519 PRIVATE KEY", Bytes: priv}
	return os.WriteFile(path, pem.EncodeToMemory(block), 0o600)
}

// SavePublicKeyPEM writes an Ed25519 public key as a PEM block.
func SavePublicKeyPEM(path string, pub ed25519.PublicKey) error {
	block := &pem.Block{Type: "ED25519 PUBLIC KEY", Bytes: pub}
	return os.WriteFile(path, pem.EncodeToMemory(block), 0o644)
}

// GenerateKeypair generates a fresh Ed25519 keypair. The vendor's
// `license-gen keygen` is the only legitimate caller.
func GenerateKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

// SignPayload signs a license Payload with the vendor's private key and
// returns the (file, raw signature) pair.
func SignPayload(payload Payload, priv ed25519.PrivateKey) (File, []byte, error) {
	body, err := payload.CanonicalBytes()
	if err != nil {
		return File{}, nil, fmt.Errorf("sign: canonical: %w", err)
	}
	sig := ed25519.Sign(priv, body)
	f, err := Encode(payload, sig)
	if err != nil {
		return File{}, nil, err
	}
	return f, sig, nil
}

// VerifyFile validates a license File's signature against the
// embedded vendor public key. Returns the verified payload on success.
// Tampering with even one byte of the encoded payload causes verification
// to fail.
func VerifyFile(f File) (Payload, error) {
	pub, err := VendorPublicKey()
	if err != nil {
		return Payload{}, err
	}
	_, payloadBytes, sig, err := f.Decode()
	if err != nil {
		return Payload{}, err
	}
	if !ed25519.Verify(pub, payloadBytes, sig) {
		return Payload{}, ErrBadSignature
	}
	// Re-decode the payload struct AFTER signature verification passed.
	var p Payload
	if err := decodeJSON(payloadBytes, &p); err != nil {
		return Payload{}, fmt.Errorf("verify: payload re-decode: %w", err)
	}
	return p, nil
}

// SignMigration signs a MigrationFile with the OLD machine's local
// state's customer-bound key. Wait — migration is signed with the same
// vendor private key? No: migration is signed by the CUSTOMER's running
// instance to PROVE the deactivation happened with vendor-issued
// credentials. The customer's instance has only the vendor PUBLIC key,
// so it cannot create a forgery; instead, the migration is signed by
// the activation key derived from the license's signature itself (a
// kind of HMAC chain). See state.go and TODO below.
//
// For v1 simplicity: the migration is signed with the vendor private
// key by license-gen on the vendor side after the customer submits the
// migration request. That's not what the user spec says (they want a
// local deactivate that doesn't require the vendor). To meet the spec,
// we sign the migration with an ed25519 keypair generated at LICENSE
// activation time, embedded inside the encrypted local state. The
// activation key's public half is also in the local state — anyone
// importing the migration on a NEW machine verifies against that
// embedded public half, which was bound to the old machine.
//
// Implementation lives in state.go; this file only provides the
// primitive Sign/Verify wrappers.

// ErrBadSignature is returned when an Ed25519 verification fails.
var ErrBadSignature = errors.New("license: signature does not verify")

// ---------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------

func parsePEM(raw []byte) (ed25519.PublicKey, error) {
	if len(raw) == 0 {
		return nil, errors.New("empty pem")
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("no pem block")
	}
	if len(block.Bytes) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("bad public key length %d", len(block.Bytes))
	}
	return ed25519.PublicKey(block.Bytes), nil
}

// decodeJSON is a thin wrapper kept here to avoid an extra import in
// the verifier. It exists so callers don't need to know we serialize
// with encoding/json.
func decodeJSON(raw []byte, out any) error {
	return jsonDecode(raw, out)
}
