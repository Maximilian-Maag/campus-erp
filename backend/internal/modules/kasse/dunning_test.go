package kasse

import (
	"context"
	"errors"
	"testing"
	"time"
)

func band(upTo, fee, bp int64) DunningBand {
	return DunningBand{UpToCents: upTo, FeeCents: fee, RateBasisPoints: bp}
}

func validLevel() DunningLevel {
	return DunningLevel{
		Year:        2026,
		Code:        "1",
		Description: "1. Mahnung",
		DaysAllowed: 14,
		Active:      true,
		NextCode:    "2",
		Bands:       []DunningBand{band(5000, 500, 0), band(50000, 1500, 100), band(0, 2500, 200)},
	}
}

func TestDunningLevelValidate(t *testing.T) {
	cases := []struct {
		name   string
		want   error
		mutate func(*DunningLevel)
	}{
		{"blank code", ErrLevelCodeRequired, func(l *DunningLevel) { l.Code = "  " }},
		{"no bands", ErrBandsRequired, func(l *DunningLevel) { l.Bands = nil }},
		{"negative days", ErrNegativeBand, func(l *DunningLevel) { l.DaysAllowed = -1 }},
		{"negative fee", ErrNegativeBand, func(l *DunningLevel) { l.Bands = []DunningBand{band(100, -1, 0)} }},
		{"negative rate", ErrNegativeBand, func(l *DunningLevel) { l.Bands = []DunningBand{band(100, 0, -1)} }},
		{"negative bound", ErrNegativeBand, func(l *DunningLevel) { l.Bands = []DunningBand{band(-1, 0, 0)} }},
		{"descending", ErrBandsNotAscending, func(l *DunningLevel) {
			l.Bands = []DunningBand{band(5000, 0, 0), band(1000, 0, 0)}
		}},
		{"duplicate bound", ErrBandsNotAscending, func(l *DunningLevel) {
			l.Bands = []DunningBand{band(5000, 0, 0), band(5000, 0, 0)}
		}},
		{"open band not last", ErrOpenBandNotLast, func(l *DunningLevel) {
			l.Bands = []DunningBand{band(0, 0, 0), band(5000, 0, 0)}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := validLevel()
			c.mutate(&l)
			if err := l.Validate(); !errors.Is(err, c.want) {
				t.Fatalf("want %v, got %v", c.want, err)
			}
		})
	}
	if err := validLevel().Validate(); err != nil {
		t.Fatalf("a valid level must pass: %v", err)
	}
	// A level with no grace period is legal: only negative days are refused.
	immediate := validLevel()
	immediate.DaysAllowed = 0
	if err := immediate.Validate(); err != nil {
		t.Fatalf("zero days must be accepted, got %v", err)
	}
	// A band bound of exactly 1 cent is legal: the ascending check must start
	// below the smallest representable bound.
	tiny := validLevel()
	tiny.Bands = []DunningBand{band(1, 0, 0), band(2, 0, 0)}
	if err := tiny.Validate(); err != nil {
		t.Fatalf("a bound of 1 cent must be accepted, got %v", err)
	}
}

func TestRoundDiv(t *testing.T) {
	cases := []struct {
		n, div, want int64
	}{
		{0, 10000, 0},
		{4999, 10000, 0}, // just below half
		{5000, 10000, 1}, // exactly half rounds up
		{5001, 10000, 1}, // just above half
		{9999, 10000, 1},
		{10000, 10000, 1}, // exact
		{12550, 10000, 1}, // 1.255
		{15000, 10000, 2}, // 1.5 rounds up
		{25000, 10000, 3}, // 2.5 rounds up
	}
	for _, c := range cases {
		if got := roundDiv(c.n, c.div); got != c.want {
			t.Fatalf("roundDiv(%d, %d) = %d, want %d", c.n, c.div, got, c.want)
		}
	}
}

func TestSelectBand(t *testing.T) {
	l := validLevel()
	cases := []struct {
		amount int64
		want   int64
	}{
		{1, 5000},      // smallest band
		{5000, 5000},   // upper bound is inclusive
		{5001, 50000},  // next band up
		{50000, 50000}, // its upper bound is inclusive
		{50001, 0},     // open-ended band
		{999999, 0},    // still the open-ended band
	}
	for _, c := range cases {
		got, err := l.SelectBand(c.amount)
		if err != nil {
			t.Fatalf("SelectBand(%d): %v", c.amount, err)
		}
		if got.UpToCents != c.want {
			t.Fatalf("SelectBand(%d) = band up to %d, want %d", c.amount, got.UpToCents, c.want)
		}
	}

	if _, err := l.SelectBand(0); !errors.Is(err, ErrAmountNotPositive) {
		t.Fatalf("zero amount: want ErrAmountNotPositive, got %v", err)
	}
	if _, err := l.SelectBand(-5); !errors.Is(err, ErrAmountNotPositive) {
		t.Fatalf("negative amount: want ErrAmountNotPositive, got %v", err)
	}

	// A level whose bands do not reach the amount has no band for it.
	gapped := DunningLevel{Bands: []DunningBand{band(1000, 0, 0), band(2000, 0, 0)}}
	if _, err := gapped.SelectBand(5000); !errors.Is(err, ErrNoDunningBand) {
		t.Fatalf("want ErrNoDunningBand, got %v", err)
	}
}

