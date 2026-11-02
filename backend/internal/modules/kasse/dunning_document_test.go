package kasse

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeDunningDocuments struct {
	key    string
	err    error
	called int
	notice DunningNotice
	level  DunningLevel
}

func (f *fakeDunningDocuments) StoreNotice(_ context.Context, n DunningNotice, l DunningLevel) (string, error) {
	f.called++
	f.notice, f.level = n, l
	if f.err != nil {
		return "", f.err
	}
	return f.key, nil
}

func activeLevel(t *testing.T) DunningLevel {
	t.Helper()
	return DunningLevel{Year: 2026, Code: "2", Description: "Mahnung", DaysAllowed: 14, Active: true,
		Bands: []DunningBand{{UpToCents: 0, FeeCents: 500, RateBasisPoints: 0}}}
}

func TestIssueStoresTheDocumentAndRecordsIt(t *testing.T) {
	repo := &fakeDunningRepo{level: activeLevel(t)}
	docs := &fakeDunningDocuments{key: "dunning/9.pdf"}
	sink := &fakeAudit{}
	svc := NewDunningService(repo, sink).WithDocuments(docs)

	got, err := svc.Issue(context.Background(), 2026, "2", "CLAIM-1", 10000, "alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if docs.called != 1 || docs.notice.ClaimReference != "CLAIM-1" || docs.level.Code != "2" {
		t.Fatalf("document issuer called with %+v / %+v (%d)", docs.notice, docs.level, docs.called)
	}
	if got.DocumentKey != "dunning/9.pdf" {
		t.Fatalf("document key = %q", got.DocumentKey)
	}
	if repo.documentID != 9 || repo.documentKey != "dunning/9.pdf" {
		t.Fatalf("repo recorded %d/%q", repo.documentID, repo.documentKey)
	}
	last := sink.events[len(sink.events)-1]
	if !strings.Contains(last.After, "document=dunning/9.pdf") {
		t.Fatalf("audit detail = %q", last.After)
	}
}

func TestIssueWithoutDocumentsRecordsNoKey(t *testing.T) {
	repo := &fakeDunningRepo{level: activeLevel(t)}
	sink := &fakeAudit{}
	got, err := NewDunningService(repo, sink).Issue(context.Background(), 2026, "2", "CLAIM-1", 10000, "alice")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if got.DocumentKey != "" || repo.documentKey != "" {
		t.Fatalf("no documents -> no key: %q / %q", got.DocumentKey, repo.documentKey)
	}
	last := sink.events[len(sink.events)-1]
	if strings.Contains(last.After, "document=") {
		t.Fatalf("audit detail = %q", last.After)
	}
}

func TestIssueFailsWhenTheDocumentFails(t *testing.T) {
	boom := errors.New("boom")
	repo := &fakeDunningRepo{level: activeLevel(t)}
	svc := NewDunningService(repo, &fakeAudit{}).WithDocuments(&fakeDunningDocuments{err: boom})
	if _, err := svc.Issue(context.Background(), 2026, "2", "CLAIM-1", 10000, "alice"); !errors.Is(err, boom) {
		t.Fatalf("Issue error = %v, want the renderer's", err)
	}
	if repo.documentKey != "" {
		t.Fatalf("nothing must be recorded when rendering fails")
	}
}

func TestIssueFailsWhenRecordingTheDocumentFails(t *testing.T) {
	boom := errors.New("boom")
	repo := &fakeDunningRepo{level: activeLevel(t), documentErr: boom}
	svc := NewDunningService(repo, &fakeAudit{}).WithDocuments(&fakeDunningDocuments{key: "dunning/9.pdf"})
	if _, err := svc.Issue(context.Background(), 2026, "2", "CLAIM-1", 10000, "alice"); !errors.Is(err, boom) {
		t.Fatalf("Issue error = %v", err)
	}
}
