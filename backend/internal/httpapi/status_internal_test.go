package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	budgetfile "github.com/Maximilian-Maag/campus-erp/backend/internal/core/budgetimport"
	core "github.com/Maximilian-Maag/campus-erp/backend/internal/core/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/budget"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/einvoice"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/lookup"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/procurement"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

func TestStatusFor(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", kasse.ErrNotFound, http.StatusNotFound},
		{"wrapped not found", fmt.Errorf("day 1: %w", kasse.ErrNotFound), http.StatusNotFound},
		{"day exists", kasse.ErrDayExists, http.StatusConflict},
		{"day closed", kasse.ErrDayClosed, http.StatusConflict},
		{"amount not positive", kasse.ErrAmountNotPositive, http.StatusUnprocessableEntity},
		{"purpose required", kasse.ErrPurposeRequired, http.StatusUnprocessableEntity},
		{"unknown kind", kasse.ErrUnknownKind, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
		{"nil", nil, http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusFor(c.err); got != c.want {
				t.Fatalf("statusFor(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForOrder(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", kasse.ErrNotFound, http.StatusNotFound},
		{"four eyes is forbidden", kasse.ErrFourEyes, http.StatusForbidden},
		{"wrapped four eyes", fmt.Errorf("approve: %w", kasse.ErrFourEyes), http.StatusForbidden},
		{"reference exists", kasse.ErrReferenceExists, http.StatusConflict},
		{"order conflict", kasse.ErrOrderConflict, http.StatusConflict},
		{"not draft", kasse.ErrNotDraft, http.StatusConflict},
		{"not submitted", kasse.ErrNotSubmitted, http.StatusConflict},
		{"not cancellable", kasse.ErrNotCancellable, http.StatusConflict},
		{"amount not positive", kasse.ErrAmountNotPositive, http.StatusUnprocessableEntity},
		{"purpose required", kasse.ErrPurposeRequired, http.StatusUnprocessableEntity},
		{"bad currency", kasse.ErrBadCurrency, http.StatusUnprocessableEntity},
		{"bad year", kasse.ErrBadYear, http.StatusUnprocessableEntity},
		{"reference required", kasse.ErrReferenceRequired, http.StatusUnprocessableEntity},
		{"recipient required", kasse.ErrRecipientRequired, http.StatusUnprocessableEntity},
		{"budget position required", kasse.ErrBudgetPositionRequired, http.StatusUnprocessableEntity},
		{"due date required", kasse.ErrDueDateRequired, http.StatusUnprocessableEntity},
		{"reason required", kasse.ErrReasonRequired, http.StatusUnprocessableEntity},
		{"allocation negative", kasse.ErrAllocationNegative, http.StatusUnprocessableEntity},
		{"over budget", budget.ErrBudgetExceeded, http.StatusUnprocessableEntity},
		{"wrapped over budget", fmt.Errorf("approve: %w", budget.ErrBudgetExceeded), http.StatusUnprocessableEntity},
		{"actor required", kasse.ErrActorRequired, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForOrder(c.err); got != c.want {
				t.Fatalf("statusForOrder(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForDunning(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", kasse.ErrNotFound, http.StatusNotFound},
		{"inactive level", kasse.ErrLevelInactive, http.StatusConflict},
		{"level exists", kasse.ErrLevelExists, http.StatusConflict},
		{"code required", kasse.ErrLevelCodeRequired, http.StatusUnprocessableEntity},
		{"bands required", kasse.ErrBandsRequired, http.StatusUnprocessableEntity},
		{"bands not ascending", kasse.ErrBandsNotAscending, http.StatusUnprocessableEntity},
		{"open band not last", kasse.ErrOpenBandNotLast, http.StatusUnprocessableEntity},
		{"negative band", kasse.ErrNegativeBand, http.StatusUnprocessableEntity},
		{"claim required", kasse.ErrClaimRequired, http.StatusUnprocessableEntity},
		{"actor required", kasse.ErrActorRequired, http.StatusUnprocessableEntity},
		{"amount not positive", kasse.ErrAmountNotPositive, http.StatusUnprocessableEntity},
		{"no band", kasse.ErrNoDunningBand, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForDunning(c.err); got != c.want {
				t.Fatalf("statusForDunning(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForTax(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", kasse.ErrNotFound, http.StatusNotFound},
		{"vat code exists", kasse.ErrVATCodeExists, http.StatusConflict},
		{"tax office exists", kasse.ErrTaxOfficeExists, http.StatusConflict},
		{"foundation exists", kasse.ErrFoundationExists, http.StatusConflict},
		{"code required", kasse.ErrVATCodeRequired, http.StatusUnprocessableEntity},
		{"code too long", kasse.ErrVATCodeTooLong, http.StatusUnprocessableEntity},
		{"rate out of range", kasse.ErrVATRateOutOfRange, http.StatusUnprocessableEntity},
		{"kind unknown", kasse.ErrVATKindUnknown, http.StatusUnprocessableEntity},
		{"iban invalid", kasse.ErrIBANInvalid, http.StatusUnprocessableEntity},
		{"number required", kasse.ErrNumberRequired, http.StatusUnprocessableEntity},
		{"name required", kasse.ErrNameRequired, http.StatusUnprocessableEntity},
		{"bad year", kasse.ErrBadYear, http.StatusUnprocessableEntity},
		{"actor required", kasse.ErrActorRequired, http.StatusUnprocessableEntity},
		{"amount negative", kasse.ErrAmountNegative, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForTax(c.err); got != c.want {
				t.Fatalf("statusForTax(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForRun(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"run not found", kasse.ErrRunNotFound, http.StatusNotFound},
		{"not found", kasse.ErrNotFound, http.StatusNotFound},
		{"wrapped not found", fmt.Errorf("run 1: %w", kasse.ErrNotFound), http.StatusNotFound},
		{"reference exists", kasse.ErrRunReferenceExists, http.StatusConflict},
		{"order already paid", kasse.ErrOrderAlreadyPaid, http.StatusConflict},
		{"reference required", kasse.ErrRunReferenceRequired, http.StatusUnprocessableEntity},
		{"no orders", kasse.ErrNoOrders, http.StatusUnprocessableEntity},
		{"order not approved", kasse.ErrOrderNotApproved, http.StatusUnprocessableEntity},
		{"cash account required", kasse.ErrCashAccountRequired, http.StatusUnprocessableEntity},
		{"execution date required", kasse.ErrExecutionDateRequired, http.StatusUnprocessableEntity},
		{"iban invalid", kasse.ErrIBANInvalid, http.StatusUnprocessableEntity},
		{"bic invalid", kasse.ErrBICInvalid, http.StatusUnprocessableEntity},
		{"bad currency", kasse.ErrBadCurrency, http.StatusUnprocessableEntity},
		{"recipient required", kasse.ErrRecipientRequired, http.StatusUnprocessableEntity},
		{"actor required", kasse.ErrActorRequired, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForRun(c.err); got != c.want {
				t.Fatalf("statusForRun(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForFoundationBook(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"book not found", kasse.ErrFoundationBookNotFound, http.StatusNotFound},
		{"not found", kasse.ErrNotFound, http.StatusNotFound},
		{"book exists", kasse.ErrFoundationBookExists, http.StatusConflict},
		{"reference required", kasse.ErrFoundationBookReferenceRequired, http.StatusUnprocessableEntity},
		{"foundation required", kasse.ErrFoundationRequired, http.StatusUnprocessableEntity},
		{"amount not positive", kasse.ErrAmountNotPositive, http.StatusUnprocessableEntity},
		{"purpose required", kasse.ErrPurposeRequired, http.StatusUnprocessableEntity},
		{"unknown kind", kasse.ErrUnknownKind, http.StatusUnprocessableEntity},
		{"bad currency", kasse.ErrBadCurrency, http.StatusUnprocessableEntity},
		{"actor required", kasse.ErrActorRequired, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForFoundationBook(c.err); got != c.want {
				t.Fatalf("statusForFoundationBook(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForEInvoice(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", einvoice.ErrNotFound, http.StatusNotFound},
		{"number exists", einvoice.ErrNumberExists, http.StatusConflict},
		{"actor required", einvoice.ErrActorRequired, http.StatusUnprocessableEntity},
		{"number required", core.ErrNumberRequired, http.StatusUnprocessableEntity},
		{"issue date required", core.ErrIssueDateRequired, http.StatusUnprocessableEntity},
		{"currency", core.ErrCurrency, http.StatusUnprocessableEntity},
		{"seller", core.ErrSellerRequired, http.StatusUnprocessableEntity},
		{"buyer", core.ErrBuyerRequired, http.StatusUnprocessableEntity},
		{"party name", core.ErrPartyNameRequired, http.StatusUnprocessableEntity},
		{"party city", core.ErrPartyCityRequired, http.StatusUnprocessableEntity},
		{"party zip", core.ErrPartyPostalCode, http.StatusUnprocessableEntity},
		{"party country", core.ErrPartyCountry, http.StatusUnprocessableEntity},
		{"seller vat", core.ErrSellerVATRequired, http.StatusUnprocessableEntity},
		{"seller email", core.ErrSellerEmailRequired, http.StatusUnprocessableEntity},
		{"buyer reference", core.ErrBuyerReference, http.StatusUnprocessableEntity},
		{"payment means", core.ErrPaymentMeansRequired, http.StatusUnprocessableEntity},
		{"payee iban", core.ErrPayeeIBANRequired, http.StatusUnprocessableEntity},
		{"no lines", core.ErrNoLines, http.StatusUnprocessableEntity},
		{"line description", core.ErrLineDescription, http.StatusUnprocessableEntity},
		{"line quantity", core.ErrLineQuantity, http.StatusUnprocessableEntity},
		{"line amount", core.ErrLineAmount, http.StatusUnprocessableEntity},
		{"vat category", core.ErrVATCategoryRequired, http.StatusUnprocessableEntity},
		{"line net mismatch", core.ErrLineNetMismatch, http.StatusUnprocessableEntity},
		{"malformed document", core.ErrMalformed, http.StatusUnprocessableEntity},
		{"bad value", core.ErrBadValue, http.StatusUnprocessableEntity},
		{"already matched", einvoice.ErrAlreadyMatched, http.StatusConflict},
		{"not inbound", einvoice.ErrNotInbound, http.StatusUnprocessableEntity},
		{"no order matcher", einvoice.ErrNoOrderMatcher, http.StatusInternalServerError},
		{"order not found", procurement.ErrOrderNotFound, http.StatusNotFound},
		{"order state", procurement.ErrBadTransition, http.StatusConflict},
		{"amount mismatch", procurement.ErrMatchAmountMismatch, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForEInvoice(c.err); got != c.want {
				t.Fatalf("statusForEInvoice(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForBudget(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"position not found", budget.ErrPositionNotFound, http.StatusNotFound},
		{"position exists", budget.ErrPositionExists, http.StatusConflict},
		{"budget exceeded", budget.ErrBudgetExceeded, http.StatusConflict},
		{"actor required", budget.ErrActorRequired, http.StatusUnprocessableEntity},
		{"year required", budget.ErrYearRequired, http.StatusUnprocessableEntity},
		{"chapter required", budget.ErrChapterRequired, http.StatusUnprocessableEntity},
		{"title required", budget.ErrTitleRequired, http.StatusUnprocessableEntity},
		{"allocated negative", budget.ErrAllocatedNegative, http.StatusUnprocessableEntity},
		{"amount not positive", budget.ErrAmountNotPositive, http.StatusUnprocessableEntity},
		{"unknown kind", budget.ErrUnknownKind, http.StatusUnprocessableEntity},
		{"source not found", budget.ErrSourceNotFound, http.StatusNotFound},
		{"center not found", budget.ErrCenterNotFound, http.StatusNotFound},
		{"source exists", budget.ErrSourceExists, http.StatusConflict},
		{"center exists", budget.ErrCenterExists, http.StatusConflict},
		{"assignment unknown", budget.ErrAssignmentUnknown, http.StatusUnprocessableEntity},
		{"unknown import layout", budgetfile.ErrUnknownLayout, http.StatusUnprocessableEntity},
		{"empty import file", budgetfile.ErrEmpty, http.StatusUnprocessableEntity},
		{"unreadable import row", budgetfile.ErrBadRow, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForBudget(c.err); got != c.want {
				t.Fatalf("statusForBudget(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForLookup(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", lookup.ErrNotFound, http.StatusNotFound},
		{"actor required", lookup.ErrActorRequired, http.StatusUnprocessableEntity},
		{"namespace required", lookup.ErrNamespaceRequired, http.StatusUnprocessableEntity},
		{"code required", lookup.ErrCodeRequired, http.StatusUnprocessableEntity},
		{"short text required", lookup.ErrShortTextRequired, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForLookup(c.err); got != c.want {
				t.Fatalf("statusForLookup(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForProcurement(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", procurement.ErrOrderNotFound, http.StatusNotFound},
		{"exists", procurement.ErrOrderExists, http.StatusConflict},
		{"bad transition", procurement.ErrBadTransition, http.StatusConflict},
		{"budget exceeded", budget.ErrBudgetExceeded, http.StatusConflict},
		{"actor required", procurement.ErrActorRequired, http.StatusUnprocessableEntity},
		{"number required", procurement.ErrNumberRequired, http.StatusUnprocessableEntity},
		{"supplier required", procurement.ErrSupplierRequired, http.StatusUnprocessableEntity},
		{"budget position required", procurement.ErrBudgetPositionRequired, http.StatusUnprocessableEntity},
		{"currency", procurement.ErrCurrency, http.StatusUnprocessableEntity},
		{"no lines", procurement.ErrNoLines, http.StatusUnprocessableEntity},
		{"line description", procurement.ErrLineDescription, http.StatusUnprocessableEntity},
		{"line quantity", procurement.ErrLineQuantity, http.StatusUnprocessableEntity},
		{"line amount", procurement.ErrLineAmount, http.StatusUnprocessableEntity},
		{"vat category", procurement.ErrVATCategoryRequired, http.StatusUnprocessableEntity},
		{"line net mismatch", procurement.ErrLineNetMismatch, http.StatusUnprocessableEntity},
		{"receipt not found", procurement.ErrReceiptNotFound, http.StatusNotFound},
		{"receipt exists", procurement.ErrReceiptExists, http.StatusConflict},
		{"receipt reference required", procurement.ErrReceiptReferenceRequired, http.StatusUnprocessableEntity},
		{"receipt date required", procurement.ErrReceiptDateRequired, http.StatusUnprocessableEntity},
		{"no receipt lines", procurement.ErrNoReceiptLines, http.StatusUnprocessableEntity},
		{"receipt quantity", procurement.ErrReceiptQuantity, http.StatusUnprocessableEntity},
		{"receipt line unknown", procurement.ErrReceiptLineUnknown, http.StatusUnprocessableEntity},
		{"receipt line duplicate", procurement.ErrReceiptLineDuplicate, http.StatusUnprocessableEntity},
		{"over receipt", procurement.ErrOverReceipt, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForProcurement(c.err); got != c.want {
				t.Fatalf("statusForProcurement(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForKLR(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"centre not found", klr.ErrCostCenterNotFound, http.StatusNotFound},
		{"unit not found", klr.ErrCostUnitNotFound, http.StatusNotFound},
		{"centre exists", klr.ErrCostCenterExists, http.StatusConflict},
		{"unit exists", klr.ErrCostUnitExists, http.StatusConflict},
		{"actor required", klr.ErrActorRequired, http.StatusUnprocessableEntity},
		{"number required", klr.ErrNumberRequired, http.StatusUnprocessableEntity},
		{"name required", klr.ErrNameRequired, http.StatusUnprocessableEntity},
		{"amount not positive", klr.ErrAmountNotPositive, http.StatusUnprocessableEntity},
		{"unknown kind", klr.ErrUnknownKind, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForKLR(c.err); got != c.want {
				t.Fatalf("statusForKLR(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForStatement(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", kasse.ErrStatementNotFound, http.StatusNotFound},
		{"bank file not found", kasse.ErrBankFileNotFound, http.StatusNotFound},
		{"empty", kasse.ErrStatementEmpty, http.StatusUnprocessableEntity},
		{"invalid", kasse.ErrStatementInvalid, http.StatusUnprocessableEntity},
		{"account required", kasse.ErrCashAccountRequired, http.StatusUnprocessableEntity},
		{"actor required", kasse.ErrActorRequired, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForStatement(c.err); got != c.want {
				t.Fatalf("statusForStatement(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForMandate(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", kasse.ErrMandateNotFound, http.StatusNotFound},
		{"duplicate", kasse.ErrMandateReferenceExists, http.StatusConflict},
		{"already retired", kasse.ErrMandateRetired, http.StatusConflict},
		{"reference required", kasse.ErrMandateReferenceRequired, http.StatusUnprocessableEntity},
		{"reference too long", kasse.ErrMandateReferenceTooLong, http.StatusUnprocessableEntity},
		{"name required", kasse.ErrNameRequired, http.StatusUnprocessableEntity},
		{"bad iban", kasse.ErrIBANInvalid, http.StatusUnprocessableEntity},
		{"bad bic", kasse.ErrDebtorBICInvalid, http.StatusUnprocessableEntity},
		{"signature required", kasse.ErrSignatureDateRequired, http.StatusUnprocessableEntity},
		{"bad sequence", kasse.ErrSequenceTypeUnknown, http.StatusUnprocessableEntity},
		{"actor required", kasse.ErrActorRequired, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForMandate(c.err); got != c.want {
				t.Fatalf("statusForMandate(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForCollectionRun(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", kasse.ErrCollectionRunNotFound, http.StatusNotFound},
		{"duplicate reference", kasse.ErrRunReferenceExists, http.StatusConflict},
		{"reference required", kasse.ErrRunReferenceRequired, http.StatusUnprocessableEntity},
		{"account required", kasse.ErrCashAccountRequired, http.StatusUnprocessableEntity},
		{"creditor id required", kasse.ErrCollectionCreditorIDRequired, http.StatusUnprocessableEntity},
		{"date required", kasse.ErrCollectionDateRequired, http.StatusUnprocessableEntity},
		{"bad sequence", kasse.ErrSequenceTypeUnknown, http.StatusUnprocessableEntity},
		{"bad local instrument", kasse.ErrLocalInstrumentUnknown, http.StatusUnprocessableEntity},
		{"no items", kasse.ErrNoCollectionItems, http.StatusUnprocessableEntity},
		{"blank mandate", kasse.ErrMandateReferenceRequired, http.StatusUnprocessableEntity},
		{"duplicate mandate", kasse.ErrDuplicateMandate, http.StatusUnprocessableEntity},
		{"end-to-end required", kasse.ErrEndToEndIDRequired, http.StatusUnprocessableEntity},
		{"amount", kasse.ErrAmountNotPositive, http.StatusUnprocessableEntity},
		{"inactive mandate", kasse.ErrMandateNotActive, http.StatusUnprocessableEntity},
		{"sequence mismatch", kasse.ErrMandateSequenceMismatch, http.StatusUnprocessableEntity},
		{"mandate not found", kasse.ErrMandateNotFound, http.StatusUnprocessableEntity},
		{"actor required", kasse.ErrActorRequired, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForCollectionRun(c.err); got != c.want {
				t.Fatalf("statusForCollectionRun(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}

func TestStatusForETL(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"unknown dataset", etl.ErrUnknownDataset, http.StatusNotFound},
		{"actor required", etl.ErrActorRequired, http.StatusUnprocessableEntity},
		{"reconcile", etl.ErrReconcile, http.StatusUnprocessableEntity},
		{"anything else", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := statusForETL(c.err); got != c.want {
				t.Fatalf("statusForETL(%v) = %d, want %d", c.err, got, c.want)
			}
		})
	}
}
