package klr

import (
	"context"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/core/audit"
)

// fakeRepo is a Repository whose writes succeed and whose reads return what the test set, so
// the service's own decisions — validation, the audit call, the error it hands back — can be
// read without a database.
type fakeRepo struct {
	center      CostCenter
	record      Record
	allocation  Allocation
	allocations []Allocation
	costs       int64
	err         error
}

func (f *fakeRepo) CreateCostCenter(context.Context, CostCenter) (CostCenter, error) {
	return f.center, f.err
}
func (f *fakeRepo) GetCostCenter(context.Context, int64) (CostCenter, error) {
	return f.center, f.err
}
func (f *fakeRepo) CreateCostUnit(context.Context, CostUnit) (CostUnit, error) {
	return CostUnit{}, f.err
}
func (f *fakeRepo) InsertRecord(context.Context, Record) (Record, error) { return f.record, f.err }
func (f *fakeRepo) Report(context.Context, int64) (Report, error)        { return Report{}, f.err }
func (f *fakeRepo) CreateAllocation(_ context.Context, a Allocation) (Allocation, error) {
	if f.err != nil {
		return Allocation{}, f.err
	}
	a.ID = f.allocation.ID
	return a, nil
}
func (f *fakeRepo) Allocations(context.Context, int64) ([]Allocation, error) {
	return f.allocations, f.err
}
func (f *fakeRepo) CostsInPeriod(context.Context, int64, time.Time, time.Time) (int64, error) {
	return f.costs, f.err
}
func (f *fakeRepo) RecordRun(_ context.Context, run Run, _ []Record) (Run, error) {
	if f.err != nil {
		return Run{}, f.err
	}
	run.ID = 1
	return run, nil
}

// failingAudit refuses every event, which is how a service's audit handling is exercised.
type failingAudit struct{ err error }

func (f failingAudit) Append(context.Context, audit.Event) (audit.Event, error) {
	return audit.Event{}, f.err
}