func TestCharge(t *testing.T) {
	cases := []struct {
		name   string
		bands  []DunningBand
		amount int64
		want   int64
	}{
		{"fee only", []DunningBand{band(5000, 500, 0)}, 1000, 500},
		{"fee plus two percent", []DunningBand{band(0, 500, 200)}, 12000, 740},
		{"rounds half up", []DunningBand{band(0, 0, 100)}, 12550, 126},
		{"rounds down below half", []DunningBand{band(0, 0, 100)}, 12549, 125},
		{"exact division", []DunningBand{band(0, 0, 100)}, 10000, 100},
		{"zero rate zero fee", []DunningBand{band(0, 0, 0)}, 9999, 0},
		{"band selection affects the charge", []DunningBand{band(1000, 100, 0), band(0, 900, 0)}, 5000, 900},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DunningLevel{Bands: c.bands}.Charge(c.amount)
			if err != nil {
				t.Fatalf("Charge: %v", err)
			}
			if got != c.want {
				t.Fatalf("Charge(%d) = %d, want %d", c.amount, got, c.want)
			}
		})
	}

	if _, err := (DunningLevel{Bands: []DunningBand{band(100, 0, 0)}}).Charge(5000); !errors.Is(err, ErrNoDunningBand) {
		t.Fatalf("want ErrNoDunningBand, got %v", err)
	}
	if _, err := (DunningLevel{Bands: []DunningBand{band(100, 0, 0)}}).Charge(0); !errors.Is(err, ErrAmountNotPositive) {
		t.Fatalf("want ErrAmountNotPositive, got %v", err)
	}
}

func TestDueDate(t *testing.T) {
	l := DunningLevel{DaysAllowed: 14}
	from := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if got := l.DueDate(from); !got.Equal(time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("DueDate = %s, want 2026-03-15", got.Format(time.DateOnly))
	}
	if got := (DunningLevel{DaysAllowed: 0}).DueDate(from); !got.Equal(from) {
		t.Fatalf("zero days must not move the date")
	}
}

func TestNewDunningNotice(t *testing.T) {
	l := validLevel()
	issued := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	n, err := NewDunningNotice(l, "  F-2026-001  ", 12000, issued, "alice")
	if err != nil {
		t.Fatalf("NewDunningNotice: %v", err)
	}
	if n.ClaimReference != "F-2026-001" {
		t.Fatalf("claim = %q, want it trimmed", n.ClaimReference)
	}
	if n.LevelCode != "1" || n.OutstandingCents != 12000 {
		t.Fatalf("notice = %+v", n)
	}
	// 12000 falls into the 50000 band: fee 1500 + 1% of 12000 = 1620.
	if n.ChargeCents != 1620 {
		t.Fatalf("charge = %d, want 1620", n.ChargeCents)
	}

	if _, err := NewDunningNotice(l, "   ", 12000, issued, "alice"); !errors.Is(err, ErrClaimRequired) {
		t.Fatalf("blank claim: want ErrClaimRequired, got %v", err)
	}
	if _, err := NewDunningNotice(l, "F-1", 12000, issued, "  "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("blank actor: want ErrActorRequired, got %v", err)
	}

	inactive := l
	inactive.Active = false
	if _, err := NewDunningNotice(inactive, "F-1", 12000, issued, "alice"); !errors.Is(err, ErrLevelInactive) {
		t.Fatalf("inactive level: want ErrLevelInactive, got %v", err)
	}
	if _, err := NewDunningNotice(l, "F-1", 0, issued, "alice"); !errors.Is(err, ErrAmountNotPositive) {
		t.Fatalf("zero outstanding: want ErrAmountNotPositive, got %v", err)
	}
}

// ── service level ───────────────────────────────────────────────────────────

type fakeDunningRepo struct {
	level       DunningLevel
	notice      DunningNotice
	getErr      error
	createErr   error
	insertErr   error
	documentErr error
	documentKey string
	documentID  int64
}

