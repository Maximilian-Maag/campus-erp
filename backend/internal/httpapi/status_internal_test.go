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
