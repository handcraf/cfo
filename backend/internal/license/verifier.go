// Package license — startup verification + migration handling.
//
// The Verifier is the single entry-point the backend uses at startup.
// One call resolves: file → bytes → signature → expiry → machine bind
// → activation state. The backend acts on the returned VerifyResult
// (and ONLY on that) to decide whether to expose APIs.
package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// VerifyResult is what the backend sees. The OK field is the only
// gate: any false makes the backend refuse to expose business APIs.
type VerifyResult struct {
	OK             bool      `json:"ok"`
	Reason         Reason    `json:"reason"`
	Message        string    `json:"message"`
	Payload        *Payload  `json:"payload,omitempty"`
	MachineID      string    `json:"machine_id,omitempty"`
	DaysRemaining  int       `json:"days_remaining"`
	Activated      bool      `json:"activated"`
	WarningDays    int       `json:"warning_days,omitempty"`    // ">0" when within renewal window
	ValidatedAtUTC time.Time `json:"validated_at_utc"`
}

// VerifyOptions tunes verification for tests / future use.
type VerifyOptions struct {
	// Now overrides time.Now (testing).
	Now func() time.Time
	// RenewalWarningDays controls when we set WarningDays. Default 30.
	RenewalWarningDays int
	// SkipMachineBind disables the local fingerprint compare. ONLY
	// used by the on-device CLI's `status` command, so a user can
	// inspect an expired license from a different host. Never set
	// this true from the backend startup path.
	SkipMachineBind bool
}

// Verifier wires file paths together with the public key and state.
type Verifier struct {
	LicenseFilePath string
	StateFilePath   string
}

// NewVerifier constructs a Verifier with sensible defaults.
func NewVerifier(licensePath, statePath string) *Verifier {
	return &Verifier{
		LicenseFilePath: licensePath,
		StateFilePath:   statePath,
	}
}

// Verify is the startup-gate call. It performs the full validation
// pipeline and returns a structured VerifyResult.
//
// Pipeline (each step short-circuits on failure):
//  1. Public key resolved (compile-time embed / env / fail)
//  2. License file present & readable
//  3. JSON well-formed
//  4. Ed25519 signature verifies → tampering caught here
//  5. Local state load succeeds (decrypts) → tampering caught here
//  6. State.Deactivated == false → must reactivate
//  7. Machine fingerprint match (or first-time bind)
//  8. Expiry not exceeded
//  9. Stamp LastValidatedAt and persist state
func (v *Verifier) Verify(opts VerifyOptions) VerifyResult {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	warnDays := opts.RenewalWarningDays
	if warnDays == 0 {
		warnDays = 30
	}

	res := VerifyResult{ValidatedAtUTC: now().UTC()}

	// 1. Public key resolvable.
	if _, err := VendorPublicKey(); err != nil {
		res.Reason = ReasonNoPublicKey
		res.Message = err.Error()
		return res
	}

	// 2. File exists & readable.
	rawFile, err := os.ReadFile(v.LicenseFilePath)
	if errors.Is(err, os.ErrNotExist) {
		res.Reason = ReasonFileMissing
		res.Message = fmt.Sprintf("license file not found at %s — request one from your vendor", v.LicenseFilePath)
		return res
	}
	if err != nil {
		res.Reason = ReasonFileUnreadable
		res.Message = err.Error()
		return res
	}

	// 3. JSON well-formed.
	var f File
	if err := json.Unmarshal(rawFile, &f); err != nil {
		res.Reason = ReasonBadFormat
		res.Message = fmt.Sprintf("license file is not valid JSON: %v", err)
		return res
	}

	// 4. Signature.
	payload, err := VerifyFile(f)
	if err != nil {
		if errors.Is(err, ErrBadSignature) {
			res.Reason = ReasonBadSignature
			res.Message = "license signature does not verify — the file has been tampered with or was not issued by this vendor"
			return res
		}
		res.Reason = ReasonBadFormat
		res.Message = err.Error()
		return res
	}
	res.Payload = &payload

	// 5. Local state load (decrypts on the calling machine only).
	state, err := LoadState(v.StateFilePath)
	if err != nil {
		res.Reason = ReasonBadFormat
		res.Message = fmt.Sprintf("local activation state unreadable: %v", err)
		return res
	}

	// 6. Deactivated?
	if state.Deactivated {
		res.Reason = ReasonMachineMismatch
		res.Message = "license was deactivated on this machine — run `cfo-license activate <migration.dat>` to re-bind"
		return res
	}

	// 7. Machine fingerprint.
	mid, err := MachineID()
	if err != nil {
		res.Reason = ReasonUnknown
		res.Message = fmt.Sprintf("machine fingerprint: %v", err)
		return res
	}
	res.MachineID = mid

	if !opts.SkipMachineBind {
		switch {
		case state.MachineID == "":
			// First-time activation: bind now.
			state.CustomerID = payload.CustomerID
			state.MachineID = mid
			state.ActivatedAt = now().UTC()
			state.LicensePayloadB64 = base64.StdEncoding.EncodeToString(mustCanonical(payload))
			if err := state.GenerateActivationKeypair(); err != nil {
				res.Reason = ReasonUnknown
				res.Message = err.Error()
				return res
			}
			res.Activated = true
		case state.MachineID == mid:
			res.Activated = true
		default:
			res.Reason = ReasonMachineMismatch
			res.Message = fmt.Sprintf(
				"license is bound to a different machine (state=%s, current=%s) — deactivate on the old host first",
				short(state.MachineID), short(mid),
			)
			return res
		}

		// If the license itself carries a MachineID, it must match too.
		// (Vendor-issued machine-locked license.)
		if payload.MachineID != "" && payload.MachineID != mid {
			res.Reason = ReasonMachineMismatch
			res.Message = "license is locked to a different machine_id in the payload"
			return res
		}
	}

	// 8. Expiry.
	if payload.IsExpired(now()) {
		res.Reason = ReasonExpired
		res.Message = fmt.Sprintf("license expired on %s — run `cfo-license export-request` to start renewal", payload.Expiry.Format("2006-01-02"))
		res.DaysRemaining = payload.DaysRemaining(now())
		return res
	}

	// 9. Stamp & persist state.
	res.DaysRemaining = payload.DaysRemaining(now())
	if res.DaysRemaining <= warnDays {
		res.WarningDays = res.DaysRemaining
	}
	state.LastValidatedAt = now().UTC()
	if err := SaveState(v.StateFilePath, state); err != nil {
		// Persistence failure is logged but not fatal — license is
		// still valid for THIS run.
		fmt.Fprintf(os.Stderr, "[license] warning: could not save state: %v\n", err)
	}

	res.OK = true
	res.Reason = ReasonOK
	res.Message = fmt.Sprintf("license valid for %s (%d days remaining)", payload.CustomerName, res.DaysRemaining)
	return res
}

