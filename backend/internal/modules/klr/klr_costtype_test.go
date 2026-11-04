package klr

import (
	"context"
	"errors"
	"testing"
)

func TestNewCostTypeValidation(t *testing.T) {
	ct, err := NewCostType(" 4711 ", " Personalkosten ")
	if err != nil {
		t.Fatalf("valid cost type rejected: %v", err)
	}
	if ct.Code != "4711" || ct.Name != "Personalkosten" {
		t.Fatalf("not trimmed: %+v", ct)
	}
	cases := []struct {
		name, code, nameArg string
		err                 error
	}{
		{"blank code", "  ", "Personalkosten", ErrCodeRequired},
		{"blank name", "4711", "  ", ErrNameRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewCostType(c.code, c.nameArg); !errors.Is(err, c.err) {
				t.Fatalf("err = %v, want %v", err, c.err)
			}
		})
	}
}

func TestNewCostTypeAccountValidation(t *testing.T) {
	link, err := NewCostTypeAccount(7, " 6810 ")
	if err != nil {
		t.Fatalf("valid link rejected: %v", err)
	}
	if link.CostTypeID != 7 || link.AccountCode != "6810" {
		t.Fatalf("link = %+v", link)
	}
	if _, err := NewCostTypeAccount(0, "6810"); !errors.Is(err, ErrCostTypeNotFound) {
		t.Fatalf("no cost type = %v, want ErrCostTypeNotFound", err)
	}
	if _, err := NewCostTypeAccount(1, "  "); !errors.Is(err, ErrAccountCodeRequired) {
		t.Fatalf("blank account = %v, want ErrAccountCodeRequired", err)
	}
}

func TestPostCostType(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, okAudit{})

	saved, err := svc.PostCostType(context.Background(), "4711", "Personalkosten", "alice")
	if err != nil {
		t.Fatalf("PostCostType: %v", err)
	}
	if saved.ID != 1 || saved.Code != "4711" || saved.CreatedBy != "alice" {
		t.Fatalf("saved = %+v", saved)
	}
	// An actor is required, and it is checked before the repository is touched.
	if _, err := svc.PostCostType(context.Background(), "4711", "x", "  "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("no actor = %v, want ErrActorRequired", err)
	}
	// Validation happens before the write.
	if _, err := svc.PostCostType(context.Background(), " ", "x", "alice"); !errors.Is(err, ErrCodeRequired) {
		t.Fatalf("blank code = %v, want ErrCodeRequired", err)
	}

	boom := errors.New("boom")
	if _, err := NewService(&fakeRepo{err: boom}, okAudit{}).PostCostType(context.Background(), "1", "x", "alice"); !errors.Is(err, boom) {
		t.Fatalf("repo error = %v, want boom", err)
	}
	if _, err := NewService(&fakeRepo{}, failingAudit{err: boom}).PostCostType(context.Background(), "1", "x", "alice"); !errors.Is(err, boom) {
		t.Fatalf("audit error = %v, want boom", err)
	}
}

func TestMapAccount(t *testing.T) {
	svc := NewService(&fakeRepo{}, okAudit{})

	saved, err := svc.MapAccount(context.Background(), 3, "6810", "alice")
	if err != nil {
		t.Fatalf("MapAccount: %v", err)
	}
	if saved.ID != 1 || saved.CostTypeID != 3 || saved.AccountCode != "6810" {
		t.Fatalf("link = %+v", saved)
	}
	if _, err := svc.MapAccount(context.Background(), 3, "6810", " "); !errors.Is(err, ErrActorRequired) {
		t.Fatalf("no actor = %v, want ErrActorRequired", err)
	}
	if _, err := svc.MapAccount(context.Background(), 0, "6810", "alice"); !errors.Is(err, ErrCostTypeNotFound) {
		t.Fatalf("no cost type = %v, want ErrCostTypeNotFound", err)
	}

	boom := errors.New("boom")
	if _, err := NewService(&fakeRepo{err: boom}, okAudit{}).MapAccount(context.Background(), 1, "1", "alice"); !errors.Is(err, boom) {
		t.Fatalf("repo error = %v, want boom", err)
	}
	if _, err := NewService(&fakeRepo{}, failingAudit{err: boom}).MapAccount(context.Background(), 1, "1", "alice"); !errors.Is(err, boom) {
		t.Fatalf("audit error = %v, want boom", err)
	}
}

func TestCostTypeReads(t *testing.T) {
	repo := &fakeRepo{
		costTypes: []CostType{{ID: 1, Code: "4711"}},
		costType:  CostType{ID: 1, Code: "4711"},
		links:     []CostTypeAccount{{ID: 1, CostTypeID: 1, AccountCode: "6810"}},
	}
	svc := NewService(repo, okAudit{})
	ctx := context.Background()

	types, err := svc.CostTypes(ctx)
	if err != nil || len(types) != 1 || types[0].Code != "4711" {
		t.Fatalf("CostTypes = %v, %v", types, err)
	}
	one, err := svc.GetCostType(ctx, 1)
	if err != nil || one.ID != 1 {
		t.Fatalf("GetCostType = %+v, %v", one, err)
	}
	links, err := svc.AccountsOf(ctx, 1)
	if err != nil || len(links) != 1 || links[0].AccountCode != "6810" {
		t.Fatalf("AccountsOf = %v, %v", links, err)
	}
	byAccount, err := svc.CostTypeByAccount(ctx, "6810")
	if err != nil || byAccount.ID != 1 {
		t.Fatalf("CostTypeByAccount = %+v, %v", byAccount, err)
	}
}
