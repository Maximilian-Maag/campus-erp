package klr

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// serviceChargeReference names the records a charge wrote, so the report can show where a charged
// cost came from. It is its own prefix, so a reference always says which engine wrote it.
func serviceChargeReference(chargeID int64) string { return fmt.Sprintf("LEI:%d", chargeID) }

const serviceRateColumns = `id, cost_type_id, rate_cents, valid_from, valid_to, created_by`

func scanServiceRate(row pgx.Row) (ServiceRate, error) {
	var r ServiceRate
	err := row.Scan(&r.ID, &r.CostTypeID, &r.RateCents, &r.ValidFrom, &r.ValidTo, &r.CreatedBy)
	return r, err
}

// CreateServiceRate stores a rate.
func (r *Repo) CreateServiceRate(ctx context.Context, rate ServiceRate) (ServiceRate, error) {
	out, err := scanServiceRate(r.pool.QueryRow(ctx,
		`INSERT INTO service_rate (cost_type_id, rate_cents, valid_from, valid_to, created_by)
		 VALUES ($1,$2,$3,$4,$5) RETURNING `+serviceRateColumns,
		rate.CostTypeID, rate.RateCents, rate.ValidFrom, rate.ValidTo, rate.CreatedBy))
	if err != nil {
		return ServiceRate{}, fmt.Errorf("klr: create service rate: %w", err)
	}
	return out, nil
}

// ServiceRates lists every rate, oldest first, which is the order an operator reads them in.
func (r *Repo) ServiceRates(ctx context.Context) ([]ServiceRate, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+serviceRateColumns+` FROM service_rate ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("klr: list service rates: %w", err)
	}
	defer rows.Close()
	out := []ServiceRate{}
	for rows.Next() {
		rate, err := scanServiceRate(rows)
		if err != nil {
			return nil, fmt.Errorf("klr: scan service rate: %w", err)
		}
		out = append(out, rate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("klr: list service rates: %w", err)
	}
	return out, nil
}

// RateFor reads the rate in force for a cost type on a day. Where several cover the day the most
// recently started one wins, which is what makes a rate change take effect without deleting the
// old rate.
func (r *Repo) RateFor(ctx context.Context, costTypeID int64, on time.Time) (ServiceRate, error) {
	out, err := scanServiceRate(r.pool.QueryRow(ctx,
		`SELECT `+serviceRateColumns+` FROM service_rate
		  WHERE cost_type_id = $1 AND valid_from <= $2 AND (valid_to IS NULL OR valid_to >= $2)
		  ORDER BY valid_from DESC, id DESC LIMIT 1`, costTypeID, on))
	if errors.Is(err, pgx.ErrNoRows) {
		return ServiceRate{}, fmt.Errorf("klr: cost type %d on %s: %w",
			costTypeID, on.Format(time.DateOnly), ErrNoRateInForce)
	}
	if err != nil {
		return ServiceRate{}, fmt.Errorf("klr: rate for cost type: %w", err)
	}
	return out, nil
}

// RecordServiceCharge books a charge and the two records it produced in one transaction, so a
// charge is never half written: the receiver charged and the provider relieved, or neither.
func (r *Repo) RecordServiceCharge(ctx context.Context, charge ServiceCharge, records []Record) (ServiceCharge, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return ServiceCharge{}, fmt.Errorf("klr: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO service_charge (provider_center_id, receiver_center_id, cost_type_id, quantity, amount_cents, charged_on, charged_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		charge.ProviderCenterID, charge.ReceiverCenterID, charge.CostTypeID, charge.Quantity,
		charge.AmountCents, charge.ChargedOn, charge.ChargedBy).Scan(&id)
	if err != nil {
		return ServiceCharge{}, fmt.Errorf("klr: record service charge: %w", err)
	}

	reference := serviceChargeReference(id)
	for _, rec := range records {
		if _, err := tx.Exec(ctx,
			`INSERT INTO accounting_record (cost_center_id, cost_unit_id, kind, amount_cents, business_date, reference, booked_by)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			rec.CostCenterID, rec.CostUnitID, rec.Kind, rec.AmountCents, rec.BusinessDate,
			reference, rec.BookedBy); err != nil {
			return ServiceCharge{}, fmt.Errorf("klr: record service charge posting: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ServiceCharge{}, fmt.Errorf("klr: commit: %w", err)
	}
	charge.ID = id
	return charge, nil
}