// HasFeature is the cheap "is this feature enabled" check used by
// HTTP handlers. Returns true only when the verifier currently
// considers the license valid AND the feature is in the payload's
// Features list.
func (r *VerifyResult) HasFeature(f Feature) bool {
	if !r.OK || r.Payload == nil {
		return false
	}
	return r.Payload.HasFeature(f)
}

// ---------------------------------------------------------------------
// Migration: deactivate / activate
// ---------------------------------------------------------------------

// Deactivate produces a signed MigrationFile and marks the local state
// as Deactivated. After this call, the customer-bearing host will NOT
// start the backend until `Activate` runs again (or a fresh license.lic
// is installed).
func (v *Verifier) Deactivate() (MigrationFile, error) {
	state, err := LoadState(v.StateFilePath)
	if err != nil {
		return MigrationFile{}, err
	}
	if state.MachineID == "" {
		return MigrationFile{}, New(ReasonUnknown, "no active license to deactivate")
	}
	if state.Deactivated {
		return MigrationFile{}, New(ReasonUnknown, "license already deactivated")
	}

	priv, err := state.ActivationPrivateKey()
	if err != nil {
		return MigrationFile{}, err
	}

	// Hash of the license payload so the receiving machine knows which
	// license this migration applies to. The payload bytes are stored
	// in state at activation time.
	payloadBytes, err := base64.StdEncoding.DecodeString(state.LicensePayloadB64)
	if err != nil || len(payloadBytes) == 0 {
		return MigrationFile{}, New(ReasonBadFormat, "state has no license payload — cannot deactivate")
	}
	digest := sha256.Sum256(payloadBytes)

	nonce, err := randomHex(16)
	if err != nil {
		return MigrationFile{}, err
	}

	m := MigrationFile{
		CustomerID:    state.CustomerID,
		OldMachineID:  state.MachineID,
		Timestamp:     time.Now().UTC(),
		Nonce:         nonce,
		LicenseDigest: hex.EncodeToString(digest[:]),
	}
	body, err := m.CanonicalBytes()
	if err != nil {
		return MigrationFile{}, err
	}
	sig := ed25519.Sign(priv, body)
	m.Signature = base64.StdEncoding.EncodeToString(sig)

	// Mark state deactivated. We do NOT add the nonce to UsedNonces
	// here — that field is reserved for INBOUND replay-protection on
	// activate(). Adding it here would prevent re-activating the same
	// migration back onto this same machine (a legitimate "undo
	// deactivation" flow). Double-deactivation is already prevented
	// by the Deactivated flag we just set.
	state.Deactivated = true
	if err := SaveState(v.StateFilePath, state); err != nil {
		return MigrationFile{}, fmt.Errorf("deactivate: state save: %w", err)
	}
	return m, nil
}

