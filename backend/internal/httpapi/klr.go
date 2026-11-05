package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Maximilian-Maag/campus-erp/backend/internal/modules/klr"
)

// klrService is the KLR surface the HTTP layer needs.
type klrService interface {
	CreateCostCenter(ctx context.Context, number, name, by string) (klr.CostCenter, error)
	CostCenters(ctx context.Context) ([]klr.CostCenter, error)
	CreateCostUnit(ctx context.Context, number, name, by string) (klr.CostUnit, error)
	Post(ctx context.Context, costCenterID int64, costUnitID, costTypeID *int64, kind klr.Kind, amountCents int64, businessDate time.Time, reference, by string) (klr.Record, error)
	Report(ctx context.Context, costCenterID int64) (klr.Report, error)
	DefineAllocation(ctx context.Context, sourceCenterID, targetCenterID, shareBP int64, validFrom time.Time, validTo *time.Time, by string) (klr.Allocation, error)
	Allocations(ctx context.Context, sourceCenterID int64) ([]klr.Allocation, error)
	Allocate(ctx context.Context, sourceCenterID int64, from, to time.Time, by string) (klr.Run, error)
	PostCostType(ctx context.Context, code, name, by string) (klr.CostType, error)
	CostTypes(ctx context.Context) ([]klr.CostType, error)
	DefineCostUnitKey(ctx context.Context, sourceCenterID, targetUnitID, shareBP int64, validFrom time.Time, validTo *time.Time, by string) (klr.CostUnitKey, error)
	CostUnitKeys(ctx context.Context, sourceCenterID int64) ([]klr.CostUnitKey, error)
	CostUnits(ctx context.Context) ([]klr.CostUnit, error)
	AllocateToCostUnits(ctx context.Context, sourceCenterID int64, from, to time.Time, by string) (klr.Run, error)
	DefineServiceRate(ctx context.Context, costTypeID, rateCents int64, validFrom time.Time, validTo *time.Time, by string) (klr.ServiceRate, error)
	ServiceRates(ctx context.Context) ([]klr.ServiceRate, error)
	ChargeService(ctx context.Context, providerCenterID, receiverCenterID, costTypeID, quantity int64, chargedOn time.Time, by string) (klr.ServiceCharge, error)
	MapAccount(ctx context.Context, costTypeID int64, accountCode, by string) (klr.CostTypeAccount, error)
	AccountsOf(ctx context.Context, costTypeID int64) ([]klr.CostTypeAccount, error)
	CostTypeByAccount(ctx context.Context, accountCode string) (klr.CostType, error)
}

// statusForKLR maps a KLR error to an HTTP status.
func statusForKLR(err error) int {
	switch {
	case errors.Is(err, klr.ErrCostCenterNotFound), errors.Is(err, klr.ErrCostUnitNotFound),
		errors.Is(err, klr.ErrCostTypeNotFound), errors.Is(err, klr.ErrAccountNotMapped):
		return http.StatusNotFound
	case errors.Is(err, klr.ErrCostCenterExists), errors.Is(err, klr.ErrCostUnitExists),
		errors.Is(err, klr.ErrCostTypeExists), errors.Is(err, klr.ErrAccountAlreadyMapped):
		return http.StatusConflict
	case errors.Is(err, klr.ErrAlreadyAllocated):
		return http.StatusConflict
	case errors.Is(err, klr.ErrActorRequired),
		errors.Is(err, klr.ErrNumberRequired),
		errors.Is(err, klr.ErrCodeRequired),
		errors.Is(err, klr.ErrAccountCodeRequired),
		errors.Is(err, klr.ErrNameRequired),
		errors.Is(err, klr.ErrAmountNotPositive),
		errors.Is(err, klr.ErrUnknownKind),
		errors.Is(err, klr.ErrBusinessDateRequired),
		errors.Is(err, klr.ErrAllocationSelf),
		errors.Is(err, klr.ErrAllocationShare),
		errors.Is(err, klr.ErrAllocationPeriod),
		errors.Is(err, klr.ErrAllocationRunPeriod),
		errors.Is(err, klr.ErrAllocationRunNoTargets),
		errors.Is(err, klr.ErrSharesIncomplete),
		errors.Is(err, klr.ErrNothingToAllocate),
		errors.Is(err, klr.ErrRateNotPositive),
		errors.Is(err, klr.ErrQuantityNotPositive),
		errors.Is(err, klr.ErrServiceChargeSelf),
		errors.Is(err, klr.ErrNoRateInForce),
		errors.Is(err, klr.ErrChargeTooLarge):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

type klrNamedRequest struct {
	Number string `json:"number"`
	Name   string `json:"name"`
}

func (s *Server) createCostCenter(w http.ResponseWriter, r *http.Request) {
	var req klrNamedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	center, err := s.klr.CreateCostCenter(r.Context(), req.Number, req.Name, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, center)
}

// listCostCenters reads every cost centre, which is what a screen needs before it can ask about
// one.
func (s *Server) listCostCenters(w http.ResponseWriter, r *http.Request) {
	centers, err := s.klr.CostCenters(r.Context())
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, centers)
}

func (s *Server) createCostUnit(w http.ResponseWriter, r *http.Request) {
	var req klrNamedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	unit, err := s.klr.CreateCostUnit(r.Context(), req.Number, req.Name, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, unit)
}

type klrRecordRequest struct {
	CostUnitID *int64 `json:"cost_unit_id"`
	CostTypeID *int64 `json:"cost_type_id"`
	Kind       string `json:"kind"`
	// BusinessDate is the period the record belongs to ("2026-04-30"). Empty means today: the
	// edge decides that, and the domain is given an explicit date.
	BusinessDate string `json:"business_date"`
	AmountCents  int64  `json:"amount_cents"`
	Reference    string `json:"reference"`
}

// businessDate reads a record's period, defaulting to today when the caller sent none.
func businessDate(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now().UTC().Truncate(24 * time.Hour), nil
	}
	parsed, err := time.Parse(time.DateOnly, raw)
	if err != nil {
		return time.Time{}, err
	}
	return parsed, nil
}

