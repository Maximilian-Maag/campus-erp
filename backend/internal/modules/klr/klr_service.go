package klr

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// AuditSink records domain events.
type AuditSink interface {
	Append(ctx context.Context, e audit.Event) (audit.Event, error)
}

// Service implements the KLR use cases. Every write is audited.
type Service struct {
	repo  Repository
	audit AuditSink
}

// NewService wires the service.
func NewService(repo Repository, sink AuditSink) *Service {
	return &Service{repo: repo, audit: sink}
}

// CreateCostCenter validates and stores a cost centre.
func (s *Service) CreateCostCenter(ctx context.Context, number, name, by string) (CostCenter, error) {
	if strings.TrimSpace(by) == "" {
		return CostCenter{}, ErrActorRequired
	}
	center, err := NewCostCenter(number, name)
	if err != nil {
		return CostCenter{}, err
	}
	center.CreatedBy = by
	saved, err := s.repo.CreateCostCenter(ctx, center)
	if err != nil {
		return CostCenter{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cost_center.create",
		Subject: costCenterSubject(saved.ID),
		After:   fmt.Sprintf("number=%s name=%s", saved.Number, saved.Name),
	}); err != nil {
		return CostCenter{}, err
	}
	return saved, nil
}

// CreateCostUnit validates and stores a cost unit.
func (s *Service) CreateCostUnit(ctx context.Context, number, name, by string) (CostUnit, error) {
	if strings.TrimSpace(by) == "" {
		return CostUnit{}, ErrActorRequired
	}
	unit, err := NewCostUnit(number, name)
	if err != nil {
		return CostUnit{}, err
	}
	unit.CreatedBy = by
	saved, err := s.repo.CreateCostUnit(ctx, unit)
	if err != nil {
		return CostUnit{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "cost_unit.create",
		Subject: fmt.Sprintf("cost_unit/%d", saved.ID),
		After:   fmt.Sprintf("number=%s name=%s", saved.Number, saved.Name),
	}); err != nil {
		return CostUnit{}, err
	}
	return saved, nil
}

// Post appends a KLR posting for a business date — the period the record belongs to.
func (s *Service) Post(ctx context.Context, costCenterID int64, costUnitID, costTypeID *int64, kind Kind, amountCents int64, businessDate time.Time, reference, by string) (Record, error) {
	if strings.TrimSpace(by) == "" {
		return Record{}, ErrActorRequired
	}
	rec, err := NewRecord(costCenterID, costUnitID, costTypeID, kind, amountCents, businessDate, reference)
	if err != nil {
		return Record{}, err
	}
	rec.BookedBy = by
	saved, err := s.repo.InsertRecord(ctx, rec)
	if err != nil {
		return Record{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "accounting_record.create",
		Subject: costCenterSubject(costCenterID),
		After:   fmt.Sprintf("kind=%s amount_cents=%d business_date=%s", saved.Kind, saved.AmountCents, saved.BusinessDate.Format("2006-01-02")),
	}); err != nil {
		return Record{}, err
	}
	return saved, nil
}

// CostCenters lists every cost centre.
func (s *Service) CostCenters(ctx context.Context) ([]CostCenter, error) {
	return s.repo.CostCenters(ctx)
}

// Report returns a cost centre's report.
func (s *Service) Report(ctx context.Context, costCenterID int64) (Report, error) {
	return s.repo.Report(ctx, costCenterID)
}

// GetCostCenter loads a cost centre.
func (s *Service) GetCostCenter(ctx context.Context, id int64) (CostCenter, error) {
	return s.repo.GetCostCenter(ctx, id)
}

func costCenterSubject(id int64) string { return fmt.Sprintf("cost_center/%d", id) }
