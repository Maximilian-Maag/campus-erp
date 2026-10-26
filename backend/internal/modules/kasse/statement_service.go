package kasse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/mt940"
)

// StatementService implements the bank-statement import slice (S4).
type StatementService struct {
	repo  StatementRepository
	audit AuditSink
}

// NewStatementService wires the statement service.
func NewStatementService(repo StatementRepository, sink AuditSink) *StatementService {
	return &StatementService{repo: repo, audit: sink}
}

// Import parses an MT940 file, stores its statements and reconciles each line
// against the paid payment orders. It is idempotent on the file: re-importing
// the same bytes for the same account returns the statements already stored.
func (s *StatementService) Import(ctx context.Context, accountID int64, content, by string) ([]Statement, error) {
	if err := requireActor(by); err != nil {
		return nil, err
	}
	if accountID <= 0 {
		return nil, ErrCashAccountRequired
	}
	if strings.TrimSpace(content) == "" {
		return nil, ErrStatementEmpty
	}

	parsed, err := mt940.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStatementInvalid, err)
	}

	fileHash := hashBytes(content)
	if existing, err := s.repo.StatementsByFile(ctx, accountID, fileHash); err != nil {
		return nil, err
	} else if len(existing) > 0 {
		return existing, nil
	}

	// Reconcile: match each line to the paid order carrying its reference.
	refs := collectRefs(parsed)
	paid, err := s.repo.PaidOrdersByReference(ctx, refs)
	if err != nil {
		return nil, err
	}

	stmts := make([]Statement, 0, len(parsed))
	for _, ps := range parsed {
		st := Statement{
			Reference:           ps.Reference,
			Number:              ps.Number,
			Page:                ps.Page,
			Currency:            ps.Currency,
			OpeningBalanceCents: ps.OpeningBalanceCents,
			OpeningDate:         ps.OpeningDate,
			ClosingBalanceCents: ps.ClosingBalanceCents,
			ClosingDate:         ps.ClosingDate,
		}
		if st.Currency == "" {
			st.Currency = "EUR"
		}
		for i, pl := range ps.Lines {
			line := StatementLine{
				LineNo:      i + 1,
				ValueDate:   pl.ValueDate,
				BookingDate: pl.BookingDate,
				Credit:      pl.Credit,
				Return:      pl.Return,
				AmountCents: pl.AmountCents,
				Currency:    pl.Currency,
				PostingKey:  pl.PostingKey,
				ClientRef:   pl.ClientRef,
				BankRef:     pl.BankRef,
				Purpose:     pl.Purpose,
				Status:      StatusUnmatched,
			}
			if line.Currency == "" {
				line.Currency = st.Currency
			}
			if orderID, ok := paid[pl.ClientRef]; ok && pl.ClientRef != "" {
				id := orderID
				line.OrderID = &id
				line.Status = StatusMatched
				if pl.Return {
					line.Status = StatusReturned
				}
			}
			st.Lines = append(st.Lines, line)
		}
		stmts = append(stmts, st)
	}

	saved, err := s.repo.InsertStatements(ctx, accountID, fileHash, by, stmts)
	if err != nil {
		return nil, err
	}
	for _, st := range saved {
		if _, err := s.audit.Append(ctx, audit.Event{
			Actor:   by,
			Action:  "bank_statement.import",
			Subject: statementSubject(st.ID),
			After:   fmt.Sprintf("account=%d lines=%d hash=%s", accountID, st.LineCount, fileHash),
		}); err != nil {
			return nil, err
		}
	}
	return saved, nil
}

// Statement loads a statement and its lines.
func (s *StatementService) Statement(ctx context.Context, id int64) (Statement, error) {
	return s.repo.GetStatement(ctx, id)
}

// collectRefs returns the distinct non-empty client references.
func collectRefs(stmts []mt940.Statement) []string {
	seen := make(map[string]bool)
	var refs []string
	for _, st := range stmts {
		for _, l := range st.Lines {
			if l.ClientRef != "" && !seen[l.ClientRef] {
				seen[l.ClientRef] = true
				refs = append(refs, l.ClientRef)
			}
		}
	}
	return refs
}

func hashBytes(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func statementSubject(id int64) string { return fmt.Sprintf("bank_statement/%d", id) }
