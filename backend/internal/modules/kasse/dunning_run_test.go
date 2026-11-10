package kasse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// ── fakes ───────────────────────────────────────────────────────────────────

type fakeLevels struct {
	levels []DunningLevel
	err    error
}

func (f fakeLevels) Levels(context.Context, int) ([]DunningLevel, error) {
	return f.levels, f.err
}

type fakeNotices struct {
	byClaim map[string]DunningNotice
	err     error
}

func (f fakeNotices) LatestNoticeByClaim(_ context.Context, claim string) (DunningNotice, bool, error) {
	if f.err != nil {
		return DunningNotice{}, false, f.err
	}
	n, ok := f.byClaim[claim]
	return n, ok, nil
}

type fakeReceivables struct {
	items []Receivable
	err   error
}

func (f fakeReceivables) OpenReceivablesDueBy(context.Context, time.Time) ([]Receivable, error) {
	return f.items, f.err
}

type fakeIssuer struct {
	calls   []string
	err     error
	charge  int64
	docKey  string
	notices []DunningNotice
}

func (f *fakeIssuer) Issue(_ context.Context, _ int, levelCode, claimReference string, _ int64, _ string) (DunningNotice, error) {
	f.calls = append(f.calls, levelCode+"|"+claimReference)
	if f.err != nil {
		return DunningNotice{}, f.err
	}
	n := DunningNotice{ID: int64(len(f.calls)), ClaimReference: claimReference, LevelCode: levelCode,
		ChargeCents: f.charge, DocumentKey: f.docKey}
	f.notices = append(f.notices, n)
	return n, nil
}

