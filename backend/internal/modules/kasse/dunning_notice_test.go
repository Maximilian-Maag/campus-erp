package kasse

import (
	"context"
	"errors"
	"testing"
)

func TestNoticeLoadsTheStoredDocumentKey(t *testing.T) {
	repo := &fakeDunningRepo{notice: DunningNotice{
		ClaimReference: "CLAIM-1", LevelCode: "2", OutstandingCents: 10000,
		ChargeCents: 500, DocumentKey: "dunning/9.pdf",
	}}
	got, err := NewDunningService(repo, &fakeAudit{}).Notice(context.Background(), 9)
	if err != nil {
		t.Fatalf("Notice: %v", err)
	}
	if got.ID != 9 || got.DocumentKey != "dunning/9.pdf" || got.ClaimReference != "CLAIM-1" {
		t.Fatalf("notice = %+v", got)
	}
}

func TestNoticePropagatesNotFound(t *testing.T) {
	repo := &fakeDunningRepo{getErr: ErrNotFound}
	if _, err := NewDunningService(repo, &fakeAudit{}).Notice(context.Background(), 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Notice error = %v, want ErrNotFound", err)
	}
}
