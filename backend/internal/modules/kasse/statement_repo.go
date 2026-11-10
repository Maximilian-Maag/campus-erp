package kasse

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StatementRepository is the persistence port for bank statements.
type StatementRepository interface {
	InsertBankFile(ctx context.Context, f BankFile) (BankFile, error)
	FindBankFile(ctx context.Context, accountID int64, fileHash string) (BankFile, bool, error)
	GetBankFile(ctx context.Context, id int64) (BankFile, error)
	InsertStatements(ctx context.Context, bankFileID int64, stmts []Statement) ([]Statement, error)
	StatementsByFile(ctx context.Context, accountID int64, fileHash string) ([]Statement, error)
	GetStatement(ctx context.Context, id int64) (Statement, error)
	Statements(ctx context.Context, accountID int64) ([]StatementSummary, error)
	PaidOrdersByReference(ctx context.Context, refs []string) (map[string]int64, error)
}

// StatementRepo is the PostgreSQL implementation.
type StatementRepo struct{ pool *pgxpool.Pool }

// NewStatementRepo returns a StatementRepo.
func NewStatementRepo(pool *pgxpool.Pool) *StatementRepo { return &StatementRepo{pool: pool} }

const bankFileColumns = `id, account_id, file_name, file_hash, imported_by`

const statementColumns = `id, account_id, bank_file_id, reference, statement_number, page, currency,
	opening_balance_cents, opening_date, closing_balance_cents, closing_date,
	line_count, imported_by`

const lineColumns = `id, line_no, value_date, booking_date, credit, is_return,
	amount_cents, currency, posting_key, client_ref, bank_ref, purpose,
	end_to_end_ref, counterparty_name, counterparty_iban, counterparty_bic,
	uncleared_cents, status, order_id`

func nullDate(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func derefDate(p *time.Time) time.Time {
	if p == nil {
		return time.Time{}
	}
	return *p
}

// Statements lists imported statements, newest closing date first — the order an operator looks
// for one in — optionally narrowed to one account (0 for all).
func (r *StatementRepo) Statements(ctx context.Context, accountID int64) ([]StatementSummary, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, account_id, reference, statement_number, page, currency,
		        opening_balance_cents, opening_date, closing_balance_cents, closing_date,
		        line_count, imported_by
		   FROM bank_statement
		  -- The cast is required: comparing the parameter to a bare 0 would type it as an
		  -- integer, and an account id past the 32-bit range would be refused rather than
		  -- simply matching nothing.
		  WHERE ($1::bigint = 0 OR account_id = $1::bigint)
		  -- A statement the bank sent without a closing date sorts last: it has no period to be
		  -- found by, and NULLs would otherwise lead the newest-first order.
		  ORDER BY closing_date DESC NULLS LAST, statement_number DESC`, accountID)
	if err != nil {
		return nil, fmt.Errorf("kasse: list statements: %w", err)
	}
	defer rows.Close()
	out := []StatementSummary{}
	for rows.Next() {
		var s StatementSummary
		// A statement's dates are nullable in the table, so they are read the same way the
		// single-statement read reads them.
		var openingDate, closingDate *time.Time
		if err := rows.Scan(&s.ID, &s.AccountID, &s.Reference, &s.Number, &s.Page, &s.Currency,
			&s.OpeningBalanceCents, &openingDate, &s.ClosingBalanceCents, &closingDate,
			&s.LineCount, &s.ImportedBy); err != nil {
			return nil, fmt.Errorf("kasse: scan statement summary: %w", err)
		}
		s.OpeningDate = derefDate(openingDate)
		s.ClosingDate = derefDate(closingDate)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kasse: list statements: %w", err)
	}
	return out, nil
}

// InsertBankFile stores an imported file.
func (r *StatementRepo) InsertBankFile(ctx context.Context, f BankFile) (BankFile, error) {
	out, err := scanBankFile(r.pool.QueryRow(ctx,
		`INSERT INTO bank_file (account_id, file_name, file_hash, imported_by)
		 VALUES ($1,$2,$3,$4)
		 RETURNING `+bankFileColumns,
		f.AccountID, f.FileName, f.FileHash, f.ImportedBy))
	if err != nil {
		return BankFile{}, fmt.Errorf("kasse: insert bank file: %w", err)
	}
	return out, nil
}

// FindBankFile returns the file already imported for an account and hash.
func (r *StatementRepo) FindBankFile(ctx context.Context, accountID int64, fileHash string) (BankFile, bool, error) {
	f, err := scanBankFile(r.pool.QueryRow(ctx,
		`SELECT `+bankFileColumns+` FROM bank_file WHERE account_id = $1 AND file_hash = $2`, accountID, fileHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return BankFile{}, false, nil
	}
	if err != nil {
		return BankFile{}, false, fmt.Errorf("kasse: find bank file: %w", err)
	}
	return f, true, nil
}

// GetBankFile loads a file by id.
func (r *StatementRepo) GetBankFile(ctx context.Context, id int64) (BankFile, error) {
	f, err := scanBankFile(r.pool.QueryRow(ctx, `SELECT `+bankFileColumns+` FROM bank_file WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return BankFile{}, fmt.Errorf("kasse: bank file %d: %w", id, ErrBankFileNotFound)
	}
	if err != nil {
		return BankFile{}, fmt.Errorf("kasse: get bank file: %w", err)
	}
	return f, nil
}

