// Package api — SQLite adapter for the AskAuditSink interface.
//
// The api package cannot import sqlstore without creating a cycle if sqlstore
// ever needed anything from api. Avoid that by defining a tiny local adapter
// that depends on the narrow AskAuditSink contract.
package api

import (
	"context"

	"github.com/cfo/backend/internal/storage/sqlstore"
)

// SQLiteAuditSink wraps *sqlstore.Store as an AskAuditSink.
type SQLiteAuditSink struct {
	Store *sqlstore.Store
}

// NewSQLiteAuditSink constructs the sink. A nil store yields a nil sink,
// which the Ask handler treats as "audit disabled" — exactly what we want
// when SQLite is turned off.
func NewSQLiteAuditSink(s *sqlstore.Store) AskAuditSink {
	if s == nil {
		return nil
	}
	return &SQLiteAuditSink{Store: s}
}

// Record appends an ask event to the ask_audit table. Failures are returned
// to the caller (who logs them) and do not block the user response.
func (s *SQLiteAuditSink) Record(ctx context.Context, e AskAuditEvent) error {
	return s.Store.RecordAskEvent(ctx, sqlstore.AskAuditRow{
		Question:    e.Question,
		Period:      e.Period,
		NumbersUsed: e.NumbersUsed,
		EvidenceIDs: e.EvidenceIDs,
		Confidence:  e.Confidence,
		Conflicts:   e.Conflicts,
		ErrorMsg:    e.Error,
	})
}
