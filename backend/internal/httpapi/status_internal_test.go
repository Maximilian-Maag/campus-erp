package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
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
		{"bank account required", kasse.ErrBankAccountRequired, http.StatusUnprocessableEntity},
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

func TestStatusForStatement(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", kasse.ErrStatementNotFound, http.StatusNotFound},
		{"empty", kasse.ErrStatementEmpty, http.StatusUnprocessableEntity},
		{"invalid", kasse.ErrStatementInvalid, http.StatusUnprocessableEntity},
		{"account required", kasse.ErrBankAccountRequired, http.StatusUnprocessableEntity},
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
