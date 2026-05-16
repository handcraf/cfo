package license

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeLicenseFile signs the payload with priv and writes the license.lic.
func writeLicenseFile(t *testing.T, dir string, p Payload) string {
	t.Helper()
	_, priv := testKeypair(t)
	f, _, err := SignPayload(p, priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	raw, _ := json.Marshal(f)
	path := filepath.Join(dir, "license.lic")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestVerify_FileMissing(t *testing.T) {
	dir := t.TempDir()
	v := NewVerifier(filepath.Join(dir, "absent.lic"), filepath.Join(dir, "state.enc"))
	r := v.Verify(VerifyOptions{})
	if r.OK {
		t.Fatal("expected verify to fail with missing file")
	}
	if r.Reason != ReasonFileMissing {
		t.Errorf("reason: got %s want %s", r.Reason, ReasonFileMissing)
	}
}

func TestVerify_HappyPathBindsMachine(t *testing.T) {
	dir := t.TempDir()
	licPath := writeLicenseFile(t, dir, samplePayload(time.Now().UTC()))
	v := NewVerifier(licPath, filepath.Join(dir, "state.enc"))

	r := v.Verify(VerifyOptions{})
	if !r.OK {
		t.Fatalf("verify failed: reason=%s msg=%s", r.Reason, r.Message)
	}
	if r.Activated != true {
		t.Error("expected first-time activation to set Activated=true")
	}
	if r.MachineID == "" {
		t.Error("MachineID should be set on a happy verify")
	}
	if r.DaysRemaining < 360 {
		t.Errorf("days remaining: got %d, want ~365", r.DaysRemaining)
	}
}

func TestVerify_ExpiredLicense(t *testing.T) {
	dir := t.TempDir()
	p := samplePayload(time.Now().AddDate(-2, 0, 0))
	p.Expiry = time.Now().AddDate(0, -1, 0)
	licPath := writeLicenseFile(t, dir, p)
	v := NewVerifier(licPath, filepath.Join(dir, "state.enc"))

	r := v.Verify(VerifyOptions{})
	if r.OK {
		t.Fatal("expected verify to fail on expired license")
	}
	if r.Reason != ReasonExpired {
		t.Errorf("reason: got %s want %s", r.Reason, ReasonExpired)
	}
}

func TestVerify_SecondCallReusesBinding(t *testing.T) {
	dir := t.TempDir()
	licPath := writeLicenseFile(t, dir, samplePayload(time.Now().UTC()))
	v := NewVerifier(licPath, filepath.Join(dir, "state.enc"))

	if r := v.Verify(VerifyOptions{}); !r.OK {
		t.Fatalf("first verify failed: %s", r.Message)
	}
	r2 := v.Verify(VerifyOptions{})
	if !r2.OK {
		t.Fatalf("second verify failed: %s", r2.Message)
	}
	if !r2.Activated {
		t.Error("expected second verify to still report Activated=true")
	}
}

func TestVerify_WarnsNearExpiry(t *testing.T) {
	dir := t.TempDir()
	p := samplePayload(time.Now().AddDate(0, -11, 0))
	p.Expiry = time.Now().AddDate(0, 0, 5)
	licPath := writeLicenseFile(t, dir, p)
	v := NewVerifier(licPath, filepath.Join(dir, "state.enc"))

	r := v.Verify(VerifyOptions{RenewalWarningDays: 30})
	if !r.OK {
		t.Fatalf("verify failed: %s", r.Message)
	}
	if r.WarningDays == 0 {
		t.Error("expected WarningDays to be set when within renewal window")
	}
}

// ---------------------------------------------------------------------
// Migration roundtrip
// ---------------------------------------------------------------------

func TestMigration_DeactivateThenActivate(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	// Activate on machine 1.
	licPath1 := writeLicenseFile(t, dir1, samplePayload(time.Now().UTC()))
	v1 := NewVerifier(licPath1, filepath.Join(dir1, "state.enc"))
	if r := v1.Verify(VerifyOptions{}); !r.OK {
		t.Fatalf("initial activation: %s", r.Message)
	}

	// Deactivate on machine 1.
	mig, err := v1.Deactivate()
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if mig.Signature == "" {
		t.Fatal("deactivate returned an unsigned migration")
	}

	// Load the activation public key that will be needed on machine 2.
	st1, _ := LoadState(filepath.Join(dir1, "state.enc"))
	pubKey, err := st1.ActivationPublicKey()
	if err != nil {
		t.Fatalf("read pub key: %v", err)
	}

	// Re-verify on machine 1 — should now refuse because deactivated.
	if r := v1.Verify(VerifyOptions{}); r.OK {
		t.Fatal("verify should fail on deactivated machine")
	}

	// Copy license.lic to machine 2 and activate.
	raw, _ := os.ReadFile(licPath1)
	licPath2 := filepath.Join(dir2, "license.lic")
	if err := os.WriteFile(licPath2, raw, 0o600); err != nil {
		t.Fatalf("copy lic: %v", err)
	}
	v2 := NewVerifier(licPath2, filepath.Join(dir2, "state.enc"))
	if err := v2.Activate(mig, pubKey); err != nil {
		t.Fatalf("activate on machine 2: %v", err)
	}
	r := v2.Verify(VerifyOptions{})
	if !r.OK {
		t.Fatalf("verify on machine 2: %s", r.Message)
	}

	// Replay protection: second activate with the same migration must fail.
	if err := v2.Activate(mig, pubKey); err == nil {
		t.Fatal("replay of migration on same target should fail")
	}
}

func TestMigration_DigestMismatchFails(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	licPath1 := writeLicenseFile(t, dir1, samplePayload(time.Now().UTC()))
	v1 := NewVerifier(licPath1, filepath.Join(dir1, "state.enc"))
	_ = v1.Verify(VerifyOptions{})

	mig, err := v1.Deactivate()
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	st1, _ := LoadState(filepath.Join(dir1, "state.enc"))
	pubKey, _ := st1.ActivationPublicKey()

	// On machine 2, write a DIFFERENT license file (different customer).
	diff := samplePayload(time.Now().UTC())
	diff.CustomerID = "OTHER-002"
	diff.CustomerName = "Other Co"
	_ = writeLicenseFile(t, dir2, diff)
	v2 := NewVerifier(filepath.Join(dir2, "license.lic"), filepath.Join(dir2, "state.enc"))

	if err := v2.Activate(mig, pubKey); err == nil {
		t.Fatal("activate should fail when license digest doesn't match")
	}
}

func TestExportRenewalRequest(t *testing.T) {
	dir := t.TempDir()
	licPath := writeLicenseFile(t, dir, samplePayload(time.Now().UTC()))
	v := NewVerifier(licPath, filepath.Join(dir, "state.enc"))
	if r := v.Verify(VerifyOptions{}); !r.OK {
		t.Fatalf("verify: %s", r.Message)
	}

	req, err := v.ExportRenewalRequest()
	if err != nil {
		t.Fatalf("export renewal: %v", err)
	}
	if req.CustomerID != "ACME-001" {
		t.Errorf("customer id: got %q want ACME-001", req.CustomerID)
	}
	if req.MachineID == "" {
		t.Error("machine id should be present in renewal request")
	}
}
