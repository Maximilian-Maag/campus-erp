#!/usr/bin/env python3
"""Generate contracts/openapi.yaml from the Go HTTP surface.

This is the *authoring* generator: it reads the explicit route table below plus
the request/response structs in the Go source and emits an OpenAPI 3.1 document.
It requires PyYAML, so it is a development tool. The quality gate
(tools/api_contract.py) is stdlib-only and parses the constrained YAML this
file emits.

Run:  /usr/bin/python3 tools/gen_openapi.py           # write contracts/openapi.yaml
      /usr/bin/python3 tools/gen_openapi.py --stdout  # print only
      /usr/bin/python3 tools/gen_openapi.py --check   # verify committed spec is current

Types are qualified as <pkg>.<Type>. Component names are the bare type name when
unique across packages, otherwise <Pkg><Type>.
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parent.parent
BACKEND = ROOT / "backend" / "internal"
SPEC = ROOT / "contracts" / "openapi.yaml"

# ─────────────────────────────────────────────────────────────────────────────
# The contract table. This is the single place to edit when a route changes.
#   path -> method -> (operationId, tag, summary, requestType, responseType,
#                      successStatus, responseKind)
# requestType/responseType are "pkg.Type" or None. responseKind: "json", "text",
# "file". A responseType of "[]pkg.Type" means a JSON array.
# ─────────────────────────────────────────────────────────────────────────────
CONTRACT: dict[str, dict[str, tuple]] = {
    "/healthz": {
        "get": ("getHealth", "platform", "Liveness/readiness probe", None, None, 200, "text"),
    },
    "/readyz": {
        "get": ("getReady", "platform", "Readiness probe (pings the database)", None, None, 200, "text"),
    },
    "/api/audit/verify": {
        "get": ("verifyAuditChain", "platform", "Recompute the audit hash chain", None, None, 200, "text"),
    },
    # ── Kasse ────────────────────────────────────────────────────────────────
    "/api/cashbook/days": {
        "post": ("openCashbookDay", "kasse", "Open a cash-book day", "httpapi.openDayRequest", "kasse.Day", 201, "json"),
    },
    "/api/cashbook/days/{id}": {
        "get": ("getCashbookDay", "kasse", "Read a cash-book day", None, "kasse.Day", 200, "json"),
    },
    "/api/cashbook/days/{id}/bookings": {
        "post": ("bookCashbookEntry", "kasse", "Post a booking to a day", "httpapi.bookRequest", "kasse.Booking", 201, "json"),
    },
    "/api/cashbook/days/{id}/close": {
        "post": ("closeCashbookDay", "kasse", "Close a cash-book day", None, "kasse.Day", 200, "json"),
    },
    "/api/cash-accounts": {
        "post": ("createCashAccount", "kasse", "Create an own cash account", "httpapi.createCashAccountRequest", "kasse.CashAccount", 201, "json"),
    },
    "/api/payment-orders": {
        "post": ("createPaymentOrder", "kasse", "Create a payment order", "httpapi.createOrderRequest", "kasse.PaymentOrder", 201, "json"),
    },
    "/api/payment-orders/{id}": {
        "get": ("getPaymentOrder", "kasse", "Read a payment order", None, "kasse.PaymentOrder", 200, "json"),
    },
    "/api/payment-orders/{id}/submit": {
        "post": ("submitPaymentOrder", "kasse", "Submit a payment order", None, "kasse.PaymentOrder", 200, "json"),
    },
    "/api/payment-orders/{id}/approve": {
        "post": ("approvePaymentOrder", "kasse", "Approve a payment order", None, "kasse.PaymentOrder", 200, "json"),
    },
    "/api/payment-orders/{id}/reject": {
        "post": ("rejectPaymentOrder", "kasse", "Reject a payment order", "httpapi.decisionRequest", "kasse.PaymentOrder", 200, "json"),
    },
    "/api/payment-orders/{id}/cancel": {
        "post": ("cancelPaymentOrder", "kasse", "Cancel a payment order", None, "kasse.PaymentOrder", 200, "json"),
    },
    "/api/payment-runs": {
        "post": ("createPaymentRun", "kasse", "Create a SEPA payment run", "httpapi.createRunRequest", "kasse.PaymentRun", 201, "json"),
    },
    "/api/payment-runs/{id}": {
        "get": ("getPaymentRun", "kasse", "Read a payment run", None, "kasse.PaymentRun", 200, "json"),
    },
    "/api/payment-runs/{id}/file": {
        "get": ("getPaymentRunFile", "kasse", "Download the pain.001 XML of a run", None, None, 200, "file"),
    },
    "/api/bank-statements": {
        "post": ("importBankStatement", "kasse", "Import an MT940 bank statement", "httpapi.importStatementRequest", "[]kasse.Statement", 201, "json"),
    },
    "/api/bank-statements/{id}": {
        "get": ("getBankStatement", "kasse", "Read a bank statement", None, "kasse.Statement", 200, "json"),
    },
    "/api/bank-files/{id}": {
        "get": ("getBankFile", "kasse", "Read an imported bank file", None, "kasse.BankFile", 200, "json"),
    },
    "/api/foundation-books": {
        "post": ("createFoundationBook", "kasse", "Create a foundation book", "httpapi.createFoundationBookRequest", "kasse.FoundationBook", 201, "json"),
    },
    "/api/foundation-books/{id}/bookings": {
        "post": ("bookFoundation", "kasse", "Post to a foundation book", "httpapi.bookFoundationRequest", "kasse.FoundationBooking", 201, "json"),
        "get": ("listFoundationBookings", "kasse", "List foundation bookings", None, "[]kasse.FoundationBooking", 200, "json"),
    },
    "/api/foundation-books/{id}/balance": {
        "get": ("getFoundationBalance", "kasse", "Read a foundation balance", None, "kasse.FoundationBalance", 200, "json"),
    },
    "/api/dunning/notices/{id}/document": {
        "get": ("getDunningNoticeDocument", "kasse", "Read a dunning notice's stored PDF", None, None, 200, "pdf"),
    },
    "/api/artefacts/{kind}/{name}": {
        "get": ("getArtefact", "kasse", "Download a stored artefact (notice PDF, report, ledger/SEPA export)", None, None, 200, "artefact"),
    },
    "/api/statistics/exports": {
        "post": ("requestStatisticsExport", "kasse", "Queue a per-state statistics export (renders on the worker role)", "httpapi.statisticsExportRequest", "httpapi.exportQueuedResponse", 202, "json"),
    },
    "/api/ledger/exports": {
        "post": ("requestLedgerExport", "kasse", "Queue a ledger posting export (renders on the worker role)", "httpapi.ledgerExportRequest", "httpapi.exportQueuedResponse", 202, "json"),
    },
    "/api/reports": {
        "get": ("listReports", "kasse", "List the reports this system renders", None, "[]report.Definition", 200, "json"),
    },
    "/api/reports/renders": {
        "post": ("requestReportRender", "kasse", "Queue a report render (renders on the worker role)", "httpapi.reportRenderRequest", "httpapi.exportQueuedResponse", 202, "json"),
    },
    "/api/bank-statements/imports": {
        "post": ("requestStatementImport", "kasse", "Queue a bank-statement import (runs on the worker role)", "httpapi.statementImportRequest", "httpapi.exportQueuedResponse", 202, "json"),
    },
    "/api/payment-runs/{id}/exports": {
        "post": ("requestSepaExport", "kasse", "Queue a payment run's SEPA file for delivery to the bank channel", None, "httpapi.exportQueuedResponse", 202, "json"),
    },
    "/api/dunning/levels": {
        "post": ("createDunningLevel", "kasse", "Create a dunning level", "httpapi.createLevelRequest", "kasse.DunningLevel", 201, "json"),
    },
    "/api/dunning/levels/{year}/{code}": {
        "get": ("getDunningLevel", "kasse", "Read a dunning level", None, "kasse.DunningLevel", 200, "json"),
    },
    "/api/dunning/notices": {
        "post": ("issueDunningNotice", "kasse", "Issue a dunning notice", "httpapi.issueNoticeRequest", "kasse.DunningNotice", 201, "json"),
    },
    # ── E-invoicing ──────────────────────────────────────────────────────────
    "/api/e-invoices": {
        "post": ("createEInvoice", "einvoice", "Generate an outgoing XRechnung 3.0 invoice", "httpapi.createEInvoiceRequest", "einvoice.Invoice", 201, "json"),
    },
    "/api/e-invoices/received": {
        "post": ("receiveEInvoice", "einvoice", "Import and validate a received XRechnung (raw XML body)", None, "einvoice.Invoice", 201, "json"),
    },
    "/api/e-invoices/{id}": {
        "get": ("getEInvoice", "einvoice", "Read an e-invoice", None, "einvoice.Invoice", 200, "json"),
    },
    "/api/e-invoices/{id}/xrechnung": {
        "get": ("getEInvoiceXML", "einvoice", "Download the UBL 2.1 XML of an e-invoice", None, None, 200, "file"),
    },
    "/api/e-invoices/{id}/match": {
        "post": ("matchEInvoice", "einvoice", "Match an inbound e-invoice to the purchase order its buyer reference names and convert the commitment", None, "einvoice.Invoice", 200, "json"),
    },
    # ── Budget ───────────────────────────────────────────────────────────────
    "/api/budget/positions": {
        "get": ("listBudgetPositions", "budget", "List every budget position with its balance", None, "[]httpapi.budgetPositionResponse", 200, "json"),
        "post": ("createBudgetPosition", "budget", "Create a budget position", "httpapi.createBudgetPositionRequest", "budget.Position", 201, "json"),
    },
    "/api/budget/positions/{id}": {
        "get": ("getBudgetPosition", "budget", "Read a budget position with balance", None, "httpapi.budgetPositionResponse", 200, "json"),
    },
    "/api/budget/positions/{id}/postings": {
        "post": ("postBudgetMovement", "budget", "Post a budget movement", "httpapi.budgetPostingRequest", "budget.Posting", 201, "json"),
    },
    "/api/budget/positions/{id}/balance": {
        "get": ("getBudgetBalance", "budget", "Read a budget position balance", None, "budget.Balance", 200, "json"),
    },
    # ── Lookups ──────────────────────────────────────────────────────────────
    "/api/lookups/seed": {
        "post": ("seedLookups", "lookup", "Seed the lookup catalogue", None, "lookup.SeedResult", 200, "json"),
    },
    "/api/lookups/{namespace}": {
        "get": ("listLookups", "lookup", "List lookup values in a namespace", None, "[]lookup.Value", 200, "json"),
        "post": ("upsertLookups", "lookup", "Upsert lookup values in a namespace", "httpapi.upsertLookupsRequest", "[]lookup.Value", 200, "json"),
    },
    "/api/lookups/{namespace}/{code}": {
        "get": ("getLookup", "lookup", "Read one lookup value", None, "lookup.Value", 200, "json"),
    },
    # ── Procurement ──────────────────────────────────────────────────────────
    "/api/purchase-orders": {
        "get": ("listPurchaseOrders", "procurement", "List purchase orders, optionally narrowed to one status", None, "[]procurement.OrderSummary", 200, "json"),
        "post": ("createPurchaseOrder", "procurement", "Create a purchase order", "httpapi.createPurchaseOrderRequest", "procurement.Order", 201, "json"),
    },
    "/api/purchase-orders/{id}": {
        "get": ("getPurchaseOrder", "procurement", "Read a purchase order", None, "procurement.Order", 200, "json"),
    },
    "/api/purchase-orders/{id}/approve": {
        "post": ("approvePurchaseOrder", "procurement", "Approve a purchase order", None, "procurement.Order", 200, "json"),
    },
    "/api/purchase-orders/{id}/receive": {
        "post": ("receivePurchaseOrder", "procurement", "Receive a purchase order", None, "procurement.Order", 200, "json"),
    },
    "/api/purchase-orders/{id}/cancel": {
        "post": ("cancelPurchaseOrder", "procurement", "Cancel a purchase order", None, "procurement.Order", 200, "json"),
    },
    # ── KLR ──────────────────────────────────────────────────────────────────
    "/api/klr/cost-centers": {
        "post": ("createCostCenter", "klr", "Create a cost centre", "httpapi.klrNamedRequest", "klr.CostCenter", 201, "json"),
    },
    "/api/klr/cost-units": {
        "post": ("createCostUnit", "klr", "Create a cost unit", "httpapi.klrNamedRequest", "klr.CostUnit", 201, "json"),
    },
    "/api/klr/cost-centers/{id}/records": {
        "post": ("postAccountingRecord", "klr", "Post an accounting record", "httpapi.klrRecordRequest", "klr.Record", 201, "json"),
    },
    "/api/klr/cost-centers/{id}/report": {
        "get": ("getKLRReport", "klr", "Read the cost-centre report", None, "klr.Report", 200, "json"),
    },
    # ── Tax / foundations ────────────────────────────────────────────────────
    "/api/vat-types": {
        "post": ("createVATType", "kasse", "Create a VAT type", "httpapi.vatTypeRequest", "kasse.VATType", 201, "json"),
    },
    "/api/vat-types/{code}": {
        "get": ("getVATType", "kasse", "Read a VAT type", None, "kasse.VATType", 200, "json"),
    },
    "/api/tax-offices": {
        "post": ("createTaxOffice", "kasse", "Create a tax office", "httpapi.taxOfficeRequest", "kasse.TaxOffice", 201, "json"),
    },
    "/api/foundations": {
        "post": ("createFoundation", "kasse", "Create a foundation", "httpapi.foundationRequest", "kasse.Foundation", 201, "json"),
    },
    "/api/direct-debit/mandates": {
        "post": ("createMandate", "kasse", "Register a SEPA direct-debit mandate", "httpapi.mandateRequest", "kasse.Mandate", 201, "json"),
    },
    "/api/direct-debit/mandates/{reference}": {
        "get": ("getMandate", "kasse", "Read a SEPA direct-debit mandate", None, "kasse.Mandate", 200, "json"),
    },
    "/api/direct-debit/runs": {
        "post": ("createCollectionRun", "kasse", "Create a SEPA direct-debit collection run", "httpapi.collectionRunRequest", "kasse.CollectionRun", 201, "json"),
    },
    "/api/direct-debit/runs/{reference}/file": {
        "get": ("getCollectionRunFile", "kasse", "Download the pain.008 XML of a collection run", None, None, 200, "file"),
    },
    "/api/etl/datasets": {
        "get": ("listETLDatasets", "etl", "List the registered ETL datasets", None, "httpapi.datasetsResponse", 200, "json"),
    },
    "/api/etl/runs": {
        "get": ("listETLRuns", "etl", "List recent ETL runs (by dataset, read from the audit stream)", None, "httpapi.etlRunsResponse", 200, "json"),
        "post": ("runETLDataset", "etl", "Run a named ETL dataset", "httpapi.etlRunRequest", "etl.Report", 200, "json"),
    },
}

# Synthetic schemas for constructs that exist only as anonymous structs in Go.
SYNTHETIC: dict[str, dict[str, str]] = {
    "httpapi.decisionRequest": {"reason": "string"},
    "lookup.SeedResult": {"seeded": "int"},
}

SCALAR_OVERRIDES = {
    "time.Time": ("string", "date-time"),
    "time.Duration": ("integer", "int64"),
}

PKG_DIRS = {
    "httpapi": BACKEND / "httpapi",
    "kasse": BACKEND / "modules" / "kasse",
    "budget": BACKEND / "modules" / "budget",
    "klr": BACKEND / "modules" / "klr",
    "lookup": BACKEND / "modules" / "lookup",
    "procurement": BACKEND / "modules" / "procurement",
    "einvoice": BACKEND / "core" / "einvoice",
    "sepa": BACKEND / "core" / "sepa",
    "audit": BACKEND / "core" / "audit",
    "report": BACKEND / "core" / "report",
    "finance": BACKEND / "modules" / "finance",
    "personal": BACKEND / "modules" / "personal",
}

TYPE_RE = re.compile(r"^type\s+([A-Za-z_][A-Za-z0-9_]*)\s+struct\s*\{\s*$")
NAMED_RE = re.compile(r"^type\s+([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z0-9_\[\]]+)\s*$")
FIELD_RE = re.compile(
    r"^\s*([A-Za-z_][A-Za-z0-9_]*)\s+([\[\]\*A-Za-z0-9_.]+)\s*(?:`([^`]*)`)?\s*$"
)
TAG_JSON_RE = re.compile(r'json:"([^"]*)"')


def _parse_fields(lines: list[str], start: int, end: int) -> tuple[list[dict], int]:
    """Parse fields between start (after the '{') and end (index of closing '}')."""
    fields: list[dict] = []
    i = start
    while i < end:
        line = lines[i]
        m = FIELD_RE.match(line)
        if not m:
            i += 1
            continue
        name, gtype, tag = m.group(1), m.group(2), m.group(3) or ""
        json_name = name
        omit = False
        tm = TAG_JSON_RE.search(tag)
        if tm:
            parts = tm.group(1).split(",")
            json_name = parts[0]
            omit = "omitempty" in parts[1:]
        if json_name in ("", "-"):
            i += 1
            continue
        # Nested anonymous struct on the next lines.
        if gtype == "struct" or line.rstrip().endswith("struct {"):
            depth = 0
            j = i
            while j < end:
                depth += lines[j].count("{") - lines[j].count("}")
                j += 1
                if depth == 0:
                    break
            sub, _ = _parse_fields(lines, i + 1, j - 1)
            fields.append({"name": json_name, "type": "object", "omit": omit, "fields": sub})
            i = j
            continue
        fields.append({"name": json_name, "type": gtype, "omit": omit})
        i += 1
    return fields, i


def parse_package(dirpath: Path) -> dict[str, dict]:
    out: dict[str, dict] = {}
    for path in sorted(dirpath.glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        lines = path.read_text().splitlines()
        i = 0
        while i < len(lines):
            m = TYPE_RE.match(lines[i])
            if not m:
                i += 1
                continue
            name = m.group(1)
            depth = 1
            j = i + 1
            while j < len(lines) and depth > 0:
                depth += lines[j].count("{") - lines[j].count("}")
                j += 1
            fields, _ = _parse_fields(lines, i + 1, j - 1)
            out[name] = {"fields": fields}
            i = j
    return out


def parse_named(dirpath: Path) -> dict[str, str]:
    """Named non-struct types (type Foo string) and their underlying kind."""
    out: dict[str, str] = {}
    for path in sorted(dirpath.glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        for line in path.read_text().splitlines():
            m = NAMED_RE.match(line)
            if m and m.group(2) not in ("struct",):
                out[m.group(1)] = m.group(2)
    return out


def camel(name: str) -> str:
    if not name:
        return name
    if name[0].isupper():
        return name
    return name[0].upper() + name[1:]


class Resolver:
    def __init__(self) -> None:
        self.pkgs: dict[str, dict[str, dict]] = {
            pkg: parse_package(d) for pkg, d in PKG_DIRS.items() if d.is_dir()
        }
        self.named: dict[str, dict[str, str]] = {
            pkg: parse_named(d) for pkg, d in PKG_DIRS.items() if d.is_dir()
        }
        self.names: dict[str, list[str]] = {}
        for pkg, types in self.pkgs.items():
            for t in types:
                self.names.setdefault(t, []).append(pkg)

    def component(self, qualified: str) -> str:
        if "." not in qualified:
            qualified = "httpapi." + qualified
        pkg, name = qualified.split(".", 1)
        # Disambiguate only when the bare type name exists in more than one package.
        if len(self.names.get(name, [])) > 1:
            return camel(pkg) + camel(name)
        return camel(name)

    def ref_for(self, qualified: str) -> dict:
        """A request/response body schema referencing a component."""
        if qualified.startswith("[]"):
            return {"type": "array", "items": {"$ref": f"#/components/schemas/{self.component(qualified[2:])}"}}
        return {"$ref": f"#/components/schemas/{self.component(qualified)}"}

    def struct_of(self, qualified: str) -> dict | None:
        pkg, name = qualified.split(".", 1)
        return self.pkgs.get(pkg, {}).get(name)

    def schema(self, qualified: str, depth: int = 0) -> dict:
        if qualified.startswith("[]"):
            return {"type": "array", "items": self.schema(qualified[2:], depth)}
        pkg, name = qualified.split(".", 1)
        if qualified in SYNTHETIC:
            fields = SYNTHETIC[qualified]
            return {
                "type": "object",
                "properties": {k: _scalar(v) for k, v in fields.items()},
                "required": list(fields),
            }
        st = self.struct_of(qualified)
        if st is None:
            return {"type": "object", "additionalProperties": True}
        return self._schema_for_fields(st["fields"], depth, pkg)

    def _schema_for_fields(self, fields: list[dict], depth: int, pkg: str = "") -> dict:
        props: dict = {}
        required: list[str] = []
        for f in fields:
            if f["type"] == "object":
                props[f["name"]] = self._schema_for_fields(f.get("fields", []), depth + 1, pkg)
            else:
                props[f["name"]] = _go_type_schema(f["type"], self, pkg)
            if not f["omit"]:
                required.append(f["name"])
        out: dict = {"type": "object", "properties": props}
        if required:
            out["required"] = required
        return out


def _scalar(t: str) -> dict:
    return {
        "string": {"type": "string"},
        "int": {"type": "integer", "format": "int64"},
        "int64": {"type": "integer", "format": "int64"},
        "bool": {"type": "boolean"},
        "float64": {"type": "number"},
    }.get(t, {"type": "string"})


def _go_type_schema(t: str, res: Resolver, pkg: str) -> dict:
    ptr = t.startswith("*")
    sch = _go_type_schema_inner(t.lstrip("*"), res, pkg)
    if ptr and isinstance(sch, dict):
        sch = dict(sch)
        sch["x-nullable"] = True
    return sch


def _go_type_schema_inner(t: str, res: Resolver, pkg: str) -> dict:
    t = t.lstrip("*")
    if t.startswith("[]"):
        return {"type": "array", "items": _go_type_schema(t[2:], res, pkg)}
    if t.startswith("map["):
        return {"type": "object", "additionalProperties": True}
    if t in SCALAR_OVERRIDES:
        kind, fmt = SCALAR_OVERRIDES[t]
        return {"type": kind, "format": fmt}
    prim = {
        "string": {"type": "string"},
        "bool": {"type": "boolean"},
        "float64": {"type": "number"},
        "float32": {"type": "number"},
        "int": {"type": "integer", "format": "int64"},
        "int8": {"type": "integer", "format": "int64"},
        "int16": {"type": "integer", "format": "int64"},
        "int32": {"type": "integer", "format": "int64"},
        "int64": {"type": "integer", "format": "int64"},
        "uint": {"type": "integer", "format": "int64"},
        "uint32": {"type": "integer", "format": "int64"},
        "uint64": {"type": "integer", "format": "int64"},
    }
    if t in prim:
        return prim[t]
    # Named type: prefer a struct in the same package, then a struct anywhere,
    # then a named scalar type in the same package, else a plain string.
    if t in res.pkgs.get(pkg, {}):
        return {"$ref": f"#/components/schemas/{pkg}.{t}"}
    if "." in t:
        # A qualified name ("budget.Position") names its package, so resolve it there instead
        # of falling through to a plain string (which is how a response body's nested object
        # used to reach the client as `string`).
        qual, name = t.split(".", 1)
        if name in res.pkgs.get(qual, {}):
            return res.ref_for(t)
        if name in res.named.get(qual, {}):
            return _scalar(res.named[qual][name])
        return {"type": "string"}
    if len(res.names.get(t, [])) == 1:
        return {"$ref": f"#/components/schemas/{res.names[t][0]}.{t}"}
    if t in res.named.get(pkg, {}):
        return _scalar(res.named[pkg][t])
    return {"type": "string"}


def build_document(res: Resolver) -> dict:
    paths: dict = {}
    for path, methods in CONTRACT.items():
        item: dict = {}
        for method, (op_id, tag, summary, req, resp, status, kind) in methods.items():
            op: dict = {"tags": [tag], "operationId": op_id, "summary": summary}
            params = re.findall(r"\{([A-Za-z_][A-Za-z0-9_]*)\}", path)
            if params:
                op["parameters"] = [
                    {
                        "name": p,
                        "in": "path",
                        "required": True,
                        "schema": {"type": "integer", "format": "int64"} if p in ("id", "year") else {"type": "string"},
                    }
                    for p in params
                ]
            if req:
                op["requestBody"] = {
                    "required": True,
                    "content": {
                        "application/json": {"schema": res.ref_for(req)},
                    },
                }
            elif path == "/api/e-invoices/received":
                op["requestBody"] = {
                    "required": True,
                    "content": {"application/xml": {"schema": {"type": "string"}}},
                }
            responses: dict = {}
            if kind == "file":
                responses[str(status)] = {
                    "description": "Binary/XML payload",
                    "content": {
                        "application/xml": {"schema": {"type": "string", "format": "binary"}},
                        "text/plain": {"schema": {"type": "string"}},
                    },
                }
            elif kind == "artefact":
                responses[str(status)] = {
                    "description": "Stored artefact",
                    "content": {
                        "application/pdf": {"schema": {"type": "string", "format": "binary"}},
                        "text/csv": {"schema": {"type": "string", "format": "binary"}},
                        "application/xml": {"schema": {"type": "string", "format": "binary"}},
                    },
                }
            elif kind == "pdf":
                responses[str(status)] = {
                    "description": "PDF document",
                    "content": {"application/pdf": {"schema": {"type": "string", "format": "binary"}}},
                }
            elif kind == "text":
                responses[str(status)] = {
                    "description": "Success",
                    "content": {"text/plain": {"schema": {"type": "string"}}},
                }
            else:
                responses[str(status)] = {
                    "description": "Success",
                    "content": {"application/json": {"schema": res.ref_for(resp)}},
                }
            responses["401"] = {"$ref": "#/components/responses/Unauthorized"}
            responses["403"] = {"$ref": "#/components/responses/Forbidden"}
            responses["400"] = {"$ref": "#/components/responses/BadRequest"}
            responses["404"] = {"$ref": "#/components/responses/NotFound"}
            responses["409"] = {"$ref": "#/components/responses/Conflict"}
            responses["422"] = {"$ref": "#/components/responses/Unprocessable"}
            responses["500"] = {"$ref": "#/components/responses/Internal"}
            op["responses"] = responses
            item[method] = op
        paths[path] = item

    # Collect schemas transitively: start from every request/response type and
    # follow $refs so nested structs (Line, StatementLine, ...) get emitted too.
    queue: list[str] = []
    for methods in CONTRACT.values():
        for m in methods.values():
            for t in (m[3], m[4]):
                if t:
                    queue.append(t.lstrip("[]"))
    queue.extend(SYNTHETIC.keys())
    seen: set[str] = set()
    schemas: dict = {}
    while queue:
        qualified = queue.pop()
        if qualified in seen or "." not in qualified:
            continue
        seen.add(qualified)
        schema = res.schema(qualified)
        schemas[res.component(qualified)] = schema
        for ref in _refs_in(schema):
            if ref not in seen:
                queue.append(ref)

    schemas["Problem"] = {
        "type": "object",
        "properties": {
            "error": {"type": "string"},
            "code": {"type": "string"},
            "request_id": {"type": "string"},
        },
        "required": ["error"],
    }

    return {
        "openapi": "3.1.0",
        "info": {
            "title": "campus-erp API",
            "version": "0.22.0",
            "description": (
                "Contract-first API for university-ERP replacement (ADR-0003). "
                "Generated from tools/gen_openapi.py (route table + Go structs); the Angular "
                "client is generated from this document by tools/api_contract.py. Tenant "
                "resolution is hostname-based (ADR-0002), never a path segment."
            ),
        },
        "servers": [{"url": "https://{tenant}.erp.example.org", "variables": {"tenant": {"default": "demo"}}}],
        "tags": [
            {"name": "platform", "description": "Health and audit endpoints."},
            {"name": "kasse", "description": "Cash desk, payment traffic, bank reconciliation."},
            {"name": "einvoice", "description": "XRechnung 3.0 / EN16931 e-invoicing."},
            {"name": "budget", "description": "Budget positions and Mittelueberwachung."},
            {"name": "lookup", "description": "Central lookup catalogue."},
            {"name": "procurement", "description": "Purchase orders and budget commitment."},
            {"name": "klr", "description": "Cost and performance accounting."},
        ],
        "security": [{"bearerAuth": []}],
        "paths": paths,
        "components": {
            "schemas": schemas,
            "responses": {
                "Unauthorized": _problem("Authentication required (missing or invalid bearer token)"),
                "Forbidden": _problem("Authenticated but not permitted"),
                "BadRequest": _problem("Malformed request"),
                "NotFound": _problem("Resource not found"),
                "Conflict": _problem("State conflict"),
                "Unprocessable": _problem("Domain rule violated"),
                "Internal": _problem("Internal error"),
            },
            "securitySchemes": {
                "bearerAuth": {
                    "type": "http",
                    "scheme": "bearer",
                    "bearerFormat": "JWT",
                    "description": "OIDC access token from the university IdP (ADR-0003).",
                }
            },
        },
    }


def _refs_in(schema) -> list[str]:
    """Return qualified type keys referenced by a schema fragment."""
    out: list[str] = []

    def walk(node):
        if isinstance(node, dict):
            for k, v in node.items():
                if k == "$ref" and isinstance(v, str) and v.startswith("#/components/schemas/"):
                    out.append(v.split("/")[-1])
                else:
                    walk(v)
        elif isinstance(node, list):
            for item in node:
                walk(item)

    walk(schema)
    return out


def _localize_refs(node, res):
    """Rewrite qualified $refs (#/components/schemas/pkg.Type) to component names."""
    if isinstance(node, dict):
        for k, v in list(node.items()):
            if k == "$ref" and isinstance(v, str) and v.startswith("#/components/schemas/"):
                key = v.split("/")[-1]
                if "." in key:
                    node[k] = "#/components/schemas/" + res.component(key)
            else:
                _localize_refs(v, res)
    elif isinstance(node, list):
        for item in node:
            _localize_refs(item, res)


def _problem(desc: str) -> dict:
    return {
        "description": desc,
        "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Problem"}}},
    }