func (s *Server) postAccountingRecord(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req klrRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	date, err := businessDate(req.BusinessDate)
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "business_date must be a date (YYYY-MM-DD)")
		return
	}
	rec, err := s.klr.Post(r.Context(), id, req.CostUnitID, req.CostTypeID, klr.Kind(req.Kind), req.AmountCents, date, req.Reference, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

type defineAllocationRequest struct {
	TargetCenterID int64  `json:"target_center_id"`
	ShareBP        int64  `json:"share_bp"`
	ValidFrom      string `json:"valid_from"`
	ValidTo        string `json:"valid_to"`
}

// defineAllocation stores a distribution key (docs/13). The shares of a cost centre have to add
// up to 10000 basis points when a run uses them, not here: keys are valid for a range, so the
// set in force changes over time.
func (s *Server) defineAllocation(w http.ResponseWriter, r *http.Request) {
	sourceID, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req defineAllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	from, err := time.Parse(time.DateOnly, strings.TrimSpace(req.ValidFrom))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "valid_from must be a date (YYYY-MM-DD)")
		return
	}
	var to *time.Time
	if raw := strings.TrimSpace(req.ValidTo); raw != "" {
		parsed, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "valid_to must be a date (YYYY-MM-DD)")
			return
		}
		to = &parsed
	}
	key, err := s.klr.DefineAllocation(r.Context(), sourceID, req.TargetCenterID, req.ShareBP, from, to, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

func (s *Server) listAllocations(w http.ResponseWriter, r *http.Request) {
	sourceID, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	keys, err := s.klr.Allocations(r.Context(), sourceID)
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

type runAllocationRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// runAllocation distributes a cost centre's period costs over its keys.
func (s *Server) runAllocation(w http.ResponseWriter, r *http.Request) {
	sourceID, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req runAllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	from, err := time.Parse(time.DateOnly, strings.TrimSpace(req.From))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "from must be a date (YYYY-MM-DD)")
		return
	}
	to, err := time.Parse(time.DateOnly, strings.TrimSpace(req.To))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "to must be a date (YYYY-MM-DD)")
		return
	}
	run, err := s.klr.Allocate(r.Context(), sourceID, from, to, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// ── Kostenträgerrechnung: a centre's costs attributed to its cost units ─────

type defineCostUnitKeyRequest struct {
	TargetUnitID int64  `json:"target_unit_id"`
	ShareBP      int64  `json:"share_bp"`
	ValidFrom    string `json:"valid_from"`
	ValidTo      string `json:"valid_to"`
}

// defineCostUnitKey stores a key from a cost centre to one of its cost units.
func (s *Server) defineCostUnitKey(w http.ResponseWriter, r *http.Request) {
	sourceID, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req defineCostUnitKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	from, err := time.Parse(time.DateOnly, strings.TrimSpace(req.ValidFrom))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "valid_from must be a date (YYYY-MM-DD)")
		return
	}
	var to *time.Time
	if raw := strings.TrimSpace(req.ValidTo); raw != "" {
		parsed, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "valid_to must be a date (YYYY-MM-DD)")
			return
		}
		to = &parsed
	}
	key, err := s.klr.DefineCostUnitKey(r.Context(), sourceID, req.TargetUnitID, req.ShareBP, from, to, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, key)
}

// listCostUnits reads every cost unit, which is what a screen needs before it can pick one.
func (s *Server) listCostUnits(w http.ResponseWriter, r *http.Request) {
	units, err := s.klr.CostUnits(r.Context())
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, units)
}