// Activate imports a MigrationFile on a NEW machine. The flow:
//
//  1. Verify migration signature using the activation public key,
//     which is embedded in the migration file itself OR carried in
//     the (offline-transferred) license payload — whichever is
//     available. v1 carries the pub key alongside.
//  2. Verify the digest matches the license payload bytes already
//     on this machine (license.lic must be present).
//  3. Verify the nonce is fresh (not in local state).
//  4. Verify timestamp is within tolerance (clock skew check).
//  5. Bind the local machine to the license.
//
// activationPubKey is the public half from the OLD machine's state.
// In v1 the customer ships both the migration.dat AND the file
// migration_pub.key (or this can be embedded — see CLI). The
// signature alone isn't enough without this pubkey — we don't have
// PKI yet.
func (v *Verifier) Activate(m MigrationFile, activationPubKey ed25519.PublicKey) error {
	// 1. Signature.
	body, err := m.CanonicalBytes()
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil {
		return fmt.Errorf("activate: signature base64: %w", err)
	}
	if !ed25519.Verify(activationPubKey, body, sig) {
		return New(ReasonBadSignature, "migration file signature does not verify")
	}

	// 2. License must be on this host already (the customer copied
	//    BOTH license.lic AND migration.dat to the new machine).
	rawLic, err := os.ReadFile(v.LicenseFilePath)
	if err != nil {
		return New(ReasonFileMissing, fmt.Sprintf("activate: license.lic not found at %s — copy it from the old host first", v.LicenseFilePath))
	}
	var f File
	if err := json.Unmarshal(rawLic, &f); err != nil {
		return New(ReasonBadFormat, "activate: license file is not valid JSON")
	}
	payload, err := VerifyFile(f)
	if err != nil {
		return err
	}
	_, payloadBytes, _, _ := f.Decode()
	digest := sha256.Sum256(payloadBytes)
	if hex.EncodeToString(digest[:]) != m.LicenseDigest {
		return New(ReasonBadFormat, "activate: migration file is for a different license (digest mismatch)")
	}

	// 3. Nonce freshness — load (or create) local state and check.
	state, err := LoadState(v.StateFilePath)
	if err != nil {
		// "Different machine" decrypt failure is expected — the
		// migration target hasn't activated yet, so its state.enc
		// may not even exist or may be from a prior tenant. Fall
		// through to a fresh state.
		state = &State{}
	}
	if state.NonceUsed(m.Nonce) {
		return New(ReasonReplayDetected, "activate: migration nonce already used on this machine (replay)")
	}

	// 4. Clock-skew guard. Reject migrations older than 30 days or
	//    timestamped >5 minutes in the future.
	now := time.Now().UTC()
	if m.Timestamp.After(now.Add(5 * time.Minute)) {
		return New(ReasonClockSkew, "activate: migration timestamp is in the future")
	}
	if now.Sub(m.Timestamp) > 30*24*time.Hour {
		return New(ReasonClockSkew, "activate: migration file is older than 30 days — request a fresh one")
	}

	// 5. Bind.
	mid, err := MachineID()
	if err != nil {
		return err
	}
	state.CustomerID = payload.CustomerID
	state.MachineID = mid
	state.ActivatedAt = now
	state.LastValidatedAt = now
	state.LicensePayloadB64 = base64.StdEncoding.EncodeToString(payloadBytes)
	state.Deactivated = false
	state.AddNonce(m.Nonce)
	// Generate a FRESH activation keypair on the new machine so the
	// old one stays invalidated.
	state.ActivationPubKeyB64 = ""
	state.ActivationPrivKeyB64 = ""
	if err := state.GenerateActivationKeypair(); err != nil {
		return err
	}
	if err := SaveState(v.StateFilePath, state); err != nil {
		return err
	}
	return nil
}

// ExportRenewalRequest builds a (NOT signed) renewal request for the
// active license. Empty if no license is active.
func (v *Verifier) ExportRenewalRequest() (RenewalRequest, error) {
	state, err := LoadState(v.StateFilePath)
	if err != nil {
		return RenewalRequest{}, err
	}
	if state.MachineID == "" {
		return RenewalRequest{}, New(ReasonUnknown, "no active license — nothing to renew")
	}
	rawLic, err := os.ReadFile(v.LicenseFilePath)
	if err != nil {
		return RenewalRequest{}, err
	}
	var f File
	if err := json.Unmarshal(rawLic, &f); err != nil {
		return RenewalRequest{}, err
	}
	payload, _, _, err := f.Decode()
	if err != nil {
		return RenewalRequest{}, err
	}
	return RenewalRequest{
		CustomerID:    payload.CustomerID,
		CustomerName:  payload.CustomerName,
		MachineID:     state.MachineID,
		CurrentExpiry: payload.Expiry,
		GeneratedAt:   time.Now().UTC(),
	}, nil
}

// ---------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------

func mustCanonical(p Payload) []byte {
	b, _ := p.CanonicalBytes()
	return b
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func short(s string) string {
	if len(s) <= 12 {
		return s
	}
	return s[:6] + "…" + s[len(s)-6:]
}
