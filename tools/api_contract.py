#!/usr/bin/env python3
"""API contract tool: drift guard + Angular client generator (ADR-0003).

The OpenAPI document in contracts/openapi.yaml is the interface between the Go
backend and the Angular frontend. This tool keeps that promise honest and
useful:

  check   Verify the committed spec matches the Go HTTP surface in both
          directions (every Go route is documented; every documented route
          exists in Go), that operationIds are present and unique, and that
          every $ref resolves. Fails the build on any drift.

  gen     Generate the Angular client (models + a typed HttpClient service)
          from the spec into frontend/src/app/core/api/generated/. The client
          is generated, never hand-written (ADR-0003), so it is excluded from
          the mutation/policy gates.

  selftest  Exercise the parser and generator on fixtures (run in the quality
          suite).

It is stdlib-only on purpose: CI must run it with any python3, with no YAML
package, no JVM and no network. It parses the constrained subset of YAML that
tools/gen_openapi.py emits.
"""
from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SPEC = ROOT / "contracts" / "openapi.yaml"
SERVER_GO = ROOT / "backend" / "internal" / "httpapi" / "server.go"
GEN_DIR = ROOT / "frontend" / "src" / "app" / "core" / "api" / "generated"

ROUTE_RE = re.compile(r'mux\.HandleFunc\("(GET|POST|PUT|PATCH|DELETE) ([^"]+)"')

# ─────────────────────────────────────────────────────────────────────────────
# A tiny YAML subset parser (stdlib only).
# Supports: nested mappings, block sequences ("- x", "- k: v"), flow sequences
# ("[a, b]"), quoted/plain scalars, block scalars ("|", ">") and comments.
# ─────────────────────────────────────────────────────────────────────────────


class YAMLError(ValueError):
    pass


def _scalar(tok: str):
    tok = tok.strip()
    if tok == "":
        return None
    if tok[0] in "'\"":
        if len(tok) >= 2 and tok[-1] == tok[0]:
            return tok[1:-1]
        return tok[1:]
    if tok.startswith("[") and tok.endswith("]"):
        inner = tok[1:-1].strip()
        if not inner:
            return []
        return [_scalar(x) for x in _split_flow(inner)]
    low = tok.lower()
    if low in ("null", "~", ""):
        return None
    if low == "true":
        return True
    if low == "false":
        return False
    if re.fullmatch(r"-?\d+", tok):
        return int(tok)
    if re.fullmatch(r"-?\d+\.\d+", tok):
        return float(tok)
    return tok


def _split_flow(s: str) -> list[str]:
    out, depth, cur = [], 0, ""
    for ch in s:
        if ch == "[":
            depth += 1
        elif ch == "]":
            depth -= 1
        if ch == "," and depth == 0:
            out.append(cur)
            cur = ""
        else:
            cur += ch
    if cur.strip():
        out.append(cur)
    return out


def _indent(line: str) -> int:
    return len(line) - len(line.lstrip(" "))