// listCostUnitKeys reads a cost centre's Kostenträger keys.
func (s *Server) listCostUnitKeys(w http.ResponseWriter, r *http.Request) {
	sourceID, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	keys, err := s.klr.CostUnitKeys(r.Context(), sourceID)
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, keys)
}

// runCostUnitAllocation attributes a cost centre's period costs to its cost units.
func (s *Server) runCostUnitAllocation(w http.ResponseWriter, r *http.Request) {
	sourceID, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req runAllocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	from, err := time.Parse(time.DateOnly, strings.TrimSpace(req.From))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "from must be a date (YYYY-MM-DD)")
		return
	}
	to, err := time.Parse(time.DateOnly, strings.TrimSpace(req.To))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "to must be a date (YYYY-MM-DD)")
		return
	}
	run, err := s.klr.AllocateToCostUnits(r.Context(), sourceID, from, to, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

// ── Internal pricing: a rate per cost type, and the charges it prices ───────

type serviceRateRequest struct {
	CostTypeID int64  `json:"cost_type_id"`
	RateCents  int64  `json:"rate_cents"`
	ValidFrom  string `json:"valid_from"`
	ValidTo    string `json:"valid_to"`
}

// defineServiceRate stores the price of one unit of a cost type's service.
func (s *Server) defineServiceRate(w http.ResponseWriter, r *http.Request) {
	var req serviceRateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	from, err := time.Parse(time.DateOnly, strings.TrimSpace(req.ValidFrom))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "valid_from must be a date (YYYY-MM-DD)")
		return
	}
	var to *time.Time
	if raw := strings.TrimSpace(req.ValidTo); raw != "" {
		parsed, err := time.Parse(time.DateOnly, raw)
		if err != nil {
			writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "valid_to must be a date (YYYY-MM-DD)")
			return
		}
		to = &parsed
	}
	rate, err := s.klr.DefineServiceRate(r.Context(), req.CostTypeID, req.RateCents, from, to, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rate)
}

// listServiceRates reads every rate.
func (s *Server) listServiceRates(w http.ResponseWriter, r *http.Request) {
	rates, err := s.klr.ServiceRates(r.Context())
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rates)
}

type serviceChargeRequest struct {
	ProviderCenterID int64  `json:"provider_center_id"`
	ReceiverCenterID int64  `json:"receiver_center_id"`
	CostTypeID       int64  `json:"cost_type_id"`
	Quantity         int64  `json:"quantity"`
	ChargedOn        string `json:"charged_on"`
}

// chargeService charges one cost centre for another's service.
func (s *Server) chargeService(w http.ResponseWriter, r *http.Request) {
	var req serviceChargeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	on, err := time.Parse(time.DateOnly, strings.TrimSpace(req.ChargedOn))
	if err != nil {
		writeProblemCode(w, http.StatusUnprocessableEntity, "invalid_date", "charged_on must be a date (YYYY-MM-DD)")
		return
	}
	charge, err := s.klr.ChargeService(r.Context(), req.ProviderCenterID, req.ReceiverCenterID,
		req.CostTypeID, req.Quantity, on, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, charge)
}

func (s *Server) getKLRReport(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	rep, err := s.klr.Report(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

// ── Cost types (Kostenarten) and the account bridge ─────────────────────────

type klrCostTypeRequest struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

func (s *Server) createCostType(w http.ResponseWriter, r *http.Request) {
	var req klrCostTypeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	ct, err := s.klr.PostCostType(r.Context(), req.Code, req.Name, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, ct)
}

// listCostTypes reads every cost type, which is what a screen needs before it can ask about one.
func (s *Server) listCostTypes(w http.ResponseWriter, r *http.Request) {
	types, err := s.klr.CostTypes(r.Context())
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, types)
}

type klrAccountRequest struct {
	AccountCode string `json:"account_code"`
}

// mapCostTypeAccount links a general-ledger account code to a cost type (the bridge).
func (s *Server) mapCostTypeAccount(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var req klrAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "bad request")
		return
	}
	link, err := s.klr.MapAccount(r.Context(), id, req.AccountCode, actor(r))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, link)
}

// listCostTypeAccounts reads the accounts a cost type owns.
func (s *Server) listCostTypeAccounts(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	accounts, err := s.klr.AccountsOf(r.Context(), id)
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, accounts)
}

// getCostTypeByAccount is the bridge's read side: which cost type an account belongs to. The
// account is passed through as it arrives — the pattern's {account} cannot be empty, and the
// other path-value handlers read their segment the same way.
func (s *Server) getCostTypeByAccount(w http.ResponseWriter, r *http.Request) {
	ct, err := s.klr.CostTypeByAccount(r.Context(), r.PathValue("account"))
	if err != nil {
		writeProblem(w, statusForKLR(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ct)
}