func day(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func chainLevel(code, next string, days int) DunningLevel {
	return DunningLevel{Year: 2026, Code: code, NextCode: next, DaysAllowed: days, Active: true,
		Bands: []DunningBand{{UpToCents: 0, FeeCents: 500}}}
}

// ── chain ───────────────────────────────────────────────────────────────────

func TestBuildDunningChain(t *testing.T) {
	t.Run("walks a chain", func(t *testing.T) {
		c, err := buildDunningChain([]DunningLevel{
			chainLevel("1", "2", 14), chainLevel("2", "3", 14), chainLevel("3", "", 14),
		})
		if err != nil {
			t.Fatalf("buildDunningChain: %v", err)
		}
		if c.head.Code != "1" {
			t.Fatalf("head = %q, want 1", c.head.Code)
		}
		if len(c.levels) != 3 {
			t.Fatalf("levels = %d, want 3", len(c.levels))
		}
	})

	t.Run("a next code naming no level ends the chain", func(t *testing.T) {
		// A level may be configured before its successor exists; that ends the
		// chain, it does not make the configuration unusable.
		c, err := buildDunningChain([]DunningLevel{chainLevel("1", "2", 14)})
		if err != nil {
			t.Fatalf("buildDunningChain: %v", err)
		}
		if c.head.Code != "1" {
			t.Fatalf("head = %q, want 1", c.head.Code)
		}
	})

	t.Run("no levels", func(t *testing.T) {
		if _, err := buildDunningChain(nil); !errors.Is(err, ErrDunningChainHead) {
			t.Fatalf("err = %v, want ErrDunningChainHead", err)
		}
	})

	t.Run("several heads", func(t *testing.T) {
		_, err := buildDunningChain([]DunningLevel{chainLevel("1", "", 14), chainLevel("2", "", 14)})
		if !errors.Is(err, ErrDunningChainHead) {
			t.Fatalf("err = %v, want ErrDunningChainHead", err)
		}
	})

	t.Run("cycle has no head", func(t *testing.T) {
		_, err := buildDunningChain([]DunningLevel{chainLevel("1", "2", 14), chainLevel("2", "1", 14)})
		if !errors.Is(err, ErrDunningChainHead) {
			t.Fatalf("err = %v, want ErrDunningChainHead", err)
		}
	})

	t.Run("self loop leaves its level unreachable", func(t *testing.T) {
		_, err := buildDunningChain([]DunningLevel{chainLevel("1", "1", 14), chainLevel("2", "", 14)})
		if !errors.Is(err, ErrDunningChainBroken) {
			t.Fatalf("err = %v, want ErrDunningChainBroken", err)
		}
	})

	t.Run("a component the head cannot reach is broken", func(t *testing.T) {
		// 1 -> 2 -> end is reachable from the single head; 8 -> 9 -> 8 is a cycle
		// no head points into, so the chain does not cover every level.
		_, err := buildDunningChain([]DunningLevel{
			chainLevel("1", "2", 14), chainLevel("2", "", 14),
			chainLevel("8", "9", 14), chainLevel("9", "8", 14),
		})
		if !errors.Is(err, ErrDunningChainBroken) {
			t.Fatalf("err = %v, want ErrDunningChainBroken", err)
		}
	})

	t.Run("a reassignment cycle off the head is broken", func(t *testing.T) {
		// 1 -> 2 -> 3 -> 2: two heads would be needed for this even to be reachable,
		// so the walk must detect the revisit rather than loop forever.
		_, err := buildDunningChain([]DunningLevel{
			chainLevel("1", "2", 14), chainLevel("2", "3", 14), chainLevel("3", "2", 14),
		})
		if !errors.Is(err, ErrDunningChainHead) && !errors.Is(err, ErrDunningChainBroken) {
			t.Fatalf("err = %v, want a chain error", err)
		}
	})
}

func TestValidateDunningRun(t *testing.T) {
	cases := []struct {
		name string
		year int
		asOf time.Time
		want error
	}{
		{"ok", 2026, day("2026-01-01"), nil},
		{"earliest year", 1900, day("1900-01-01"), nil},
		{"latest year", 2999, day("2999-12-31"), nil},
		{"year low", 1899, day("2026-01-01"), ErrRunYearInvalid},
		{"year high", 3000, day("2026-01-01"), ErrRunYearInvalid},
		{"no date", 2026, time.Time{}, ErrRunDateRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateDunningRun(c.year, c.asOf); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestReceivableClaimReference(t *testing.T) {
	if got := ReceivableClaimReference(42); got != "receivable/42" {
		t.Fatalf("reference = %q", got)
	}
}

// ── the run ─────────────────────────────────────────────────────────────────

func newRunService(levels []DunningLevel, recs []Receivable, notices map[string]DunningNotice,
	issuer *fakeIssuer) (*DunningRunService, *fakeAudit) {

	sink := &fakeAudit{}
	svc := NewDunningRunService(fakeLevels{levels: levels}, fakeNotices{byClaim: notices},
		fakeReceivables{items: recs}, issuer, sink)
	return svc, sink
}

func openItem(id int64, due string) Receivable {
	return Receivable{ID: id, MandateReference: "MAN-1", AmountCents: 10000,
		Currency: "EUR", DueDate: day(due), Purpose: "Semesterbeitrag", State: ReceivableOpen}
}

func TestDunningRunIssuesTheFirstLevelWhenDue(t *testing.T) {
	issuer := &fakeIssuer{charge: 500, docKey: "dunning/1.pdf"}
	svc, sink := newRunService(
		[]DunningLevel{chainLevel("1", "2", 14), chainLevel("2", "", 14)},
		[]Receivable{openItem(7, "2026-01-01")},
		nil, issuer)

	got, err := svc.Run(context.Background(), 2026, day("2026-01-15"), "alice")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Issued != 1 || got.Skipped != 0 || got.Failed != 0 {
		t.Fatalf("counts = %d/%d/%d, want 1/0/0", got.Issued, got.Skipped, got.Failed)
	}
	if len(got.Outcomes) != 1 {
		t.Fatalf("outcomes = %d", len(got.Outcomes))
	}
	o := got.Outcomes[0]
	if !o.Issued || o.LevelCode != "1" || o.NoticeID != 1 || o.ChargeCents != 500 ||
		o.DocumentKey != "dunning/1.pdf" || o.ClaimReference != "receivable/7" || o.Reason != "" {
		t.Fatalf("outcome = %+v", o)
	}
	if len(issuer.calls) != 1 || issuer.calls[0] != "1|receivable/7" {
		t.Fatalf("issuer calls = %v", issuer.calls)
	}
	if got.AsOf.Format(time.DateOnly) != "2026-01-15" {
		t.Fatalf("as-of = %v", got.AsOf)
	}
	if len(sink.events) != 1 || sink.events[0].Action != "dunning.run" ||
		!strings.Contains(sink.events[0].Subject, "dunning-run/2026/2026-01-15") ||
		!strings.Contains(sink.events[0].After, "issued=1") {
		t.Fatalf("audit = %+v", sink.events)
	}
}

func TestDunningRunAdvancesFromTheLatestNotice(t *testing.T) {
	issuer := &fakeIssuer{charge: 700}
	svc, _ := newRunService(
		[]DunningLevel{chainLevel("1", "2", 14), chainLevel("2", "", 7)},
		[]Receivable{openItem(7, "2026-01-01")},
		map[string]DunningNotice{"receivable/7": {ID: 3, ClaimReference: "receivable/7",
			LevelCode: "1", IssuedOn: day("2026-01-10")}},
		issuer)

	got, err := svc.Run(context.Background(), 2026, day("2026-01-25"), "alice")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Issued != 1 || got.Outcomes[0].LevelCode != "2" {
		t.Fatalf("outcome = %+v", got.Outcomes[0])
	}
	if issuer.calls[0] != "2|receivable/7" {
		t.Fatalf("issuer calls = %v", issuer.calls)
	}
}

func TestDunningRunSkips(t *testing.T) {
	cases := []struct {
		name    string
		levels  []DunningLevel
		due     string
		asOf    string
		notices map[string]DunningNotice
		reason  string
	}{
		{
			name:   "not due",
			levels: []DunningLevel{chainLevel("1", "", 14)},
			due:    "2026-01-01", asOf: "2026-01-10",
			reason: DunningSkipNotDue,
		},
		{
			name:   "already dunned on the as-of date",
			levels: []DunningLevel{chainLevel("1", "2", 0), chainLevel("2", "", 0)},
			due:    "2026-01-01", asOf: "2026-02-01",
			notices: map[string]DunningNotice{"receivable/7": {LevelCode: "1", IssuedOn: day("2026-02-01")}},
			reason:  DunningSkipAlreadyDunned,
		},
		{
			name:   "a notice from after the as-of date",
			levels: []DunningLevel{chainLevel("1", "2", 0), chainLevel("2", "", 0)},
			due:    "2026-01-01", asOf: "2026-02-01",
			notices: map[string]DunningNotice{"receivable/7": {LevelCode: "1", IssuedOn: day("2026-03-01")}},
			reason:  DunningSkipAlreadyDunned,
		},
		{
			name:   "latest level is not in the chain",
			levels: []DunningLevel{chainLevel("1", "2", 0), chainLevel("2", "", 0)},
			due:    "2026-01-01", asOf: "2026-02-01",
			notices: map[string]DunningNotice{"receivable/7": {LevelCode: "9", IssuedOn: day("2026-01-10")}},
			reason:  DunningSkipUnknownLevel,
		},
		{
			name:   "no next level",
			levels: []DunningLevel{chainLevel("1", "2", 0), chainLevel("2", "", 0)},
			due:    "2026-01-01", asOf: "2026-05-01",
			notices: map[string]DunningNotice{"receivable/7": {LevelCode: "2", IssuedOn: day("2026-02-01")}},
			reason:  DunningSkipNoNextLevel,
		},
		{
			name:   "next level missing",
			levels: []DunningLevel{chainLevel("1", "2", 0)},
			due:    "2026-01-01", asOf: "2026-05-01",
			notices: map[string]DunningNotice{"receivable/7": {LevelCode: "1", IssuedOn: day("2026-02-01")}},
			reason:  DunningSkipNextLevelMissing,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			issuer := &fakeIssuer{charge: 500}
			svc, sink := newRunService(c.levels, []Receivable{openItem(7, c.due)}, c.notices, issuer)
			got, err := svc.Run(context.Background(), 2026, day(c.asOf), "alice")
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if got.Issued != 0 || got.Skipped != 1 || got.Failed != 0 {
				t.Fatalf("counts = %d/%d/%d, want 0/1/0", got.Issued, got.Skipped, got.Failed)
			}
			if got.Outcomes[0].Reason != c.reason {
				t.Fatalf("reason = %q, want %q", got.Outcomes[0].Reason, c.reason)
			}
			if got.Outcomes[0].Issued {
				t.Fatal("a skipped claim must not be issued")
			}
			if len(issuer.calls) != 0 {
				t.Fatalf("issuer was called: %v", issuer.calls)
			}
			if len(sink.events) != 1 || !strings.Contains(sink.events[0].After, "skipped=1") {
				t.Fatalf("audit = %+v", sink.events)
			}
		})
	}
}

func TestDunningRunContinuesAfterAFailedIssue(t *testing.T) {
	issuer := &fakeIssuer{err: errors.New("document render failed")}
	svc, _ := newRunService(
		[]DunningLevel{chainLevel("1", "", 0)},
		[]Receivable{openItem(7, "2026-01-01"), openItem(8, "2026-01-02")},
		nil, issuer)

	got, err := svc.Run(context.Background(), 2026, day("2026-02-01"), "alice")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Issued != 0 || got.Failed != 2 || got.Skipped != 0 {
		t.Fatalf("counts = %d/%d/%d, want 0/0/2", got.Issued, got.Skipped, got.Failed)
	}
	for _, o := range got.Outcomes {
		if o.Reason != DunningSkipIssueFailed || o.Issued {
			t.Fatalf("outcome = %+v", o)
		}
		if !strings.Contains(o.Detail, "document render failed") {
			t.Fatalf("detail = %q", o.Detail)
		}
	}
}

func TestDunningRunErrors(t *testing.T) {
	levels := []DunningLevel{chainLevel("1", "", 0)}
	sink := &fakeAudit{}
	issuer := &fakeIssuer{}

	t.Run("actor required", func(t *testing.T) {
		svc := NewDunningRunService(fakeLevels{levels: levels}, fakeNotices{}, fakeReceivables{}, issuer, sink)
		if _, err := svc.Run(context.Background(), 2026, day("2026-01-01"), " "); !errors.Is(err, ErrActorRequired) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad year", func(t *testing.T) {
		svc := NewDunningRunService(fakeLevels{levels: levels}, fakeNotices{}, fakeReceivables{}, issuer, sink)
		if _, err := svc.Run(context.Background(), 1800, day("2026-01-01"), "alice"); !errors.Is(err, ErrRunYearInvalid) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("no levels", func(t *testing.T) {
		svc := NewDunningRunService(fakeLevels{}, fakeNotices{}, fakeReceivables{}, issuer, sink)
		if _, err := svc.Run(context.Background(), 2026, day("2026-01-01"), "alice"); !errors.Is(err, ErrDunningChainHead) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("level read fails", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewDunningRunService(fakeLevels{err: boom}, fakeNotices{}, fakeReceivables{}, issuer, sink)
		if _, err := svc.Run(context.Background(), 2026, day("2026-01-01"), "alice"); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("receivable read fails", func(t *testing.T) {
		boom := errors.New("boom")
		svc := NewDunningRunService(fakeLevels{levels: levels}, fakeNotices{}, fakeReceivables{err: boom}, issuer, sink)
		if _, err := svc.Run(context.Background(), 2026, day("2026-01-01"), "alice"); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("claim history unreadable aborts", func(t *testing.T) {
		boom := errors.New("history down")
		svc := NewDunningRunService(fakeLevels{levels: levels}, fakeNotices{err: boom},
			fakeReceivables{items: []Receivable{openItem(7, "2026-01-01")}}, issuer, sink)
		if _, err := svc.Run(context.Background(), 2026, day("2026-02-01"), "alice"); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("audit fails", func(t *testing.T) {
		boom := errors.New("audit down")
		svc := NewDunningRunService(fakeLevels{levels: levels}, fakeNotices{}, fakeReceivables{}, issuer,
			failingAudit{err: boom})
		if _, err := svc.Run(context.Background(), 2026, day("2026-01-01"), "alice"); !errors.Is(err, boom) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestDunningRunEmptyReport(t *testing.T) {
	svc, _ := newRunService([]DunningLevel{chainLevel("1", "", 0)}, nil, nil, &fakeIssuer{})
	got, err := svc.Run(context.Background(), 2026, day("2026-01-01"), "alice")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got.Outcomes == nil || len(got.Outcomes) != 0 {
		t.Fatalf("outcomes = %#v, want an empty slice", got.Outcomes)
	}
}
