// Package kasse is the Kassenbuchung module (legacy cash-book schema).
//
// Implemented: the cash-book day lifecycle, payment orders with four-eyes release,
// the payment run and SEPA credit-transfer export (pain.001), direct-debit mandates
// and the collection run (pain.008), bank-statement import and reconciliation
// (MT940, CAMT.052/053/054), dunning, foundation books, VAT/tax-office data.
//
// Open: the return postings to FiBu (the rueckfibu_* format) and the EBICS
// transport (an interface, tracked in internal/platform/interfaces).
package kasse
