package kasse

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CollectionRunRepository is the persistence port for direct-debit collection runs.
type CollectionRunRepository interface {
	CollectionRunByReference(ctx context.Context, reference string) (CollectionRun, bool, error)
	GetCollectionRun(ctx context.Context, reference string) (CollectionRun, error)
	CollectionRunFile(ctx context.Context, reference string) (string, error)
	InsertCollectionRun(ctx context.Context, run CollectionRun, fileXML string, items []CollectionRunItem) (CollectionRun, error)
	InsertCollectionRunFromReceivables(ctx context.Context, run CollectionRun, fileXML string, items []CollectionRunItem, receivableIDs []int64) (CollectionRun, error)
}

// CollectionRunRepo is the PostgreSQL implementation.
type CollectionRunRepo struct{ pool *pgxpool.Pool }

// NewCollectionRunRepo returns a CollectionRunRepo.
func NewCollectionRunRepo(pool *pgxpool.Pool) *CollectionRunRepo {
	return &CollectionRunRepo{pool: pool}
}

const collectionRunColumns = `id, reference, account_id, creditor_id, collection_date,
	sequence_type, local_instrument, message_id, item_count, sum_cents, file_hash, created_by`

func scanCollectionRun(row pgx.Row) (CollectionRun, error) {
	var c CollectionRun
	err := row.Scan(&c.ID, &c.Reference, &c.AccountID, &c.CreditorID, &c.CollectionDate,
		&c.SequenceType, &c.LocalInstrument, &c.MessageID, &c.ItemCount, &c.SumCents,
		&c.FileHash, &c.CreatedBy)
	return c, err
}

// CollectionRunByReference returns the run with the given reference, if any.
func (r *CollectionRunRepo) CollectionRunByReference(ctx context.Context, reference string) (CollectionRun, bool, error) {
	run, err := scanCollectionRun(r.pool.QueryRow(ctx,
		`SELECT `+collectionRunColumns+` FROM direct_debit_run WHERE reference = $1`, reference))
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectionRun{}, false, nil
	}
	if err != nil {
		return CollectionRun{}, false, fmt.Errorf("kasse: collection run by reference: %w", err)
	}
	return run, true, nil
}

// GetCollectionRun loads a run by reference.
func (r *CollectionRunRepo) GetCollectionRun(ctx context.Context, reference string) (CollectionRun, error) {
	run, err := scanCollectionRun(r.pool.QueryRow(ctx,
		`SELECT `+collectionRunColumns+` FROM direct_debit_run WHERE reference = $1`, reference))
	if errors.Is(err, pgx.ErrNoRows) {
		return CollectionRun{}, fmt.Errorf("kasse: collection run %q: %w", reference, ErrCollectionRunNotFound)
	}
	if err != nil {
		return CollectionRun{}, fmt.Errorf("kasse: get collection run: %w", err)
	}
	return run, nil
}

// CollectionRunFile returns the stored pain.008 file of a run.
func (r *CollectionRunRepo) CollectionRunFile(ctx context.Context, reference string) (string, error) {
	var xml string
	err := r.pool.QueryRow(ctx, `SELECT file_xml FROM direct_debit_run WHERE reference = $1`, reference).Scan(&xml)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("kasse: collection run %q: %w", reference, ErrCollectionRunNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("kasse: get collection run file: %w", err)
	}
	return xml, nil
}

// InsertCollectionRun stores a run and its items atomically. A duplicate reference
// maps to ErrRunReferenceExists; a duplicate mandate maps to ErrDuplicateMandate.
func (r *CollectionRunRepo) InsertCollectionRun(ctx context.Context, run CollectionRun, fileXML string, items []CollectionRunItem) (CollectionRun, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CollectionRun{}, fmt.Errorf("kasse: begin collection run: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	saved, err := insertRunWithin(ctx, tx, run, fileXML, items)
	if err != nil {
		return CollectionRun{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return CollectionRun{}, fmt.Errorf("kasse: commit collection run: %w", err)
	}
	return saved, nil
}

// InsertCollectionRunFromReceivables stores a run, its items and the open items
// it claims, in one transaction: the run and the collected state of its
// receivables are written together, so a stored run never points at items still
// shown open. A receivable that is no longer open — taken by another run in the
// meantime — maps to ErrReceivableNotOpen and the whole run is rolled back.
func (r *CollectionRunRepo) InsertCollectionRunFromReceivables(ctx context.Context, run CollectionRun, fileXML string, items []CollectionRunItem, receivableIDs []int64) (CollectionRun, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return CollectionRun{}, fmt.Errorf("kasse: begin collection run: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	saved, err := insertRunWithin(ctx, tx, run, fileXML, items)
	if err != nil {
		return CollectionRun{}, err
	}
	tag, err := tx.Exec(ctx,
		`UPDATE receivable SET state = 'collected', collected_run_id = $2
		  WHERE id = ANY($1) AND state = 'open'`, receivableIDs, saved.ID)
	if err != nil {
		return CollectionRun{}, fmt.Errorf("kasse: claim receivables: %w", err)
	}
	if int(tag.RowsAffected()) != len(receivableIDs) {
		return CollectionRun{}, ErrReceivableNotOpen
	}
	if err := tx.Commit(ctx); err != nil {
		return CollectionRun{}, fmt.Errorf("kasse: commit collection run: %w", err)
	}
	return saved, nil
}

// insertRunWithin writes the run row and its items inside an existing
// transaction, mapping the two unique constraints to their sentinel errors.
func insertRunWithin(ctx context.Context, tx pgx.Tx, run CollectionRun, fileXML string, items []CollectionRunItem) (CollectionRun, error) {
	saved, err := scanCollectionRun(tx.QueryRow(ctx,
		`INSERT INTO direct_debit_run
		   (reference, account_id, creditor_id, collection_date, sequence_type, local_instrument,
		    message_id, item_count, sum_cents, file_xml, file_hash, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 RETURNING `+collectionRunColumns,
		run.Reference, run.AccountID, run.CreditorID, run.CollectionDate, run.SequenceType,
		run.LocalInstrument, run.MessageID, run.ItemCount, run.SumCents, fileXML, run.FileHash, run.CreatedBy))
	if err != nil {
		if isUniqueViolationOn(err, "direct_debit_run_reference_key") {
			return CollectionRun{}, ErrRunReferenceExists
		}
		return CollectionRun{}, fmt.Errorf("kasse: insert collection run: %w", err)
	}

	for _, it := range items {
		if _, err := tx.Exec(ctx,
			`INSERT INTO direct_debit_run_item (run_id, mandate_reference, end_to_end_id, amount_cents, purpose)
			 VALUES ($1,$2,$3,$4,$5)`,
			saved.ID, it.MandateReference, it.EndToEndID, it.AmountCents, it.Purpose); err != nil {
			if isUniqueViolationOn(err, "direct_debit_run_item_mandate_key") {
				return CollectionRun{}, ErrDuplicateMandate
			}
			return CollectionRun{}, fmt.Errorf("kasse: insert collection run item: %w", err)
		}
	}
	return saved, nil
}
