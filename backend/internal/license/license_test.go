package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testKeypair generates a fresh keypair and installs the public key
// via env var so VendorPublicKey() resolves to it for the duration of
// the test. Returns a cleanup func.
func testKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := GenerateKeypair()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	t.Setenv("LICENSE_PUBLIC_KEY", base64.StdEncoding.EncodeToString(pub))
	// Reset the cached public key so the new env value is picked up.
	resetPubKeyCache()
	return pub, priv
}

func samplePayload(now time.Time) Payload {
	return Payload{
		CustomerName: "ACME Corp",
		CustomerID:   "ACME-001",
		IssuedAt:     now,
		Expiry:       now.AddDate(1, 0, 0),
		Features:     []Feature{FeatureAICFO, FeatureForecasting},
		MaxUsers:     50,
		LicenseType:  TypeEnterprise,
		Nonce:        "test-nonce-1",
	}
}

// ---------------------------------------------------------------------
// Canonicalization determinism
// ---------------------------------------------------------------------

func TestCanonicalJSON_Deterministic(t *testing.T) {
	p := samplePayload(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	a, err := p.CanonicalBytes()
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	b, _ := p.CanonicalBytes()
	if string(a) != string(b) {
		t.Fatalf("canonical not deterministic:\nA: %s\nB: %s", a, b)
	}
}

func TestCanonicalJSON_SortedKeys(t *testing.T) {
	p := samplePayload(time.Now())
	b, _ := p.CanonicalBytes()
	s := string(b)
	// customer_id must appear before customer_name (lex order).
	idx1 := indexOf(s, `"customer_id"`)
	idx2 := indexOf(s, `"customer_name"`)
	if idx1 < 0 || idx2 < 0 || idx1 >= idx2 {
		t.Fatalf("keys not sorted: %s", s)
	}
}

func TestCanonicalJSON_NoHTMLEscaping(t *testing.T) {
	p := samplePayload(time.Now())
	p.CustomerName = "M&M <Co>"
	b, _ := p.CanonicalBytes()
	s := string(b)
	if indexOf(s, "&amp;") >= 0 || indexOf(s, `\u0026`) >= 0 {
		t.Fatalf("HTML/Unicode escaping leaked into canonical bytes: %s", s)
	}
}

// ---------------------------------------------------------------------
// Ed25519 sign/verify roundtrip
// ---------------------------------------------------------------------

func TestSignVerify_HappyPath(t *testing.T) {
	_, priv := testKeypair(t)
	p := samplePayload(time.Now())
	f, _, err := SignPayload(p, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	got, err := VerifyFile(f)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.CustomerID != p.CustomerID {
		t.Fatalf("payload roundtrip mismatch: got %q want %q", got.CustomerID, p.CustomerID)
	}
}

func TestVerify_TamperedPayloadFails(t *testing.T) {
	_, priv := testKeypair(t)
	p := samplePayload(time.Now())
	f, _, _ := SignPayload(p, priv)
	// Tamper: re-encode payload with the customer name changed but
	// keep the original signature.
	pBytes, _ := base64.StdEncoding.DecodeString(f.License)
	var raw map[string]any
	_ = json.Unmarshal(pBytes, &raw)
	raw["customer_name"] = "Evil Corp"
	tampered, _ := json.Marshal(raw)
	f.License = base64.StdEncoding.EncodeToString(tampered)

	if _, err := VerifyFile(f); err == nil {
		t.Fatal("verify should have failed on tampered payload")
	}
}

func TestVerify_TamperedSignatureFails(t *testing.T) {
	_, priv := testKeypair(t)
	p := samplePayload(time.Now())
	f, _, _ := SignPayload(p, priv)
	sigBytes, _ := base64.StdEncoding.DecodeString(f.Signature)
	sigBytes[0] ^= 0xff
	f.Signature = base64.StdEncoding.EncodeToString(sigBytes)
	if _, err := VerifyFile(f); err == nil {
		t.Fatal("verify should have failed on tampered signature")
	}
}

func TestVerify_DifferentKeyFails(t *testing.T) {
	_, priv := testKeypair(t)
	p := samplePayload(time.Now())
	f, _, _ := SignPayload(p, priv)
	// Swap in a different vendor public key.
	pub2, _, _ := GenerateKeypair()
	t.Setenv("LICENSE_PUBLIC_KEY", base64.StdEncoding.EncodeToString(pub2))
	resetPubKeyCache()
	if _, err := VerifyFile(f); err == nil {
		t.Fatal("verify should have failed with foreign public key")
	}
}

// ---------------------------------------------------------------------
// Feature + expiry
// ---------------------------------------------------------------------

func TestHasFeature(t *testing.T) {
	p := samplePayload(time.Now())
	if !p.HasFeature(FeatureAICFO) {
		t.Fatal("expected AI_CFO to be present")
	}
	if p.HasFeature(FeatureAuditAssistant) {
		t.Fatal("AuditAssistant should NOT be present")
	}
}

func TestExpiry(t *testing.T) {
	p := samplePayload(time.Now())
	if p.IsExpired(time.Now()) {
		t.Fatal("freshly issued license shouldn't be expired")
	}
	if !p.IsExpired(p.Expiry.Add(1 * time.Second)) {
		t.Fatal("license should be expired one second after expiry")
	}
}

func TestDaysRemaining(t *testing.T) {
	p := samplePayload(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	p.Expiry = time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC) // +10 days
	got := p.DaysRemaining(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if got != 10 {
		t.Fatalf("days remaining: got %d want 10", got)
	}
}

// ---------------------------------------------------------------------
// State encryption
// ---------------------------------------------------------------------

func TestState_EncryptedRoundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "license.state.enc")

	s := &State{
		CustomerID:      "ACME-001",
		MachineID:       "deadbeef",
		ActivatedAt:     time.Now().UTC().Truncate(time.Second),
		LastValidatedAt: time.Now().UTC().Truncate(time.Second),
	}
	_ = s.GenerateActivationKeypair()
	if err := SaveState(path, s); err != nil {
		t.Fatalf("save: %v", err)
	}

	// File on disk must NOT be plain-text JSON.
	raw, _ := os.ReadFile(path)
	if indexOf(string(raw), "ACME-001") >= 0 {
		t.Fatalf("state file contains plaintext customer id — encryption failed: %q", raw)
	}

	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.CustomerID != "ACME-001" {
		t.Fatalf("roundtrip mismatch: got %q", got.CustomerID)
	}
}

func TestState_NonceTracking(t *testing.T) {
	s := &State{}
	if s.NonceUsed("abc") {
		t.Fatal("fresh state should report nonce unused")
	}
	s.AddNonce("abc")
	if !s.NonceUsed("abc") {
		t.Fatal("after AddNonce, nonce should be used")
	}
	s.AddNonce("abc") // idempotent
	if len(s.UsedNonces) != 1 {
		t.Fatalf("AddNonce not idempotent: %v", s.UsedNonces)
	}
}

// ---------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

// resetPubKeyCache is a test-only escape hatch so we can rotate keys
// within a single test. It's defined in pubkey_reset_test.go (build tag
// not used; keeps it in test binaries only).