class _Parser:
    def __init__(self, lines: list[str]):
        self.lines = lines
        self.i = 0

    def _peek(self):
        while self.i < len(self.lines):
            raw = self.lines[self.i]
            stripped = raw.strip()
            if stripped == "" or stripped.startswith("#"):
                self.i += 1
                continue
            return raw
        return None

    def parse_block(self, indent: int):
        raw = self._peek()
        if raw is None:
            return None
        if raw.strip().startswith("- "):
            return self.parse_sequence(indent)
        return self.parse_mapping(indent)

    def parse_sequence(self, indent: int) -> list:
        out = []
        while True:
            raw = self._peek()
            if raw is None or _indent(raw) != indent or not raw.strip().startswith("- "):
                break
            content = raw.strip()[2:]
            self.i += 1
            if ":" in content and not content.lstrip()[0] in "'\"":
                # inline map item: "- key: value" possibly with deeper siblings
                key, _, rest = content.partition(":")
                item = {}
                if rest.strip() in ("|", ">"):
                    item[key.strip()] = self._block_scalar(indent, rest.strip())
                else:
                    item[key.strip()] = _scalar(rest)
                # deeper keys belonging to the same item
                extra = self._peek()
                if extra is not None and _indent(extra) > indent:
                    more = self.parse_mapping(_indent(extra))
                    if isinstance(more, dict):
                        item.update(more)
                out.append(item)
            else:
                out.append(_scalar(content))
        return out

    def parse_mapping(self, indent: int) -> dict:
        out = {}
        while True:
            raw = self._peek()
            if raw is None or _indent(raw) != indent:
                break
            line = raw.strip()
            if line.startswith("- "):
                break
            key, sep, rest = line.partition(":")
            if not sep:
                self.i += 1
                continue
            key = key.strip().strip("'\"")
            self.i += 1
            rest = rest.strip()
            if rest in ("|", ">"):
                out[key] = self._block_scalar(indent, rest)
            elif rest == "":
                nxt = self._peek()
                if nxt is not None and _indent(nxt) > indent:
                    out[key] = self.parse_block(_indent(nxt))
                elif nxt is not None and _indent(nxt) == indent and nxt.strip().startswith("- "):
                    out[key] = self.parse_sequence(indent)
                else:
                    out[key] = None
            else:
                out[key] = _scalar(rest)
        return out

    def _block_scalar(self, indent: int, style: str) -> str:
        collected = []
        while self.i < len(self.lines):
            raw = self.lines[self.i]
            if raw.strip() == "":
                collected.append("")
                self.i += 1
                continue
            if _indent(raw) <= indent:
                break
            collected.append(raw[indent + 2:])
            self.i += 1
        text = "\n".join(collected).rstrip("\n")
        return text if style == "|" else " ".join(l.strip() for l in text.splitlines())


def load_yaml(text: str):
    return _Parser(text.splitlines()).parse_block(0) or {}


# ─────────────────────────────────────────────────────────────────────────────
# Drift guard
# ─────────────────────────────────────────────────────────────────────────────


def go_routes() -> set[tuple[str, str]]:
    if not SERVER_GO.exists():
        raise SystemExit(f"missing {SERVER_GO}")
    return {(m.lower(), p) for m, p in ROUTE_RE.findall(SERVER_GO.read_text())}


def spec_ops(doc: dict) -> dict[tuple[str, str], dict]:
    ops: dict[tuple[str, str], dict] = {}
    for path, item in (doc.get("paths") or {}).items():
        for method, op in (item or {}).items():
            ops[(method.lower(), path)] = op or {}
    return ops


def collect_refs(node, out: set[str]) -> None:
    if isinstance(node, dict):
        for k, v in node.items():
            if k == "$ref" and isinstance(v, str):
                out.add(v)
            else:
                collect_refs(v, out)
    elif isinstance(node, list):
        for x in node:
            collect_refs(x, out)


def cmd_check() -> int:
    doc = load_yaml(SPEC.read_text())
    routes = go_routes()
    ops = spec_ops(doc)
    problems: list[str] = []

    missing = sorted(routes - set(ops))
    extra = sorted(set(ops) - routes)
    for m, p in missing:
        problems.append(f"route in Go but not documented: {m.upper()} {p}")
    for m, p in extra:
        problems.append(f"documented but not in Go: {m.upper()} {p}")

    ids: list[str] = []
    for (m, p), op in sorted(ops.items()):
        if not op.get("operationId"):
            problems.append(f"missing operationId: {m.upper()} {p}")
        else:
            ids.append(op["operationId"])
        if not op.get("responses"):
            problems.append(f"no responses: {m.upper()} {p}")
    dupes = sorted({i for i in ids if ids.count(i) > 1})
    for d in dupes:
        problems.append(f"duplicate operationId: {d}")

    sch = set((doc.get("components") or {}).get("schemas") or {})
    resp = set((doc.get("components") or {}).get("responses") or {})
    refs: set[str] = set()
    collect_refs(doc, refs)
    for r in sorted(refs):
        target = r.rsplit("/", 1)[-1]
        section = r.rsplit("/", 2)[-2] if r.count("/") >= 2 else ""
        if section == "responses":
            if target not in resp:
                problems.append(f"dangling response ref: {r}")
        elif target not in sch:
            problems.append(f"dangling schema ref: {r}")

    print(f"contract: {len(ops)} documented operations, {len(routes)} Go routes, "
          f"{len(sch)} schemas")
    if problems:
        for p in problems:
            print(f"  FAIL  {p}")
        print(f"contract check FAILED ({len(problems)} problem(s))")
        return 1
    print("contract check OK")
    return 0


