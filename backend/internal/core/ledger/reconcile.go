package ledger

import (
	"sort"
	"strings"
)

// Reconciliation is the tie-back between the bookings of a period and the postings an
// export rendered from them. It is the check that makes the export trustworthy: every
// booking becomes exactly one posting, and nothing else appears.
type Reconciliation struct {
	// Matched counts the occurrences that appear on both sides.
	Matched int
	// Missing lists bookings that produced no posting (once per surplus).
	Missing []string
	// Extra lists postings that match no booking (once per surplus).
	Extra []string
}

// Balanced reports whether the export accounts for the bookings exactly.
func (r Reconciliation) Balanced() bool { return len(r.Missing) == 0 && len(r.Extra) == 0 }

// Reconcile compares the booking references of a period with the postings rendered from
// them. References are trimmed, each side is counted, and a surplus on either side is
// reported, so a duplicate cannot hide behind a matching reference.
func Reconcile(bookings []string, postings []Posting) Reconciliation {
	booked := map[string]int{}
	for _, b := range bookings {
		booked[strings.TrimSpace(b)]++
	}
	rendered := map[string]int{}
	for _, p := range postings {
		rendered[strings.TrimSpace(p.Reference)]++
	}

	r := Reconciliation{}
	for ref, n := range booked {
		m := rendered[ref]
		r.Matched += min(n, m)
		for i := 0; i < n-m; i++ {
			r.Missing = append(r.Missing, ref)
		}
	}
	for ref, m := range rendered {
		if n := booked[ref]; m > n {
			for i := 0; i < m-n; i++ {
				r.Extra = append(r.Extra, ref)
			}
		}
	}
	sort.Strings(r.Missing)
	sort.Strings(r.Extra)
	return r
}
