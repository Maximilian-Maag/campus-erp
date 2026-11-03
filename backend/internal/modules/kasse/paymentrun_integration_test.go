package kasse_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
)

// newApprovedOrder creates an order (recipient + budget position), submits it and
// approves it with a different actor, returning the approved order.
func newApprovedOrder(t *testing.T, svc *kasse.OrderService, amountCents int64) kasse.PaymentOrder {
	t.Helper()
	ctx := context.Background()
	rec, err := svc.CreateRecipient(ctx, kasse.Recipient{
		Name: "Empfänger " + uniqueSuffix(t), AccountHolder: "Empfänger GmbH",
		IBAN: uniqueIBAN(t), BIC: "COBADEFFXXX", BankName: "Commerzbank",
	})
	if err != nil {
		t.Fatalf("CreateRecipient: %v", err)
	}
	pos, err := svc.CreateBudgetPosition(ctx, kasse.BudgetPosition{
		Year: 2026, Chapter: "0710", Title: "6810", CostCentre: "KST-" + uniqueSuffix(t), CostCarrier: "KT",
		AllocatedCents: 2 * amountCents,
	})
	if err != nil {
		t.Fatalf("CreateBudgetPosition: %v", err)
	}
	order, err := kasse.NewPaymentOrder("AO-"+uniqueSuffix(t), 2026, amountCents, "EUR",
		rec.ID, pos.ID, "Stromabschlag", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "alice")
	if err != nil {
		t.Fatalf("NewPaymentOrder: %v", err)
	}
	created, err := svc.Create(ctx, order)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Submit(ctx, created.ID, "alice"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	approved, err := svc.Approve(ctx, created.ID, "bob")
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	return approved
}

func newRunService(t *testing.T) (*kasse.PaymentRunService, *kasse.OrderService) {
	t.Helper()
	pool := testPool(t)
	store := audit.NewStore(pool)
	return kasse.NewPaymentRunService(kasse.NewPaymentRunRepo(pool), store),
		kasse.NewOrderService(kasse.NewOrderRepo(pool), budgetService(pool), store)
}

func TestPaymentRunProducesSEPAFile(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)

	a := newApprovedOrder(t, orderSvc, 125000)
	b := newApprovedOrder(t, orderSvc, 4999)

	account, err := runSvc.CreateCashAccount(ctx, "Universität Musterstadt", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}

	ref := "RUN-" + uniqueSuffix(t)
	run, err := runSvc.CreateRun(ctx, ref, account.ID, time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{a.ID, b.ID}, "carol")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.ItemCount != 2 {
		t.Fatalf("ItemCount = %d, want 2", run.ItemCount)
	}
	if run.SumCents != 129999 {
		t.Fatalf("SumCents = %d, want 129999", run.SumCents)
	}
	if run.FileHash == "" {
		t.Fatal("FileHash must be set")
	}

	file, err := runSvc.RunFile(ctx, run.ID)
	if err != nil {
		t.Fatalf("RunFile: %v", err)
	}
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		"<MsgId>" + ref + "</MsgId>",
		"<EndToEndId>" + a.Reference + "</EndToEndId>",
		"<EndToEndId>" + b.Reference + "</EndToEndId>",
		"<IBAN>DE02120300000000202051</IBAN>",
	} {
		if !strings.Contains(file, want) {
			t.Errorf("file missing %q", want)
		}
	}

	// Idempotency: the same reference returns the same run and file, no second run.
	again, err := runSvc.CreateRun(ctx, ref, account.ID, time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{a.ID, b.ID}, "carol")
	if err != nil {
		t.Fatalf("re-run CreateRun: %v", err)
	}
	if again.ID != run.ID {
		t.Fatalf("re-run produced a new run %d (want %d)", again.ID, run.ID)
	}

	// An order can be paid at most once.
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{a.ID}, "carol"); !errors.Is(err, kasse.ErrOrderAlreadyPaid) {
		t.Fatalf("want ErrOrderAlreadyPaid, got %v", err)
	}

	// Audit chain intact.
	store := audit.NewStore(testPool(t))
	if err := store.Verify(ctx); err != nil {
		t.Fatalf("audit verify: %v", err)
	}
}

func TestPaymentRunRejectsUnapprovedOrder(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	// A fresh draft order (not approved).
	rec, err := orderSvc.CreateRecipient(ctx, kasse.Recipient{Name: "X", AccountHolder: "X", IBAN: "GB29NWBK60161331926819", BIC: "NWBKGB2L"})
	if err != nil {
		t.Fatalf("CreateRecipient: %v", err)
	}
	pos, err := orderSvc.CreateBudgetPosition(ctx, kasse.BudgetPosition{Year: 2026, Chapter: "1", Title: "2", CostCentre: uniqueSuffix(t), CostCarrier: "k", AllocatedCents: 10000})
	if err != nil {
		t.Fatalf("CreateBudgetPosition: %v", err)
	}
	draft, err := kasse.NewPaymentOrder("AO-"+uniqueSuffix(t), 2026, 100, "EUR", rec.ID, pos.ID, "p", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "alice")
	if err != nil {
		t.Fatalf("NewPaymentOrder: %v", err)
	}
	created, err := orderSvc.Create(ctx, draft)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{created.ID}, "carol"); !errors.Is(err, kasse.ErrOrderNotApproved) {
		t.Fatalf("want ErrOrderNotApproved, got %v", err)
	}
}