# ─────────────────────────────────────────────────────────────────────────────
# Angular client generator
# ─────────────────────────────────────────────────────────────────────────────

TS_PRIM = {
    "string": "string",
    "integer": "number",
    "number": "number",
    "boolean": "boolean",
}


def ts_type(schema: dict, indent: str = "") -> str:
    if schema is None:
        return "unknown"
    if "$ref" in schema:
        return schema["$ref"].rsplit("/", 1)[-1]
    t = schema.get("type")
    if t == "array":
        inner = ts_type(schema.get("items") or {}, indent) + "[]"
        return inner + " | null" if schema.get("x-nullable") else inner
    if t == "object":
        props = schema.get("properties") or {}
        if not props:
            return "Record<string, unknown>"
        return "unknown"
    base = TS_PRIM.get(t, "unknown")
    return base + " | null" if schema.get("x-nullable") else base


def gen_models(doc: dict) -> str:
    schemas = (doc.get("components") or {}).get("schemas") or {}
    lines = [
        "/* GENERATED by tools/api_contract.py — do not edit. */",
        "/* angular-client: models */",
        "",
    ]
    for name in sorted(schemas):
        schema = schemas[name]
        if schema.get("type") != "object":
            lines.append(f"export type {name} = {ts_type(schema)};")
            lines.append("")
            continue
        lines.append(f"export interface {name} {{")
        required = set(schema.get("required") or [])
        for prop, pschema in (schema.get("properties") or {}).items():
            opt = "" if prop in required else "?"
            lines.append(f"  {prop}{opt}: {ts_type(pschema, '  ')};")
        lines.append("}")
        lines.append("")
    return "\n".join(lines)


def _ts_param(name: str) -> str:
    return re.sub(r"[^A-Za-z0-9_]", "_", name)


