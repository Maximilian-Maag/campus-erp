package lookup

import core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"

// standardCatalogues returns the built-in code lists, seeded from the standards
// the productive source itself references (docs/08-reuse-inventory.md §4/§5):
// UNTDID 1001 (document type), UNTDID 4461 (payment means), UNTDID 5305 (VAT
// category), UNECE units of measure, ISO 4217 (currency) and ISO 3166-1 (country).
func standardCatalogues() []Value {
	var out []Value
	add := func(ns, code, short, long string, sort int) {
		out = append(out, Value{Namespace: ns, Code: code, ShortText: short, LongText: long, SortOrder: sort, Active: true})
	}
	// addCodes seeds a whole standard code list, keeping the standard's order.
	addCodes := func(ns string, codes []core.Code) {
		for i, c := range codes {
			add(ns, c.Code, c.ShortText, c.LongText, (i+1)*10)
		}
	}

	// UNTDID 1001 — invoice document type codes.
	add("document_type", "380", "Rechnung", "Commercial invoice", 10)
	add("document_type", "381", "Gutschrift", "Credit note", 20)
	add("document_type", "384", "Korrigierte Rechnung", "Corrected invoice", 30)
	add("document_type", "389", "Selbstausgestellte Rechnung", "Self-billed invoice", 40)

	// UNTDID 4461 — payment means, and UNTDID 5305 — VAT category. Both are generated from
	// the code lists core/einvoice validates against, so the catalogue an operator sees and the
	// codes the generator accepts are the same list by construction rather than by habit.
	addCodes("payment_means", core.PaymentMeansCodes())
	addCodes("vat_category", core.VATCategories())

	// UNECE Recommendation 20 — units of measure.
	add("unit_of_measure", "C62", "Stück", "one", 10)
	add("unit_of_measure", "HUR", "Stunde", "hour", 20)
	add("unit_of_measure", "DAY", "Tag", "day", 30)
	add("unit_of_measure", "KGM", "Kilogramm", "kilogram", 40)
	add("unit_of_measure", "MTR", "Meter", "metre", 50)
	add("unit_of_measure", "MTK", "Quadratmeter", "square metre", 60)
	add("unit_of_measure", "LTR", "Liter", "litre", 70)

	// ISO 4217 — currency.
	add("currency", "EUR", "Euro", "Euro", 10)
	add("currency", "CHF", "Schweizer Franken", "Swiss franc", 20)
	add("currency", "GBP", "Pfund Sterling", "Pound sterling", 30)
	add("currency", "USD", "US-Dollar", "US dollar", 40)

	// ISO 3166-1 alpha-2 — a small European subset.
	add("country", "DE", "Deutschland", "Germany", 10)
	add("country", "AT", "Österreich", "Austria", 20)
	add("country", "CH", "Schweiz", "Switzerland", 30)
	add("country", "FR", "Frankreich", "France", 40)
	add("country", "NL", "Niederlande", "Netherlands", 50)
	add("country", "IT", "Italien", "Italy", 60)
	add("country", "PL", "Polen", "Poland", 70)
	add("country", "GB", "Vereinigtes Königreich", "United Kingdom", 80)

	return out
}