def render() -> str:
    res = Resolver()
    doc = build_document(res)
    _localize_refs(doc, res)
    text = yaml.safe_dump(doc, sort_keys=False, allow_unicode=True, width=10_000, default_flow_style=False)
    header = (
        "# GENERATED by tools/gen_openapi.py — do not edit by hand.\n"
        "# Change the CONTRACT table or the Go structs, then re-run:\n"
        "#   /usr/bin/python3 tools/gen_openapi.py\n"
    )
    return header + text


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--stdout", action="store_true")
    ap.add_argument("--check", action="store_true", help="fail if the committed spec differs")
    args = ap.parse_args()

    text = render()
    if args.stdout:
        sys.stdout.write(text)
        return 0
    if args.check:
        current = SPEC.read_text() if SPEC.exists() else ""
        if current != text:
            print("contracts/openapi.yaml is stale — run /usr/bin/python3 tools/gen_openapi.py", file=sys.stderr)
            return 1
        print("contracts/openapi.yaml is up to date")
        return 0
    SPEC.parent.mkdir(parents=True, exist_ok=True)
    SPEC.write_text(text)
    doc = yaml.safe_load(text)
    ops = sum(len(v) for v in doc["paths"].values())
    print(f"wrote contracts/openapi.yaml ({len(text.splitlines())} lines, "
          f"{len(doc['paths'])} paths, {ops} operations, "
          f"{len(doc['components']['schemas'])} schemas)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
