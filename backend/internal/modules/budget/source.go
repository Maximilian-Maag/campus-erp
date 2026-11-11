package budget

import (
	"errors"
	"strings"
)

// Where a position's money comes from and who administers it (docs/10).
//
// A **source** (Mittelquelle, a Haushaltsstelle) is where a title's money comes from — the state's
// chapter and title, a third-party funder, a foundation. A **centre** (Bewirtschaftungsstelle) is
// who administers it. Both mirror the vendor's budget_source / budget_center
// (docs/08-reuse-inventory.md §3); the vendor's two join tables are flattened onto the position,
// because at this size a position draws on one source and is administered by one centre.
//
// Neither is required: a position may exist before its funding is settled, and the
// Mittelüberwachung does not depend on either. Assigning one is a change to the budget's
// configuration, so it is audited like every other write.

// Errors of the source and the centre.
var (
	// ErrSourceCodeRequired is returned when a source names no code.
	ErrSourceCodeRequired = errors.New("budget: a source needs a code")
	// ErrSourceNameRequired is returned when a source names no name.
	ErrSourceNameRequired = errors.New("budget: a source needs a name")
	// ErrSourceExists is returned when a source with that code already exists.
	ErrSourceExists = errors.New("budget: a source with that code already exists")
	// ErrSourceNotFound is returned when a source does not exist.
	ErrSourceNotFound = errors.New("budget: budget source not found")
	// ErrCenterCodeRequired is returned when a centre names no code.
	ErrCenterCodeRequired = errors.New("budget: a centre needs a code")
	// ErrCenterNameRequired is returned when a centre names no name.
	ErrCenterNameRequired = errors.New("budget: a centre needs a name")
	// ErrCenterExists is returned when a centre with that code already exists.
	ErrCenterExists = errors.New("budget: a centre with that code already exists")
	// ErrCenterNotFound is returned when a centre does not exist.
	ErrCenterNotFound = errors.New("budget: budget centre not found")
	// ErrAssignmentUnknown is returned when an assignment names a source or a centre that does
	// not exist. The foreign key is the authority on what exists, so the refusal carries the
	// constraint that rejected it rather than guessing which half was wrong.
	ErrAssignmentUnknown = errors.New("budget: the assignment names an unknown source or centre")
)

// Source is a Mittelquelle: where a position's money comes from.
type Source struct {
	ID        int64
	Code      string
	Name      string
	Chapter   string
	Active    bool
	CreatedBy string
}

// Center is a Bewirtschaftungsstelle: who administers a position's money.
type Center struct {
	ID        int64
	Code      string
	Name      string
	Active    bool
	CreatedBy string
}

// NewSource validates and builds a source. The chapter is optional: a source may be a funder with
// no state chapter at all.
func NewSource(code, name, chapter string) (Source, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Source{}, ErrSourceCodeRequired
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Source{}, ErrSourceNameRequired
	}
	return Source{Code: code, Name: name, Chapter: strings.TrimSpace(chapter), Active: true}, nil
}

// NewCenter validates and builds a centre.
func NewCenter(code, name string) (Center, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return Center{}, ErrCenterCodeRequired
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Center{}, ErrCenterNameRequired
	}
	return Center{Code: code, Name: name, Active: true}, nil
}

// Named reports whether a position draws on a source or is administered by a centre.
func (p Position) Named() bool { return p.SourceID != nil || p.CenterID != nil }
