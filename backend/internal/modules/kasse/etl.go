package kasse

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

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
// integer basis points (1/100 %): 19.00 -> 1900.
func percentToBasisPoints(percent string) (int64, error) { return hundredthsFromDecimal(percent) }

// hundredthsFromDecimal parses a legacy decimal ("19.00", "-7", "100", "19.5")
// into whole hundredths (×100), truncating past the second decimal. It is the one
// scaling helper behind both VAT basis points and cash amounts in cents, since
// both are hundredths of their unit. The legacy numeric is unconstrained; finer
// precision than 1/100 is not representable and not expected.
func hundredthsFromDecimal(s string) (int64, error) {
	s = strings.TrimSpace(s)
	negative := strings.HasPrefix(s, "-")
	s = strings.TrimLeft(s, "+-")
	if s == "" {
		return 0, fmt.Errorf("decimal %q: empty", s)
	}
	wholeText, fracText, _ := strings.Cut(s, ".")
	// Fix the fraction to exactly two digits: pad with zeros or truncate a finer
	// value — no branch, so no spurious mutant.
	fracText = (fracText + "00")[:2]
	whole, err := strconv.ParseInt(wholeText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decimal %q: %w", s, err)
	}
	frac, err := strconv.ParseInt(fracText, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decimal %q: %w", s, err)
	}
	n := whole*100 + frac
	if negative {
		n = -n
	}
	return n, nil
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

// boolFromLegacyFlag interprets a legacy 1-char carry-over flag. The legacy column
// is an unconstrained char(1) and the Muster dump is empty, so the exact encoding
// is unverified. *(inferred)*: the common German flags "J"/"Y"/"1"/"T" mean yes;
// anything else (including empty) means no.
func boolFromLegacyFlag(flag string) bool {
	switch strings.ToUpper(strings.TrimSpace(flag)) {
	case "J", "Y", "1", "T":
		return true
	default:
		return false
	}
}

// TaxOfficeSink loads extracted rows into the canonical tax-office table via the
// tax service; it builds every value before writing, so a bad row fails the import
// without a partial load.
type TaxOfficeSink struct {
	svc *TaxService
}

// NewTaxOfficeSink wires the sink to the tax service.
func NewTaxOfficeSink(svc *TaxService) TaxOfficeSink { return TaxOfficeSink{svc: svc} }

// Load implements etl.Sink.
func (s TaxOfficeSink) Load(ctx context.Context, records []etl.Record, by string) (int, error) {
	values := make([]TaxOffice, 0, len(records))
	for _, r := range records {
		year, err := strconv.Atoi(r.Fields["year"])
		if err != nil {
			return 0, fmt.Errorf("tax office %q: year %q: %w", r.Fields["number"], r.Fields["year"], err)
		}
		t, err := NewTaxOffice(year, r.Fields["number"], r.Fields["name"], r.Fields["street"],
			r.Fields["postal_code"], r.Fields["city"], r.Fields["account_holder"], r.Fields["iban"], r.Fields["bic"])
		if err != nil {
			return 0, err
		}
		values = append(values, t)
	}
	n, err := s.svc.ImportTaxOffices(ctx, values, by)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// LegacyTaxOfficeSource reads kbs.finanzamt. The query must select, in order:
// jahr, finanzamt_nr, bezeichnung, str, plz, ort, inhaber, iban, bic.
func LegacyTaxOfficeSource(pool *pgxpool.Pool, query string) etl.QuerySource {
	return etl.QuerySource{
		Pool: pool,
		SQL:  query,
		Scan: func(rows pgx.Rows) (etl.Record, error) {
			var year int
			var number, name, street, postal, city, holder, iban, bic string
			if err := rows.Scan(&year, &number, &name, &street, &postal, &city, &holder, &iban, &bic); err != nil {
				return etl.Record{}, err
			}
			return etl.Record{
				Key: fmt.Sprintf("tax-office/%d/%s", year, number),
				Fields: map[string]string{
					"year": strconv.Itoa(year), "number": number, "name": name, "street": street,
					"postal_code": postal, "city": city, "account_holder": holder, "iban": iban, "bic": bic,
				},
			}, nil
		},
	}
}

// FoundationSink loads extracted rows into the canonical foundation table.
type FoundationSink struct {
	svc *TaxService
}

// NewFoundationSink wires the sink to the tax service.
func NewFoundationSink(svc *TaxService) FoundationSink { return FoundationSink{svc: svc} }

// Load implements etl.Sink.
func (s FoundationSink) Load(ctx context.Context, records []etl.Record, by string) (int, error) {
	values := make([]Foundation, 0, len(records))
	for _, r := range records {
		year, err := strconv.Atoi(r.Fields["year"])
		if err != nil {
			return 0, fmt.Errorf("foundation %q: year %q: %w", r.Fields["number"], r.Fields["year"], err)
		}
		f, err := NewFoundation(year, r.Fields["number"], r.Fields["description"], r.Fields["carry_over"] == "true")
		if err != nil {
			return 0, err
		}
		values = append(values, f)
	}
	n, err := s.svc.ImportFoundations(ctx, values, by)
	if err != nil {
		return 0, err
	}
	return n, nil
}

// LegacyFoundationSource reads kbs.stift. The query must select, in order:
// jahr, stift_nr, bez, kz_uebern.
func LegacyFoundationSource(pool *pgxpool.Pool, query string) etl.QuerySource {
	return etl.QuerySource{
		Pool: pool,
		SQL:  query,
		Scan: func(rows pgx.Rows) (etl.Record, error) {
			var year int
			var number, description, carry string
			if err := rows.Scan(&year, &number, &description, &carry); err != nil {
				return etl.Record{}, err
			}
			return etl.Record{
				Key: fmt.Sprintf("foundation/%d/%s", year, number),
				Fields: map[string]string{
					"year": strconv.Itoa(year), "number": number, "description": description,
					"carry_over": strconv.FormatBool(boolFromLegacyFlag(carry)),
				},
			}, nil
		},
	}
}

// centsAndKindFromLegacyAmount parses a legacy counter-book amount and derives the
// booking kind. *(inferred)*: the sign decides — positive is income, negative is
// expense — because no booking-type code list exists in kbs
// (docs/legacy/kbs-cashbook.md). A zero amount has no kind and is rejected.
func centsAndKindFromLegacyAmount(amount string) (int64, BookingKind, error) {
	cents, err := hundredthsFromDecimal(amount)
	if err != nil {
		return 0, "", err
	}
	if cents == 0 {
		return 0, "", fmt.Errorf("amount %q: zero has no kind", amount)
	}
	if cents < 0 {
		return -cents, Expense, nil
	}
	return cents, Income, nil
}

// CounterBookSink loads extracted counter-book rows into the cash book via the
// cash-book service, grouped by business day.
type CounterBookSink struct {
	svc *Service
}

// NewCounterBookSink wires the sink to the cash-book service.
func NewCounterBookSink(svc *Service) CounterBookSink { return CounterBookSink{svc: svc} }

// Load implements etl.Sink. It validates each record, groups by business date and
// imports each day through the service (one audited write per day).
func (s CounterBookSink) Load(ctx context.Context, records []etl.Record, by string) (int, error) {
	byDate := map[string][]CounterBookEntry{}
	for _, r := range records {
		cents, err := strconv.ParseInt(r.Fields["amount_cents"], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("counter book %q: amount_cents %q: %w", r.Key, r.Fields["amount_cents"], err)
		}
		d := r.Fields["business_date"]
		byDate[d] = append(byDate[d], CounterBookEntry{
			LegacyRef:   r.Fields["legacy_ref"],
			Kind:        BookingKind(r.Fields["kind"]),
			AmountCents: cents,
			Purpose:     r.Fields["purpose"],
		})
	}
	dates := make([]string, 0, len(byDate))
	for d := range byDate {
		dates = append(dates, d)
	}
	sort.Strings(dates) // deterministic day order
	total := 0
	for _, d := range dates {
		day, err := time.Parse(time.DateOnly, d)
		if err != nil {
			return 0, fmt.Errorf("counter book: business_date %q: %w", d, err)
		}
		n, err := s.svc.ImportCounterBook(ctx, day, byDate[d], by)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// LegacyCounterBookSource reads kbs.schalterbuch. The query must select, in order:
// scha_dat, betrag (as text, to avoid float rounding), grund1, grund2, scha_nr,
// jahr. Kind comes from the amount sign and the purpose from grund1+grund2; the
// legacy reference (table/year/number) keys the idempotent upsert.
func LegacyCounterBookSource(pool *pgxpool.Pool, query string) etl.QuerySource {
	return etl.QuerySource{
		Pool: pool,
		SQL:  query,
		Scan: func(rows pgx.Rows) (etl.Record, error) {
			var date time.Time
			var betrag, grund1, grund2, jahr string
			var schaNr int
			if err := rows.Scan(&date, &betrag, &grund1, &grund2, &schaNr, &jahr); err != nil {
				return etl.Record{}, err
			}
			cents, kind, err := centsAndKindFromLegacyAmount(betrag)
			if err != nil {
				return etl.Record{}, fmt.Errorf("counter book %d/%s: %w", schaNr, jahr, err)
			}
			purpose := strings.TrimSpace(strings.TrimSpace(grund1) + " " + strings.TrimSpace(grund2))
			if purpose == "" {
				purpose = "(ohne Buchungstext)"
			}
			ref := fmt.Sprintf("kbs.schalterbuch/%s/%d", strings.TrimSpace(jahr), schaNr)
			return etl.Record{
				Key: ref,
				Fields: map[string]string{
					"business_date": date.Format(time.DateOnly),
					"kind":          string(kind),
					"amount_cents":  strconv.FormatInt(cents, 10),
					"purpose":       purpose,
					"legacy_ref":    ref,
				},
			}, nil
		},
	}
}