func (f *fakeDunningRepo) CreateLevel(_ context.Context, l DunningLevel) (DunningLevel, error) {
	if f.createErr != nil {
		return DunningLevel{}, f.createErr
	}
	l.ID = 3
	f.level = l
	return l, nil
}

func (f *fakeDunningRepo) GetLevel(context.Context, int, string) (DunningLevel, error) {
	if f.getErr != nil {
		return DunningLevel{}, f.getErr
	}
	return f.level, nil
}

func (f *fakeDunningRepo) InsertNotice(_ context.Context, n DunningNotice) (DunningNotice, error) {
	if f.insertErr != nil {
		return DunningNotice{}, f.insertErr
	}
	n.ID = 9
	return n, nil
}

func (f *fakeDunningRepo) GetNotice(_ context.Context, id int64) (DunningNotice, error) {
	if f.getErr != nil {
		return DunningNotice{}, f.getErr
	}
	n := f.notice
	n.ID = id
	return n, nil
}

func (f *fakeDunningRepo) SetNoticeDocument(_ context.Context, id int64, key string) error {
	if f.documentErr != nil {
		return f.documentErr
	}
	f.documentID, f.documentKey = id, key
	return nil
}

func TestDunningServiceCreateLevel(t *testing.T) {
	ctx := context.Background()
	repo := &fakeDunningRepo{}
	sink := &fakeAudit{}
	svc := NewDunningService(repo, sink)

	saved, err := svc.CreateLevel(ctx, validLevel(), "alice")
	if err != nil {
		t.Fatalf("CreateLevel: %v", err)
	}
	if saved.ID != 3 {
		t.Fatalf("id = %d", saved.ID)
	}
	if len(sink.events) != 1 || sink.events[0].Action != "dunning.level.create" {
		t.Fatalf("audit = %+v", sink.events)
	}

	// validation happens before the repository is touched
	if _, err := svc.CreateLevel(ctx, DunningLevel{Code: "x"}, "alice"); !errors.Is(err, ErrBandsRequired) {
		t.Fatalf("want ErrBandsRequired, got %v", err)
	}
	if _, err := svc.CreateLevel(ctx, validLevel(), "  "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("blank actor: want ErrActorRequired, got %v", err)
	}

	boom := errors.New("boom")
	if _, err := NewDunningService(&fakeDunningRepo{createErr: boom}, &fakeAudit{}).CreateLevel(ctx, validLevel(), "alice"); !errors.Is(err, boom) {
		t.Fatalf("repo error: want boom, got %v", err)
	}
	if _, err := NewDunningService(repo, failingAudit{err: boom}).CreateLevel(ctx, validLevel(), "alice"); !errors.Is(err, boom) {
		t.Fatalf("audit error: want boom, got %v", err)
	}
}

func TestDunningServiceIssue(t *testing.T) {
	ctx := context.Background()
	repo := &fakeDunningRepo{level: validLevel()}
	sink := &fakeAudit{}
	svc := NewDunningService(repo, sink)

	notice, err := svc.Issue(ctx, 2026, "1", "F-2026-001", 12000, "alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if notice.ID != 9 || notice.ChargeCents != 1620 {
		t.Fatalf("notice = %+v", notice)
	}
	if len(sink.events) != 1 || sink.events[0].Action != "dunning.notice.issue" {
		t.Fatalf("audit = %+v", sink.events)
	}

	boom := errors.New("boom")
	if _, err := NewDunningService(&fakeDunningRepo{getErr: boom}, &fakeAudit{}).Issue(ctx, 2026, "1", "F-1", 100, "a"); !errors.Is(err, boom) {
		t.Fatalf("level error: want boom, got %v", err)
	}
	if _, err := NewDunningService(&fakeDunningRepo{level: validLevel(), insertErr: boom}, &fakeAudit{}).Issue(ctx, 2026, "1", "F-1", 100, "a"); !errors.Is(err, boom) {
		t.Fatalf("insert error: want boom, got %v", err)
	}
	if _, err := NewDunningService(repo, failingAudit{err: boom}).Issue(ctx, 2026, "1", "F-1", 100, "a"); !errors.Is(err, boom) {
		t.Fatalf("audit error: want boom, got %v", err)
	}
}

func TestDunningServiceLevel(t *testing.T) {
	ctx := context.Background()
	svc := NewDunningService(&fakeDunningRepo{level: validLevel()}, &fakeAudit{})
	got, err := svc.Level(ctx, 2026, "1")
	if err != nil {
		t.Fatalf("Level: %v", err)
	}
	if got.Code != "1" {
		t.Fatalf("code = %q", got.Code)
	}
}
