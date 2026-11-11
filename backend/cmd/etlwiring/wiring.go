// Package etlwiring holds the one composition both commands share: which ETL datasets the registry
// gets.
//
// It exists because the registry is the only door to a dataset, and both `cmd/api` and `cmd/worker`
// are the only places that build one. When the two commands assembled that list themselves, the
// module's datasets could be implemented, integration-tested and documented as wired while neither
// runtime registered them — the registry then held two datasets of six, and every run answered
// "unknown dataset" (docs/19).
//
// Putting the list in one executable function makes it a covered line rather than a hope, and the
// source check in the test keeps the last hop — that each command actually calls it — from going
// missing again.
package etlwiring

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/kasse"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/lookup"
	"github.com/Maximilian-Maag/campus-erp/backend/internal/platform/etl"
)

// Datasets returns every dataset a runtime registers: the lookup reference sets, which hold real
// rows in the productive source, and the cash-book module's own (VAT catalogue, tax offices,
// foundations, counter book).
//
// srcPool is the legacy extract when one is configured and the canonical pool otherwise; the sinks
// always write canonically, because they are built from the services.
func Datasets(srcPool *pgxpool.Pool, lookupSvc *lookup.ValueService, taxSvc *kasse.TaxService, cashSvc *kasse.Service) []etl.Dataset {
	datasets := lookup.StandardDatasets(srcPool, lookupSvc)
	return append(datasets, kasse.StandardDatasets(srcPool, taxSvc, cashSvc)...)
}
