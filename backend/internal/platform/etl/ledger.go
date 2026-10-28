package etl

import (
	"context"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// EventReader reads back the most recent audit event for an action and subject.
// audit.Store satisfies it.
type EventReader interface {
	LastEvent(ctx context.Context, action, subject string) (audit.Event, bool, error)
}

// AuditLedger derives a dataset's previous checksum from the audit stream: the
// run history is the hash-chained audit log itself, so drift detection needs no
// separate ledger table.
type AuditLedger struct {
	reader EventReader
}

// NewAuditLedger wires the ledger to an event reader.
func NewAuditLedger(reader EventReader) AuditLedger {
	return AuditLedger{reader: reader}
}

// LastChecksum implements Ledger over the last recorded etl.run event for the
// dataset. It reports ok=false when the dataset has never been run.
func (l AuditLedger) LastChecksum(ctx context.Context, dataset string) (string, bool, error) {
	event, ok, err := l.reader.LastEvent(ctx, "etl.run", "etl/"+dataset)
	if err != nil || !ok {
		return "", false, err
	}
	checksum, ok := checksumOf(event.After)
	return checksum, ok, nil
}

// checksumOf extracts the checksum written by afterText, if present.
func checksumOf(detail string) (string, bool) {
	for _, field := range strings.Fields(detail) {
		if value, ok := strings.CutPrefix(field, checksumPrefix); ok {
			return value, true
		}
	}
	return "", false
}
