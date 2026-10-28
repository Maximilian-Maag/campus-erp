package kasse

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// This file wires the VAT catalogue (legacy kbs.mwst_art) into the ETL: the one
// populated table of the tax-office group, and a useful one because a rate alone
// cannot describe it (docs/legacy/kbs-tax-and-foundations.md). The mapping is the
// documented S7 one: percent -> integer basis points, the 1-char flag -> the
// enumerated kind, and no sign column.

// VATSink loads extracted rows into the canonical VAT catalogue via the tax
// service. It builds every value before writing, so a bad row fails the import
// without a partial load.
type VATSink struct {
	svc *TaxService
}

// NewVATSink wires the sink to the tax service.
func NewVATSink(svc *TaxService) VATSink { return VATSink{svc: svc} }

// Load implements etl.Sink.
func (s VATSink) Load(ctx context.Context, records []etl.Record, by string) (int, error) {
	values := make([]VATType, 0, len(records))
	for _, r := range records {
		rate, err := strconv.ParseInt(r.Fields["rate_basis_points"], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("VAT %q: rate_basis_points %q: %w", r.Fields["code"], r.Fields["rate_basis_points"], err)
		}
		v, err := NewVATType(r.Fields["code"], r.Fields["description"], rate, VATKind(r.Fields["kind"]))
		if err != nil {
			return 0, err
		}
		values = append(values, v)
	}
	n, err := s.svc.ImportVATTypes(ctx, values, by)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// LegacyVATSource reads kbs.mwst_art. The query must select, in order: the code,
// the description, the rate in percent as text (mw_kz::text, to avoid float
// rounding) and the 1-char kind flag (mw_typ).
func LegacyVATSource(pool *pgxpool.Pool, query string) etl.QuerySource {
	return etl.QuerySource{
		Pool: pool,
		SQL:  query,
		Scan: func(rows pgx.Rows) (etl.Record, error) {
			var code, description, rate, flag string
			if err := rows.Scan(&code, &description, &rate, &flag); err != nil {
				return etl.Record{}, err
			}
			basisPoints, err := percentToBasisPoints(rate)
			if err != nil {
				return etl.Record{}, fmt.Errorf("VAT %q: %w", code, err)
			}
			return etl.Record{
				Key: "vat/" + code,
				Fields: map[string]string{
					"code":              code,
					"description":       description,
					"rate_basis_points": strconv.FormatInt(basisPoints, 10),
					"kind":              string(vatKindFromLegacy(flag)),
				},
			}, nil
		},
	}
}

// percentToBasisPoints converts a decimal percent ("19.00", "7", "19.5") to
// integer basis points (1/100 %): 19.00 -> 1900. It truncates anything past the
// second decimal (the legacy numeric is unconstrained; a rate with finer
// precision than 1/100 % cannot be represented and is not expected).
func percentToBasisPoints(percent string) (int64, error) {
	percent = strings.TrimSpace(percent)
	negative := strings.HasPrefix(percent, "-")
	percent = strings.TrimLeft(percent, "+-")
	if percent == "" {
		return 0, fmt.Errorf("percent %q: empty", percent)
	}
	wholeText, fracText, _ := strings.Cut(percent, ".")
	// Fix the fraction to exactly two digits (hundredths of a percent): pad with
	// zeros, or truncate a finer value — no branch, so no spurious mutant.
	fracText = (fracText + "00")[:2]
	whole, err := strconv.ParseInt(wholeText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("percent %q: %w", percent, err)
	}
	frac, err := strconv.ParseInt(fracText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("percent %q: %w", percent, err)
	}
	basisPoints := whole*100 + frac
	if negative {
		basisPoints = -basisPoints
	}
	return basisPoints, nil
}

// vatKindFromLegacy maps the legacy 1-char flag to the canonical kind. The legacy
// flag is 'U' (Umsatzsteuer) or 'V' (Vorsteuer); many rows leave it unset.
// *(inferred)*: an unset or unrecognised flag defaults to output tax — the
// catalogue's majority case — because the canonical model requires a kind and
// most unset rows are output tax. A row that is really input tax (e.g. legacy
// code 67, "abziehbare Vorsteuer") will be misclassified until a production
// extract lets the mapping be reviewed.
func vatKindFromLegacy(flag string) VATKind {
	if strings.EqualFold(strings.TrimSpace(flag), "V") {
		return VATInput
	}
	return VATOutput
}