def gen_service(doc: dict) -> str:
    paths = doc.get("paths") or {}
    tags: dict[str, list[str]] = {}
    for path, item in paths.items():
        for method, op in item.items():
            for tag in op.get("tags") or ["default"]:
                tags.setdefault(tag, []).append(op.get("operationId", ""))
    tags = {t: sorted(set(v)) for t, v in tags.items()}

    lines = [
        "/* GENERATED by tools/api_contract.py — do not edit. */",
        "/* angular-client: service */",
        "import { HttpClient } from '@angular/common/http';",
        "import { Injectable } from '@angular/core';",
        "import { Observable } from 'rxjs';",
        "import * as m from './models';",
        "",
        "@Injectable({ providedIn: 'root' })",
        "export class CampusErpApi {",
        "  private readonly base = '';",
        "",
        "  constructor(private readonly http: HttpClient) {}",
        "",
    ]
    any_query = False
    for path, item in sorted(paths.items()):
        for method, op in sorted(item.items()):
            op_id = op.get("operationId", "unnamed")
            declared = op.get("parameters") or []
            path_params = [p["name"] for p in declared if p.get("in") == "path"]
            query_params = [p for p in declared if p.get("in") == "query"]
            args = []
            for p in path_params:
                args.append(f"{_ts_param(p)}: number | string")
            for q in query_params:
                # Query values travel as strings, so an optional filter reads as `status?: string`.
                optional = "" if q.get("required") else "?"
                args.append(f"{_ts_param(q['name'])}{optional}: string")
            body_ref = _body_schema(op)
            raw_body = body_ref is None and _has_raw_body(op)
            if body_ref:
                args.append(f"body: m.{body_ref}")
            elif raw_body:
                # A route that takes a raw document (the received XRechnung, say) has no schema
                # to type the argument with, but it still takes one: sending a null body would
                # make the route unreachable through the client.
                args.append("body: string")
            ret = _response_type(op)
            # A response that is not JSON (a stored file) must be read as text: Angular parses a
            # `json` response blindly, and an XML body would throw instead of arriving as a string.
            text_response = _response_is_text(op)
            # turn {name} into ${name}
            url_expr = re.sub(r"\{([A-Za-z0-9_]+)\}", r"${\1}", path)
            if query_params:
                any_query = True
                pairs = ", ".join(q["name"] for q in query_params)
                url_expr += "$" + "{this._query({ " + pairs + " })}"
            if raw_body:
                body_arg = ", body, { headers: { 'Content-Type': 'application/xml' } }"
            else:
                body_arg = ", body" if body_ref else (", null" if method in ("post", "put", "patch") else "")
            if text_response:
                body_arg = (body_arg + ", " if body_arg else ", ") + "{ responseType: 'text' }"
            arg_list = (", ".join(args)) if args else ""
            # The `text` response overload of HttpClient takes no type argument: `get<T>(url, ...)`
            # only accepts `responseType: 'json'`, so a file read needs the string overload.
            generic = "" if text_response else f"<{ret}>"
            lines.append(f"  {op_id}({arg_list}): Observable<{ret}> {{")
            lines.append(f"    return this.http.{method}{generic}(this.base + `{url_expr}`{body_arg});")
            lines.append("  }")
            lines.append("")
    if any_query:
        # One helper, used by every method that takes filters. Empty and undefined values drop
        # out, so an omitted filter is simply not sent.
        lines.append("  /** Renders the query string for the parameters that were given. */")
        lines.append("  private _query(params: Record<string, string | number | undefined>): string {")
        lines.append("    const parts = Object.entries(params)")
        lines.append("      .filter(([, value]) => value !== undefined && value !== '')")
        lines.append("      .map(([key, value]) => `${key}=${encodeURIComponent(String(value))}`);")
        lines.append("    return parts.length > 0 ? '?' + parts.join('&') : '';")
        lines.append("  }")
        lines.append("")
    lines.append("}")
    lines.append("")
    return "\n".join(lines)


def _has_raw_body(op: dict) -> bool:
    """True when the operation takes a body that is not a JSON schema reference."""
    rb = op.get("requestBody")
    if not rb:
        return False
    for _mt, media in (rb.get("content") or {}).items():
        schema = media.get("schema") or {}
        if schema and not schema.get("$ref"):
            return True
    return False


def _body_schema(op: dict) -> str | None:
    rb = op.get("requestBody")
    if not rb:
        return None
    for _mt, media in (rb.get("content") or {}).items():
        ref = (media.get("schema") or {}).get("$ref")
        if ref:
            return ref.rsplit("/", 1)[-1]
    return None


def _response_type(op: dict) -> str:
    for status in sorted((op.get("responses") or {}), key=lambda s: str(s)):
        resp = op["responses"][status]
        if not isinstance(resp, dict) or "$ref" in resp:
            continue
        for _mt, media in ((resp or {}).get("content") or {}).items():
            return _schema_ts(media.get("schema") or {})
    return "void"


def _response_is_text(op: dict) -> bool:
    """Whether the response is not JSON — a stored file the client must read as text."""
    for status in sorted((op.get("responses") or {}), key=lambda s: str(s)):
        resp = op["responses"][status]
        if not isinstance(resp, dict) or "$ref" in resp:
            continue
        for media_type in ((resp or {}).get("content") or {}):
            return "json" not in media_type
    return False


def _schema_ts(schema: dict) -> str:
    if "$ref" in schema:
        base = "m." + schema["$ref"].rsplit("/", 1)[-1]
        return base + " | null" if schema.get("x-nullable") else base
    if schema.get("type") == "array":
        base = _schema_ts(schema.get("items") or {}) + "[]"
        return base + " | null" if schema.get("x-nullable") else base
    if schema.get("type") == "string":
        return "string"
    if schema.get("type") == "integer" or schema.get("type") == "number":
        return "number"
    if schema.get("type") == "boolean":
        return "boolean"
    return "void"


