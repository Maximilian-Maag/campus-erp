// Package interfaces is the registry of the external systems campus-erp talks to
// (requirements §5, IR-1..IR-8). It makes the interface surface explicit and
// reviewable: every interface has a direction, a format and a status, and the
// registry is validated, so a new interface cannot be added without declaring
// what it is and where it stands. The bundle analysed in docs/14 is the source
// for the vendor's own interfaces (bank, KfW, chip terminal, SOAP services).
package interfaces

import (
	"fmt"
	"slices"
	"strings"
)

// Direction is the data flow of an interface.
type Direction string

// Status is how far the interface is implemented.
type Status string

// Statuses an interface can be in.
const (
	Out         Direction = "out"
	In          Direction = "in"
	Both        Direction = "both"
	Implemented Status    = "implemented"
	Planned     Status    = "planned"
)

// Interface is one external interface.
type Interface struct {
	Key         string    // unique, stable key
	Requirement string    // IR-1..IR-8 (requirements §5)
	Name        string    // human name
	Direction   Direction // out / in / both
	Format      string    // the binding format(s)
	Status      Status    // implemented / planned
	Reference   string    // evidence (doc or the vendor artefact)
}

// Registry is a validated set of interfaces.
type Registry struct {
	byKey map[string]Interface
	list  []Interface
}

// Standard returns the registry seeded with the interfaces known today. Entries
// marked implemented correspond to shipped code; planned ones are tracked work.
func Standard() *Registry {
	r, err := New([]Interface{
		{Key: "bank-sepa-sct", Requirement: "IR-2", Name: "Bank — SEPA credit transfer", Direction: Out, Format: "pain.001.001.09 (GBIC/DK)", Status: Implemented, Reference: "docs/legacy/kbs-sepa-export.md"},
		{Key: "bank-sepa-sdd", Requirement: "IR-2", Name: "Bank — SEPA direct debit", Direction: Out, Format: "pain.008.001.08", Status: Implemented, Reference: "docs/20-sepa-direct-debit.md, docs/22-collection-runs.md"},
		{Key: "bank-mt940", Requirement: "IR-2", Name: "Bank — statement import (MT940)", Direction: In, Format: "SWIFT MT940", Status: Implemented, Reference: "docs/legacy/kbs-statement-import.md"},
		{Key: "bank-camt052", Requirement: "IR-2", Name: "Bank — account report (CAMT.052)", Direction: In, Format: "ISO 20022 camt.052.001.08", Status: Implemented, Reference: "docs/legacy/kbs-statement-import.md"},
		{Key: "bank-camt053", Requirement: "IR-2", Name: "Bank — statement import (CAMT.053)", Direction: In, Format: "ISO 20022 camt.053.001.08", Status: Implemented, Reference: "docs/legacy/kbs-statement-import.md"},
		{Key: "bank-camt054", Requirement: "IR-2", Name: "Bank — notification import (CAMT.054)", Direction: In, Format: "ISO 20022 camt.054.001.08", Status: Implemented, Reference: "docs/legacy/kbs-statement-import.md"},
		{Key: "bank-ebics", Requirement: "IR-2", Name: "Bank — EBICS channel", Direction: Both, Format: "EBICS", Status: Planned, Reference: "docs/14 §4.3"},
		{Key: "einvoice-out", Requirement: "IR-7", Name: "E-invoice — outbound XRechnung", Direction: Out, Format: "XRechnung 3.0 (UBL 2.1)", Status: Implemented, Reference: "docs/09-e-invoicing.md"},
		{Key: "einvoice-in", Requirement: "IR-7", Name: "E-invoice — inbound XRechnung", Direction: In, Format: "XRechnung 3.0 (UBL 2.1)", Status: Implemented, Reference: "docs/09-e-invoicing.md"},
		{Key: "idp-oidc", Requirement: "IR-5", Name: "Identity provider — OIDC", Direction: In, Format: "OIDC (JWT/JWKS)", Status: Implemented, Reference: "docs/16-http-transport.md"},
		{Key: "lbv-payoffice", Requirement: "IR-1", Name: "State pay office (LBV)", Direction: Out, Format: "per-Bundesland file", Status: Planned, Reference: "requirements §5"},
		{Key: "elster-eric", Requirement: "IR-3", Name: "Finanzamt — ELSTER/ERiC", Direction: Out, Format: "ERiC", Status: Planned, Reference: "requirements §5"},
		{Key: "statistics", Requirement: "IR-4", Name: "Statistics offices", Direction: Out, Format: "per-state formats", Status: Planned, Reference: "requirements §5"},
		{Key: "dms", Requirement: "IR-6", Name: "DMS / document store", Direction: Both, Format: "own store + connector", Status: Planned, Reference: "requirements §5"},
		{Key: "erp-soap", Requirement: "IR-7", Name: "ERP SOAP services", Direction: Both, Format: "SOAP (79 services)", Status: Planned, Reference: "docs/14 §3"},
		{Key: "kfw-loan", Requirement: "IR-7", Name: "KfW — student loan SOAP", Direction: Out, Format: "SOAP (SBwsService)", Status: Planned, Reference: "docs/14 §4.2"},
		{Key: "card-terminal", Requirement: "IR-7", Name: "Chip-card / cash terminal (BSOS)", Direction: Both, Format: "BSOS (TCP bridge)", Status: Planned, Reference: "docs/14 §4.1"},
		{Key: "hochschulstart", Requirement: "IR-8", Name: "hochschulstart.de / DoSV", Direction: Both, Format: "DoSV", Status: Planned, Reference: "requirements §5"},
	})
	if err != nil {
		// The seed above is a compile-time constant; a violation is a bug.
		panic("interfaces: standard registry invalid: " + err.Error())
	}
	return r
}