func TestPaymentRunValidation(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	order := newApprovedOrder(t, orderSvc, 500)
	exec := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		fn   func() (kasse.PaymentRun, error)
		want error
	}{
		{"no reference", func() (kasse.PaymentRun, error) {
			return runSvc.CreateRun(ctx, "  ", account.ID, exec, []int64{order.ID}, "carol")
		}, kasse.ErrRunReferenceRequired},
		{"no actor", func() (kasse.PaymentRun, error) {
			return runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID, exec, []int64{order.ID}, "")
		}, kasse.ErrActorRequired},
		{"no account", func() (kasse.PaymentRun, error) {
			return runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), 0, exec, []int64{order.ID}, "carol")
		}, kasse.ErrCashAccountRequired},
		{"no execution date", func() (kasse.PaymentRun, error) {
			return runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID, time.Time{}, []int64{order.ID}, "carol")
		}, kasse.ErrExecutionDateRequired},
		{"no orders", func() (kasse.PaymentRun, error) {
			return runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID, exec, nil, "carol")
		}, kasse.ErrNoOrders},
		{"unknown account", func() (kasse.PaymentRun, error) {
			return runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), 9223372036854775807, exec, []int64{order.ID}, "carol")
		}, kasse.ErrNotFound},
		{"unknown order", func() (kasse.PaymentRun, error) {
			return runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID, exec, []int64{9223372036854775807}, "carol")
		}, kasse.ErrNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := c.fn(); !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
		})
	}
}

func TestPaymentRunNotFound(t *testing.T) {
	ctx := context.Background()
	runSvc, _ := newRunService(t)
	if _, err := runSvc.Run(ctx, 9223372036854775807); !errors.Is(err, kasse.ErrRunNotFound) {
		t.Fatalf("Run: want ErrRunNotFound, got %v", err)
	}
	if _, err := runSvc.RunFile(ctx, 9223372036854775807); !errors.Is(err, kasse.ErrRunNotFound) {
		t.Fatalf("RunFile: want ErrRunNotFound, got %v", err)
	}
}

// TestPaymentRunMissingOrderMessage pins the diagnostic: it must report how many
// of the requested orders were not found.
func TestPaymentRunMissingOrderMessage(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	order := newApprovedOrder(t, orderSvc, 500)
	_, err = runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{order.ID, 9223372036854775807}, "carol")
	if !errors.Is(err, kasse.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "1 of 2 orders not found") {
		t.Fatalf("message = %q, want it to report 1 of 2", err.Error())
	}
}

// TestPaymentRunRejectsInvalidCreditorBIC covers the BIC guard on the order.
func TestPaymentRunRejectsInvalidCreditorBIC(t *testing.T) {
	ctx := context.Background()
	runSvc, orderSvc := newRunService(t)
	account, err := runSvc.CreateCashAccount(ctx, "Uni", "DE02120300000000202051", "BYLADEM1001", "EUR")
	if err != nil {
		t.Fatalf("CreateCashAccount: %v", err)
	}
	rec, err := orderSvc.CreateRecipient(ctx, kasse.Recipient{Name: "X", AccountHolder: "X", IBAN: uniqueIBAN(t), BIC: "NOTABIC"})
	if err != nil {
		t.Fatalf("CreateRecipient: %v", err)
	}
	pos, err := orderSvc.CreateBudgetPosition(ctx, kasse.BudgetPosition{Year: 2026, Chapter: "1", Title: "2", CostCentre: uniqueSuffix(t), CostCarrier: "k", AllocatedCents: 10000})
	if err != nil {
		t.Fatalf("CreateBudgetPosition: %v", err)
	}
	order, err := kasse.NewPaymentOrder("AO-"+uniqueSuffix(t), 2026, 700, "EUR", rec.ID, pos.ID, "p", time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC), "alice")
	if err != nil {
		t.Fatalf("NewPaymentOrder: %v", err)
	}
	created, err := orderSvc.Create(ctx, order)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := orderSvc.Submit(ctx, created.ID, "alice"); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if _, err := orderSvc.Approve(ctx, created.ID, "bob"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if _, err := runSvc.CreateRun(ctx, "RUN-"+uniqueSuffix(t), account.ID,
		time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC), []int64{created.ID}, "carol"); !errors.Is(err, kasse.ErrBICInvalid) {
		t.Fatalf("want ErrBICInvalid, got %v", err)
	}
}
