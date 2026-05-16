// Package license — Ed25519-signed offline license file (license.lic).
//
// Contract:
//
//	license.lic = {
//	  "license":   base64( canonical_json(payload) ),
//	  "signature": base64( Ed25519.sign(private_key, canonical_json(payload)) )
//	}
//
// "canonical_json" means: payload struct → marshal with sorted keys, no
// extra whitespace, no HTML escaping. The same canonical bytes are signed
// AND verified, so JSON whitespace, key order, or pretty-printing cannot
// affect signature validity. Tampering with even one byte in the payload
// breaks verification.
//
// Public key is embedded in the application binary (see pubkey.go).
// Private key NEVER ships with the application — it lives only with
// the vendor's license-gen tool.
package license

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// Type is the license tier.
type Type string

const (
	TypeTrial      Type = "Trial"
	TypeStartup    Type = "Startup"
	TypeEnterprise Type = "Enterprise"
	TypeUnlimited  Type = "Unlimited"
)

// ValidTypes is the closed set of acceptable license types.
func ValidTypes() []Type {
	return []Type{TypeTrial, TypeStartup, TypeEnterprise, TypeUnlimited}
}

// Feature is a named capability gated by the license. Keep these short
// PascalCase identifiers; the backend uses string-equality checks.
type Feature string

const (
	FeatureAICFO          Feature = "AI_CFO"
	FeatureForecasting    Feature = "Forecasting"
	FeatureFinancialRpts  Feature = "FinancialReports"
	FeatureAuditAssistant Feature = "AuditAssistant"
)

// Payload is the canonical license body. The vendor signs THIS, the
// customer's runtime verifies THIS. The MachineID field is added at
// activation time on the customer side (it's optional in a fresh,
// unactivated license).
type Payload struct {
	CustomerName string    `json:"customer_name"`
	CustomerID   string    `json:"customer_id"`
	IssuedAt     time.Time `json:"issued_at"`
	Expiry       time.Time `json:"expiry"`
	Features     []Feature `json:"features"`
	MaxUsers     int       `json:"max_users"`
	LicenseType  Type      `json:"license_type"`
	// MachineID, when present, binds this license to a specific machine.
	// Empty means "unbound — will bind on first activation".
	MachineID string `json:"machine_id,omitempty"`
	// Nonce is a random 128-bit value to prevent two licenses with
	// otherwise identical content from being byte-for-byte identical.
	Nonce string `json:"nonce"`
}

// File is the on-disk license.lic format.
type File struct {
	License   string `json:"license"`   // base64(canonical_json(Payload))
	Signature string `json:"signature"` // base64(Ed25519.Sign(private_key, canonical_bytes))
}

// CanonicalBytes returns the deterministic, signable byte representation
// of a payload: JSON with sorted keys, no extra whitespace, no HTML
// escaping. Two payloads with the same values always produce identical
// bytes — this is what makes the Ed25519 signature stable.
func (p Payload) CanonicalBytes() ([]byte, error) {
	return canonicalJSON(p)
}

// Encode wraps a payload + signature into a File ready to write to disk.
func Encode(payload Payload, signature []byte) (File, error) {
	body, err := payload.CanonicalBytes()
	if err != nil {
		return File{}, fmt.Errorf("encode: canonical: %w", err)
	}
	return File{
		License:   base64.StdEncoding.EncodeToString(body),
		Signature: base64.StdEncoding.EncodeToString(signature),
	}, nil
}

// Decode parses a File into payload bytes + signature bytes. It does
// NOT verify — that's verifier.go's job. Use this when you need the
// payload but verification will run separately (e.g. for diagnostic
// commands like `status` that should work even on expired licenses).
func (f File) Decode() (payload Payload, payloadBytes, signature []byte, err error) {
	payloadBytes, err = base64.StdEncoding.DecodeString(f.License)
	if err != nil {
		return Payload{}, nil, nil, fmt.Errorf("decode: bad license base64: %w", err)
	}
	signature, err = base64.StdEncoding.DecodeString(f.Signature)
	if err != nil {
		return Payload{}, nil, nil, fmt.Errorf("decode: bad signature base64: %w", err)
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return Payload{}, nil, nil, fmt.Errorf("decode: bad payload json: %w", err)
	}
	return payload, payloadBytes, signature, nil
}

