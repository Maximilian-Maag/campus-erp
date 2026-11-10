package klr

import (
	"context"
	"fmt"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// PostCostType validates and stores a cost type.
func (s *Service) PostCostType(ctx context.Context, code, name, by string) (CostType, error) {
	if strings.TrimSpace(by) == "" {
		return CostType{}, ErrActorRequired
	}
	t, err := NewCostType(code, name)
	if err != nil {
		return CostType{}, err
	}
	t.CreatedBy = by
	saved, err := s.repo.CreateCostType(ctx, t)
	if err != nil {
		return CostType{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cost_type.create",
		Subject: costTypeSubject(saved.ID),
		After:   fmt.Sprintf("code=%s name=%s", saved.Code, saved.Name),
	}); err != nil {
		return CostType{}, err
	}
	return saved, nil
}

// CostTypeFigures lists every cost type with the accounts bridged to it and what the books carry
// on it — the read the Kostenartenübersicht print view is built from (docs/25). It is a read, so
// it is not audited.
func (s *Service) CostTypeFigures(ctx context.Context) ([]CostTypeLine, error) {
	return s.repo.CostTypeFigures(ctx)
}

// CostTypes lists every cost type.
func (s *Service) CostTypes(ctx context.Context) ([]CostType, error) {
	return s.repo.CostTypes(ctx)
}

// GetCostType loads a cost type.
func (s *Service) GetCostType(ctx context.Context, id int64) (CostType, error) {
	return s.repo.GetCostType(ctx, id)
}

// MapAccount links a general-ledger account code to a cost type.
func (s *Service) MapAccount(ctx context.Context, costTypeID int64, accountCode, by string) (CostTypeAccount, error) {
	if strings.TrimSpace(by) == "" {
		return CostTypeAccount{}, ErrActorRequired
	}
	link, err := NewCostTypeAccount(costTypeID, accountCode)
	if err != nil {
		return CostTypeAccount{}, err
	}
	saved, err := s.repo.MapAccount(ctx, link)
	if err != nil {
		return CostTypeAccount{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cost_type.map_account",
		Subject: costTypeSubject(saved.CostTypeID),
		After:   fmt.Sprintf("account_code=%s", saved.AccountCode),
	}); err != nil {
		return CostTypeAccount{}, err
	}
	return saved, nil
}

// AccountsOf lists the accounts a cost type owns.
func (s *Service) AccountsOf(ctx context.Context, costTypeID int64) ([]CostTypeAccount, error) {
	return s.repo.AccountsOf(ctx, costTypeID)
}

// CostTypeByAccount answers the bridge: which cost type an account belongs to.
func (s *Service) CostTypeByAccount(ctx context.Context, accountCode string) (CostType, error) {
	return s.repo.CostTypeByAccount(ctx, accountCode)
}

func costTypeSubject(id int64) string { return fmt.Sprintf("cost_type/%d", id) }