// InsertStatements stores each statement and its lines in one transaction.
func (r *StatementRepo) InsertStatements(ctx context.Context, bankFileID int64, stmts []Statement) ([]Statement, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("kasse: begin statement import: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	out := make([]Statement, 0, len(stmts))
	for _, s := range stmts {
		saved, err := scanStatement(tx.QueryRow(ctx,
			`INSERT INTO bank_statement
			   (account_id, bank_file_id, reference, statement_number, page, currency,
			    opening_balance_cents, opening_date, closing_balance_cents, closing_date,
			    line_count, imported_by)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			 RETURNING `+statementColumns,
			s.AccountID, bankFileID, s.Reference, s.Number, s.Page, s.Currency,
			s.OpeningBalanceCents, nullDate(s.OpeningDate), s.ClosingBalanceCents, nullDate(s.ClosingDate),
			len(s.Lines), s.ImportedBy))
		if err != nil {
			return nil, fmt.Errorf("kasse: insert statement: %w", err)
		}
		for i := range s.Lines {
			line := &s.Lines[i]
			savedLine, err := scanLine(tx.QueryRow(ctx,
				`INSERT INTO bank_statement_line
				   (statement_id, line_no, value_date, booking_date, credit, is_return,
				    amount_cents, currency, posting_key, client_ref, bank_ref, purpose,
				    end_to_end_ref, counterparty_name, counterparty_iban, counterparty_bic,
				    uncleared_cents, status, order_id)
				 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
				 RETURNING `+lineColumns,
				saved.ID, line.LineNo, nullDate(line.ValueDate), nullDate(line.BookingDate),
				line.Credit, line.Return, line.AmountCents, line.Currency, line.PostingKey,
				line.ClientRef, line.BankRef, line.Purpose, line.EndToEndRef, line.CounterpartyName,
				line.CounterpartyIBAN, line.CounterpartyBIC, line.UnclearedCents, line.Status, line.OrderID))
			if err != nil {
				return nil, fmt.Errorf("kasse: insert statement line: %w", err)
			}
			saved.Lines = append(saved.Lines, savedLine)
		}
		out = append(out, saved)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("kasse: commit statement import: %w", err)
	}
	return out, nil
}

// StatementsByFile returns the statements already imported for a file hash.
func (r *StatementRepo) StatementsByFile(ctx context.Context, accountID int64, fileHash string) ([]Statement, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+statementColumns+`
		   FROM bank_statement
		  WHERE bank_file_id = (SELECT id FROM bank_file WHERE account_id = $1 AND file_hash = $2)
		  ORDER BY id`,
		accountID, fileHash)
	if err != nil {
		return nil, fmt.Errorf("kasse: statements by file: %w", err)
	}
	defer rows.Close()
	var out []Statement
	for rows.Next() {
		s, err := scanStatement(rows)
		if err != nil {
			return nil, fmt.Errorf("kasse: scan statement: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetStatement loads a statement and its lines.
func (r *StatementRepo) GetStatement(ctx context.Context, id int64) (Statement, error) {
	s, err := scanStatement(r.pool.QueryRow(ctx, `SELECT `+statementColumns+` FROM bank_statement WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Statement{}, fmt.Errorf("kasse: statement %d: %w", id, ErrStatementNotFound)
	}
	if err != nil {
		return Statement{}, fmt.Errorf("kasse: get statement: %w", err)
	}
	rows, err := r.pool.Query(ctx, `SELECT `+lineColumns+` FROM bank_statement_line WHERE statement_id = $1 ORDER BY line_no`, id)
	if err != nil {
		return Statement{}, fmt.Errorf("kasse: get statement lines: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		line, err := scanLine(rows)
		if err != nil {
			return Statement{}, fmt.Errorf("kasse: scan statement line: %w", err)
		}
		s.Lines = append(s.Lines, line)
	}
	return s, rows.Err()
}

// ReturnedLines lists the statement lines the bank took back in [from, to], in
// (effective date, id) order so the return export is deterministic. The effective date
// is the value date, falling back to the booking date when the bank sent none: a
// returning line that carried neither cannot be dated and does not belong to a period.
func (r *StatementRepo) ReturnedLines(ctx context.Context, from, to time.Time) ([]StatementLine, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+lineColumns+`
		   FROM bank_statement_line
		  WHERE status = 'returned'
		    AND coalesce(value_date, booking_date) BETWEEN $1 AND $2
		  ORDER BY coalesce(value_date, booking_date), id`, from, to)
	if err != nil {
		return nil, fmt.Errorf("kasse: returned lines: %w", err)
	}
	defer rows.Close()
	out := []StatementLine{}
	for rows.Next() {
		line, err := scanLine(rows)
		if err != nil {
			return nil, fmt.Errorf("kasse: scan returned line: %w", err)
		}
		out = append(out, line)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("kasse: returned lines: %w", err)
	}
	return out, nil
}

// PaidOrdersByReference returns the ids of paid orders (those that belong to a
// payment run) keyed by their reference.
func (r *StatementRepo) PaidOrdersByReference(ctx context.Context, refs []string) (map[string]int64, error) {
	out := make(map[string]int64)
	if len(refs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT o.reference, o.id
		   FROM payment_order o
		   JOIN payment_run_item i ON i.order_id = o.id
		  WHERE o.reference = ANY($1)`, refs)
	if err != nil {
		return nil, fmt.Errorf("kasse: paid orders by reference: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ref string
		var id int64
		if err := rows.Scan(&ref, &id); err != nil {
			return nil, fmt.Errorf("kasse: scan paid order: %w", err)
		}
		out[ref] = id
	}
	return out, rows.Err()
}

func scanBankFile(row pgx.Row) (BankFile, error) {
	var f BankFile
	err := row.Scan(&f.ID, &f.AccountID, &f.FileName, &f.FileHash, &f.ImportedBy)
	return f, err
}

func scanStatement(row pgx.Row) (Statement, error) {
	var s Statement
	var openingDate, closingDate *time.Time
	err := row.Scan(&s.ID, &s.AccountID, &s.BankFileID, &s.Reference, &s.Number, &s.Page, &s.Currency,
		&s.OpeningBalanceCents, &openingDate, &s.ClosingBalanceCents, &closingDate,
		&s.LineCount, &s.ImportedBy)
	if err != nil {
		return Statement{}, err
	}
	s.OpeningDate = derefDate(openingDate)
	s.ClosingDate = derefDate(closingDate)
	return s, nil
}

func scanLine(row pgx.Row) (StatementLine, error) {
	var l StatementLine
	var valueDate, bookingDate *time.Time
	err := row.Scan(&l.ID, &l.LineNo, &valueDate, &bookingDate, &l.Credit, &l.Return,
		&l.AmountCents, &l.Currency, &l.PostingKey, &l.ClientRef, &l.BankRef, &l.Purpose,
		&l.EndToEndRef, &l.CounterpartyName, &l.CounterpartyIBAN, &l.CounterpartyBIC,
		&l.UnclearedCents, &l.Status, &l.OrderID)
	if err != nil {
		return StatementLine{}, err
	}
	l.ValueDate = derefDate(valueDate)
	l.BookingDate = derefDate(bookingDate)
	return l, nil
}