// HasFeature reports whether the license grants the named feature.
// Empty receiver returns false (defensive).
func (p Payload) HasFeature(f Feature) bool {
	for _, x := range p.Features {
		if x == f {
			return true
		}
	}
	return false
}

// IsExpired reports whether the license's expiry is strictly before now.
func (p Payload) IsExpired(now time.Time) bool {
	return now.After(p.Expiry)
}

// DaysRemaining is signed integer days until expiry. Negative means
// the license is already expired.
func (p Payload) DaysRemaining(now time.Time) int {
	d := p.Expiry.Sub(now)
	return int(d.Hours() / 24)
}

// MigrationFile is the structure of a signed deactivation token. The
// customer generates it with `cfo-license deactivate` on the OLD
// machine; they hand-carry it to the new machine and run
// `cfo-license activate migration.dat`. Replay protection is via the
// Nonce — activated nonces are persisted in encrypted local state
// and refused on second use.
type MigrationFile struct {
	CustomerID    string    `json:"customer_id"`
	OldMachineID  string    `json:"old_machine"`
	Timestamp     time.Time `json:"timestamp"`
	Nonce         string    `json:"nonce"`
	LicenseDigest string    `json:"license_digest"` // sha256 of the OLD license payload bytes
	Signature     string    `json:"signature"`      // base64 ed25519 over canonical body
}

// MigrationBody returns the canonical bytes that get signed inside
// a MigrationFile. Used by both signer (license-gen / cfo-license
// deactivate path) and verifier.
type migrationBody struct {
	CustomerID    string    `json:"customer_id"`
	OldMachineID  string    `json:"old_machine"`
	Timestamp     time.Time `json:"timestamp"`
	Nonce         string    `json:"nonce"`
	LicenseDigest string    `json:"license_digest"`
}

// CanonicalBytes for a migration file = same canonical rules as Payload.
func (m MigrationFile) CanonicalBytes() ([]byte, error) {
	return canonicalJSON(migrationBody{
		CustomerID:    m.CustomerID,
		OldMachineID:  m.OldMachineID,
		Timestamp:     m.Timestamp,
		Nonce:         m.Nonce,
		LicenseDigest: m.LicenseDigest,
	})
}

// RenewalRequest is what the customer sends to the vendor when their
// license is nearing expiry. It is NOT signed — the vendor uses it as
// hints only and issues a fresh, freshly-signed license.lic in return.
type RenewalRequest struct {
	CustomerID    string    `json:"customer_id"`
	CustomerName  string    `json:"customer_name"`
	MachineID     string    `json:"machine"`
	CurrentExpiry time.Time `json:"current_expiry"`
	GeneratedAt   time.Time `json:"generated_at"`
}

// ---------------------------------------------------------------------
// canonical JSON helpers
// ---------------------------------------------------------------------

// canonicalJSON produces a deterministic byte sequence: keys sorted at
// every nesting level, no whitespace, no HTML escaping (`&` / `<` / `>`
// stay literal), time.Time as RFC3339Nano via the default time marshal.
// Two values that are deeply-equal always produce identical bytes —
// that's the invariant the Ed25519 signature depends on.
func canonicalJSON(v any) ([]byte, error) {
	// First pass: through encoding/json to get the value as a generic
	// tree (map[string]any / []any / primitives). This handles all the
	// struct-tag / time.Time / json.Marshaler machinery for free.
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return emitCanonical(generic)
}

// emitCanonical recursively writes a canonical byte representation:
//   - objects: keys sorted, no spaces, recursive
//   - arrays:  preserve order, no spaces, recursive
//   - primitives: json.Encoder with SetEscapeHTML(false), trailing newline stripped
func emitCanonical(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var buf bytes.Buffer
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			kb, err := emitPrimitive(k)
			if err != nil {
				return nil, err
			}
			buf.Write(kb)
			buf.WriteByte(':')
			vb, err := emitCanonical(t[k])
			if err != nil {
				return nil, err
			}
			buf.Write(vb)
		}
		buf.WriteByte('}')
		return buf.Bytes(), nil
	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			vb, err := emitCanonical(e)
			if err != nil {
				return nil, err
			}
			buf.Write(vb)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	default:
		return emitPrimitive(t)
	}
}

func emitPrimitive(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
