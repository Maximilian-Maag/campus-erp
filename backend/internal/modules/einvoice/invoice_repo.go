package einvoice

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
)

const uniqueViolation = "23505"

// ErrNotFound is returned when an invoice does not exist.
var ErrNotFound = errors.New("einvoice: invoice not found")

// ErrNumberExists is returned when the invoice number is taken.
var ErrNumberExists = errors.New("einvoice: an invoice with that number already exists")

// InvoiceRepository is the persistence port for e-invoices.
type InvoiceRepository interface {
	Insert(ctx context.Context, inv core.Invoice) (core.Invoice, error)
	Get(ctx context.Context, id int64) (core.Invoice, error)
}

// InvoiceRepo is the PostgreSQL implementation.
type InvoiceRepo struct{ pool *pgxpool.Pool }

// NewInvoiceRepo returns an InvoiceRepo.
func NewInvoiceRepo(pool *pgxpool.Pool) *InvoiceRepo { return &InvoiceRepo{pool: pool} }

const invoiceColumns = `id, number, issue_date, type_code, currency, buyer_reference,
	payment_means_code, payee_iban, payment_terms, net_cents, vat_cents, gross_cents,
	created_by, created_at`

// Insert stores an invoice with its parties, lines and VAT breakdown atomically.
func (r *InvoiceRepo) Insert(ctx context.Context, inv core.Invoice) (core.Invoice, error) {
	net, vat, gross, groups := core.ComputeTotals(inv)
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return core.Invoice{}, fmt.Errorf("einvoice: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO e_invoice
		   (number, issue_date, type_code, currency, buyer_reference, payment_means_code,
		    payee_iban, payment_terms, net_cents, vat_cents, gross_cents, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		 RETURNING id`,
		inv.Number, inv.IssueDate, inv.TypeCode, inv.Currency, inv.BuyerReference,
		inv.PaymentMeansCode, inv.PayeeIBAN, inv.PaymentTerms, net, vat, gross, inv.CreatedBy).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
			return core.Invoice{}, ErrNumberExists
		}
		return core.Invoice{}, fmt.Errorf("einvoice: insert invoice: %w", err)
	}

	for _, p := range []struct {
		role  string
		party core.Party
	}{{"seller", inv.Seller}, {"buyer", inv.Buyer}} {
		if _, err := tx.Exec(ctx,
			`INSERT INTO e_invoice_party
			   (invoice_id, role, name, street, city, postal_code, country, vat_id,
			    endpoint_id, endpoint_scheme, email)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			id, p.role, p.party.Name, p.party.Street, p.party.City, p.party.PostalCode,
			p.party.Country, p.party.VatID, p.party.EndpointID, p.party.EndpointScheme, p.party.Email); err != nil {
			return core.Invoice{}, fmt.Errorf("einvoice: insert party: %w", err)
		}
	}

	for _, l := range inv.Lines {
		if _, err := tx.Exec(ctx,
			`INSERT INTO e_invoice_line
			   (invoice_id, line_no, description, quantity_milli, unit_code,
			    unit_price_cents, net_cents, vat_category, vat_rate_bp)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			id, l.LineNo, l.Description, l.QuantityMilli, l.UnitCode,
			l.UnitPriceCents, l.NetCents, l.VatCategory, l.VatRateBP); err != nil {
			return core.Invoice{}, fmt.Errorf("einvoice: insert line: %w", err)
		}
	}

	for _, g := range groups {
		if _, err := tx.Exec(ctx,
			`INSERT INTO e_invoice_vat (invoice_id, category, rate_bp, taxable_cents, tax_cents)
			 VALUES ($1,$2,$3,$4,$5)`,
			id, g.Category, g.RateBP, g.TaxableCents, g.TaxCents); err != nil {
			return core.Invoice{}, fmt.Errorf("einvoice: insert vat: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return core.Invoice{}, fmt.Errorf("einvoice: commit: %w", err)
	}
	inv.ID = id
	return inv, nil
}

// Get loads an invoice with its parties and lines.
func (r *InvoiceRepo) Get(ctx context.Context, id int64) (core.Invoice, error) {
	var inv core.Invoice
	var net, vat, gross int64
	err := r.pool.QueryRow(ctx, `SELECT `+invoiceColumns+` FROM e_invoice WHERE id = $1`, id).
		Scan(&inv.ID, &inv.Number, &inv.IssueDate, &inv.TypeCode, &inv.Currency, &inv.BuyerReference,
			&inv.PaymentMeansCode, &inv.PayeeIBAN, &inv.PaymentTerms, &net, &vat, &gross,
			&inv.CreatedBy, &inv.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return core.Invoice{}, fmt.Errorf("einvoice: invoice %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return core.Invoice{}, fmt.Errorf("einvoice: get invoice: %w", err)
	}

	parties, err := r.pool.Query(ctx,
		`SELECT role, name, street, city, postal_code, country, vat_id, endpoint_id, endpoint_scheme, email
		   FROM e_invoice_party WHERE invoice_id = $1`, id)
	if err != nil {
		return core.Invoice{}, fmt.Errorf("einvoice: get parties: %w", err)
	}
	defer parties.Close()
	for parties.Next() {
		var role string
		var p core.Party
		if err := parties.Scan(&role, &p.Name, &p.Street, &p.City, &p.PostalCode, &p.Country,
			&p.VatID, &p.EndpointID, &p.EndpointScheme, &p.Email); err != nil {
			return core.Invoice{}, fmt.Errorf("einvoice: scan party: %w", err)
		}
		if role == "seller" {
			inv.Seller = p
		} else {
			inv.Buyer = p
		}
	}
	if err := parties.Err(); err != nil {
		return core.Invoice{}, err
	}

	lines, err := r.pool.Query(ctx,
		`SELECT line_no, description, quantity_milli, unit_code, unit_price_cents, net_cents, vat_category, vat_rate_bp
		   FROM e_invoice_line WHERE invoice_id = $1 ORDER BY line_no`, id)
	if err != nil {
		return core.Invoice{}, fmt.Errorf("einvoice: get lines: %w", err)
	}
	defer lines.Close()
	for lines.Next() {
		var l core.Line
		if err := lines.Scan(&l.LineNo, &l.Description, &l.QuantityMilli, &l.UnitCode,
			&l.UnitPriceCents, &l.NetCents, &l.VatCategory, &l.VatRateBP); err != nil {
			return core.Invoice{}, fmt.Errorf("einvoice: scan line: %w", err)
		}
		inv.Lines = append(inv.Lines, l)
	}
	if err := lines.Err(); err != nil {
		return core.Invoice{}, err
	}
	return inv, nil
}
