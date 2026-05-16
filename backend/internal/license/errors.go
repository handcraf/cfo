// Package license — typed errors so callers (and tests) can branch on
// the specific reason a license rejected without resorting to string
// comparison.
package license

import (
	"encoding/json"
	"errors"
)

// Reason is a stable, machine-readable code surfaced in API responses
// and in the on-device CLI. Stable across versions so customer support
// scripts can pattern-match on it.
type Reason string

const (
	ReasonOK                 Reason = "ok"
	ReasonFileMissing        Reason = "file_missing"
	ReasonFileUnreadable     Reason = "file_unreadable"
	ReasonBadFormat          Reason = "bad_format"
	ReasonBadSignature       Reason = "bad_signature"
	ReasonExpired            Reason = "expired"
	ReasonMachineMismatch    Reason = "machine_mismatch"
	ReasonFeatureUnavailable Reason = "feature_unavailable"
	ReasonReplayDetected     Reason = "replay_detected"
	ReasonClockSkew          Reason = "clock_skew"
	ReasonNoPublicKey        Reason = "no_public_key"
	ReasonUnknown            Reason = "unknown"
)

// Error is the structured error type returned by every license API.
// Callers can switch on .Reason; humans get .Message.
type Error struct {
	Reason  Reason `json:"reason"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// New builds a typed Error.
func New(r Reason, msg string) *Error { return &Error{Reason: r, Message: msg} }

// AsLicenseError extracts a typed *Error from any error in the chain.
// Returns (nil, false) if not present.
func AsLicenseError(err error) (*Error, bool) {
	var le *Error
	if errors.As(err, &le) {
		return le, true
	}
	return nil, false
}

// jsonDecode is here (not in crypto.go) so the license package has a
// single canonical wrapper. It's a thin alias kept so future swaps
// (e.g. to a streaming decoder) have one place to change.
func jsonDecode(raw []byte, out any) error { return json.Unmarshal(raw, out) }