// New validates and stores a set of interfaces.
func New(items []Interface) (*Registry, error) {
	r := &Registry{byKey: make(map[string]Interface, len(items)), list: append([]Interface(nil), items...)}
	for _, it := range items {
		if err := validate(it); err != nil {
			return nil, err
		}
		if _, dup := r.byKey[it.Key]; dup {
			return nil, fmt.Errorf("interfaces: duplicate key %q", it.Key)
		}
		r.byKey[it.Key] = it
	}
	slices.SortFunc(r.list, func(a, b Interface) int { return strings.Compare(a.Key, b.Key) })
	return r, nil
}

func validate(it Interface) error {
	switch {
	case it.Key == "":
		return fmt.Errorf("interfaces: empty key")
	case it.Name == "":
		return fmt.Errorf("interfaces: %q: empty name", it.Key)
	case it.Requirement == "":
		return fmt.Errorf("interfaces: %q: empty requirement", it.Key)
	case it.Format == "":
		return fmt.Errorf("interfaces: %q: empty format", it.Key)
	case it.Direction != Out && it.Direction != In && it.Direction != Both:
		return fmt.Errorf("interfaces: %q: invalid direction %q", it.Key, it.Direction)
	case it.Status != Implemented && it.Status != Planned:
		return fmt.Errorf("interfaces: %q: invalid status %q", it.Key, it.Status)
	}
	return nil
}

// All returns the interfaces, ordered by key.
func (r *Registry) All() []Interface { return r.list }

// Get returns one interface by key.
func (r *Registry) Get(key string) (Interface, bool) {
	it, ok := r.byKey[key]
	return it, ok
}

// Count returns how many interfaces have the given status.
func (r *Registry) Count(status Status) int {
	n := 0
	for _, it := range r.list {
		if it.Status == status {
			n++
		}
	}
	return n
}

// Render returns a stable, human-readable table used as a golden file.
func (r *Registry) Render() string {
	var b strings.Builder
	b.WriteString("key | requirement | direction | status | name | format\n")
	for _, it := range r.list {
		fmt.Fprintf(&b, "%s | %s | %s | %s | %s | %s\n",
			it.Key, it.Requirement, it.Direction, it.Status, it.Name, it.Format)
	}
	return b.String()
}
