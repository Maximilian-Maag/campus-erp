package lookup

import (
	"context"
	"fmt"
	"strings"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// AuditSink records domain events.
type AuditSink interface {
	Append(ctx context.Context, e audit.Event) (audit.Event, error)
}

// ValueService implements the lookup-catalogue use cases.
type ValueService struct {
	repo  ValueRepository
	audit AuditSink
}

// NewValueService wires the service.
func NewValueService(repo ValueRepository, sink AuditSink) *ValueService {
	return &ValueService{repo: repo, audit: sink}
}

// Upsert validates and stores one value (insert or update in place).
func (s *ValueService) Upsert(ctx context.Context, v Value, by string) (Value, error) {
	if strings.TrimSpace(by) == "" {
		return Value{}, ErrActorRequired
	}
	clean, err := NewValue(v.Namespace, v.Code, v.ShortText, v.LongText, v.SortOrder, v.Active)
	if err != nil {
		return Value{}, err
	}
	saved, err := s.repo.Upsert(ctx, clean)
	if err != nil {
		return Value{}, err
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "lookup.upsert",
		Subject: fmt.Sprintf("lookup/%s/%s", saved.Namespace, saved.Code),
		After:   fmt.Sprintf("short_text=%s active=%t", saved.ShortText, saved.Active),
	}); err != nil {
		return Value{}, err
	}
	return saved, nil
}

// UpsertMany validates and stores a batch, audited once.
func (s *ValueService) UpsertMany(ctx context.Context, values []Value, by string) ([]Value, error) {
	if strings.TrimSpace(by) == "" {
		return nil, ErrActorRequired
	}
	saved := make([]Value, 0, len(values))
	for _, v := range values {
		clean, err := NewValue(v.Namespace, v.Code, v.ShortText, v.LongText, v.SortOrder, v.Active)
		if err != nil {
			return nil, err
		}
		out, err := s.repo.Upsert(ctx, clean)
		if err != nil {
			return nil, err
		}
		saved = append(saved, out)
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "lookup.upsert_many",
		Subject: "lookup",
		After:   fmt.Sprintf("values=%d", len(saved)),
	}); err != nil {
		return nil, err
	}
	return saved, nil
}

// Get loads one value.
func (s *ValueService) Get(ctx context.Context, namespace, code string) (Value, error) {
	return s.repo.Get(ctx, namespace, code)
}

// List returns a namespace's values.
func (s *ValueService) List(ctx context.Context, namespace string) ([]Value, error) {
	return s.repo.List(ctx, namespace)
}

// Seed loads the built-in standard catalogues (idempotent).
func (s *ValueService) Seed(ctx context.Context, by string) (int, error) {
	if strings.TrimSpace(by) == "" {
		return 0, ErrActorRequired
	}
	n := 0
	for _, v := range standardCatalogues() {
		if _, err := s.repo.Upsert(ctx, v); err != nil {
			return n, err
		}
		n++
	}
	if _, err := s.audit.Append(ctx, audit.Event{
		Actor:   by,
		Action:  "lookup.seed",
		Subject: "lookup",
		After:   fmt.Sprintf("values=%d", n),
	}); err != nil {
		return n, err
	}
	return n, nil
}
