package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// Every code is unique and shaped `<package>.<snake_case>`: a client branches on the code, so a
// duplicate would make two different refusals indistinguishable, and a code that is not shaped is
// one a client cannot rely on.
func TestDomainErrorCodesAreUniqueAndShaped(t *testing.T) {
	seen := make(map[string]error, len(domainErrorCodes))
	for _, dc := range domainErrorCodes {
		if dc.err == nil {
			t.Fatalf("a code without an error: %q", dc.code)
		}
		parts := strings.SplitN(dc.code, ".", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			t.Fatalf("code %q is not <package>.<name>", dc.code)
		}
		if parts[1] != strings.ToLower(parts[1]) || strings.Contains(parts[1], " ") {
			t.Fatalf("code %q is not snake_case", dc.code)
		}
		if prev, ok := seen[dc.code]; ok {
			t.Fatalf("code %q is claimed by both %v and %v", dc.code, prev, dc.err)
		}
		seen[dc.code] = dc.err
	}
	// A table that lost most of its entries would still pass the checks above.
	if len(seen) < 100 {
		t.Fatalf("only %d codes — the generated table looks truncated", len(seen))
	}
}

// A mapped error answers with its own code rather than the status's, and keeps the error's text.
func TestWriteDomainProblemUsesTheErrorsCode(t *testing.T) {
	rec := httptest.NewRecorder()
	writeDomainProblem(rec, http.StatusUnprocessableEntity,
		fmt.Errorf("kasse: mandate %q: %w", "MAN-1", kasse.ErrMandateNotActive))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Code != "kasse.mandate_not_active" {
		t.Fatalf("code = %q, want kasse.mandate_not_active", p.Code)
	}
	// The message is still the error's own text, so a log reader keeps the detail.
	if !strings.Contains(p.Error, "MAN-1") {
		t.Fatalf("message = %q, want the error's text", p.Error)
	}
}

// An error no handler maps falls back to the status's code, so the envelope always carries one.
func TestWriteDomainProblemFallsBackToTheStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	writeDomainProblem(rec, http.StatusConflict, errors.New("something else"))
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Code != "conflict" {
		t.Fatalf("code = %q, want conflict", p.Code)
	}
}

// A wrapped error still finds its code: handlers wrap domain errors with context, and the code must
// survive that.
func TestCodeForDomainErrorWalksWrappedErrors(t *testing.T) {
	wrapped := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", kasse.ErrMandateNotActive))
	if got := codeForDomainError(wrapped); got != "kasse.mandate_not_active" {
		t.Fatalf("wrapped code = %q", got)
	}
	if got := codeForDomainError(nil); got != "" {
		t.Fatalf("nil code = %q, want empty", got)
	}
	if got := codeForDomainError(errors.New("unmapped")); got != "" {
		t.Fatalf("unmapped code = %q, want empty", got)
	}
}