def cmd_gen() -> int:
    doc = load_yaml(SPEC.read_text())
    GEN_DIR.mkdir(parents=True, exist_ok=True)
    (GEN_DIR / "models.ts").write_text(gen_models(doc))
    (GEN_DIR / "api.service.ts").write_text(gen_service(doc))
    (GEN_DIR / "index.ts").write_text(
        "/* GENERATED by tools/api_contract.py — do not edit. */\n"
        "export * from './models';\n"
        "export * from './api.service';\n"
    )
    print(f"generated Angular client in {GEN_DIR.relative_to(ROOT)}")
    return 0


# ─────────────────────────────────────────────────────────────────────────────
# Self-test
# ─────────────────────────────────────────────────────────────────────────────


def cmd_selftest() -> int:
    failures = 0

    def check(cond, msg):
        nonlocal failures
        if cond:
            print(f"  ok    {msg}")
        else:
            print(f"  FAIL  {msg}")
            failures += 1

    sample = """
paths:
  /healthz:
    get:
      operationId: getHealth
      responses:
        '200':
          description: ok
  /api/x:
    get:
      operationId: listX
      parameters:
        - name: status
          in: query
          required: false
          schema:
            type: string
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: '#/components/schemas/X'
  /api/x/{id}:
    post:
      operationId: doX
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: '#/components/schemas/X'
      responses:
        '201':
          description: created
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/X'
components:
  schemas:
    X:
      type: object
      required: [a]
      properties:
        a:
          type: string
        b:
          type: integer
        tags:
          type: array
          items:
            type: string
"""
    doc = load_yaml(sample)
    check(set((doc.get("paths") or {})) == {"/healthz", "/api/x", "/api/x/{id}"}, "parses paths")
    check(doc["paths"]["/api/x/{id}"]["post"]["operationId"] == "doX", "parses operationId")
    check(doc["components"]["schemas"]["X"]["required"] == ["a"], "parses required list")
    check(ts_type({"$ref": "#/components/schemas/X"}) == "X", "ref -> type name")
    check(ts_type({"type": "array", "items": {"type": "string"}}) == "string[]", "array type")
    models = gen_models(doc)
    check("export interface X" in models and "a: string" in models and "b?: number" in models,
          "models interface")
    service = gen_service(doc)
    check("doX(" in service and "${id}" in service, "service method + path param")
    check("listX(status?: string)" in service, "query param is optional and typed as a string")
    check("this._query({ status })" in service, "query params are rendered into the URL")
    check("private _query(" in service, "one query helper is emitted")
    check("responseType" not in service, "a JSON response is not forced to text")

    # A non-JSON response (a stored file) must be read as text: Angular parses a json response
    # blindly, so an XML body would throw instead of arriving as a string.
    file_doc = {"paths": {"/f": {"get": {"operationId": "getF", "tags": ["f"], "responses": {
        "200": {"content": {"application/xml": {"schema": {"type": "string"}}}}}}}}}
    file_service = gen_service(file_doc)
    check("Observable<string>" in file_service and "responseType: 'text'" in file_service
          and "http.get<string>" not in file_service,
          "a non-JSON response is read as text (string overload, no type argument)")

    # The real, committed spec must also parse and stay consistent.
    real = load_yaml(SPEC.read_text())
    check(len(real.get("paths") or {}) > 40, "real spec parses with many paths")

    if failures:
        print(f"selftest FAILED ({failures})")
        return 1
    print("selftest OK")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sub.add_parser("check")
    sub.add_parser("gen")
    sub.add_parser("selftest")
    args = ap.parse_args()
    if args.cmd == "check":
        return cmd_check()
    if args.cmd == "gen":
        return cmd_gen()
    return cmd_selftest()


if __name__ == "__main__":
    raise SystemExit(main())
