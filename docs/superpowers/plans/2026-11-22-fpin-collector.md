# FPIN Collector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collect Unisphere 10.5 FPIN (SAN fabric congestion) events as per-port Prometheus/OTLP gauges and show them in the overview dashboard.

**Architecture:** A new `FPIN` `ResourceCollector` GETs the Enhanced Endpoints `/fpins` list per array, aggregates by (director, port, fpin_type), and emits two gauges. It is silent (no samples, no error) on Unisphere < 10.5 or when the endpoint 404s. Requires a typed HTTP status error in `pmaxclient` so a 404 is distinguishable.

**Tech Stack:** Go 1.27, `resty/v2` client, `pmaxclient.Mock`, Grafana dashboard JSON.

**Spec:** `docs/superpowers/specs/2026-11-22-fpin-collector-design.md` (approach A). Endpoint schemas: `docs/swagger/14406-10.5.0.json`.

## Global Constraints

- Metric prefix `pmax_`; gauges only; absent, never zero (ADR-0009): a port with no FPINs gets no series.
- One label-key set per family (ADR-0006): `server` (stamped by the loop), `array`, `director`, `port`, `fpin_type` — identical on both FPIN families.
- Enhanced Endpoints root is `/univmax/rest` (NOT `RestBase` = `/univmax/restapi`); path `/v1/systems/{array}/fpins`.
- Collector `Name()` is `fpin` (appears as `pmax_collector_up{collector="fpin"}`).
- Decode tolerantly: response list key is `results` per schema but `fpins` per the spec example — accept both. JSON decoding is case-insensitive in Go; the spec's `Fpin` schema uses mixed-case property names (`detected_Time`, `director_Port`, `fpin_Type`) — the spec gate must compare lower-cased.
- No `nolint`/semgrep suppressions. Never log credentials.
- Tests go through both the Prometheus registry gather and the OTLP reader only where an existing collector test already does; collector unit tests use `pmaxclient.Mock`.
- `make ci` must pass; dashboards reference only emitted `pmax_*` names (enforced by `TestDashboardRefsEmitted`).

## Review Focus

- Pre-10.5 server (`APIVersion()` < 105) → zero samples, nil error, endpoint never called.
- Endpoint returns 404 on a 10.5+ server (feature absent) → zero samples, nil error.
- Non-404 failure (500, 401) → collector returns the error (turns `collector_up` red).
- Empty list / port with no events → no series (absent, not 0).
- Paginated list (`paging.resume_token`) is fully read, bounded by a page cap, truncation logged.
- `director_port` without `:` or empty, and a missing `director` object → no panic; sample still emitted with best-effort labels or skipped (see Task 2).

---

### Task 1: Typed HTTP status error and richer Mock

**Files:**
- Modify: `internal/pmaxclient/server.go` (Get/Post error return), `internal/pmaxclient/mock.go`
- Test: `internal/pmaxclient/server_test.go`, `internal/pmaxclient/mock_test.go`

**Interfaces:**
- Produces:
  - `type StatusError struct { Method, Path string; Code int }` with `Error() string` returning the existing text format (`"GET /p: status 404"`).
  - `func IsNotFound(err error) bool` — true if `errors.As` finds a `*StatusError` with `Code == 404`.
  - `func (m *Mock) SetAPIVersion(v string)` — `APIVersion()` returns `v`, default `"100"`.
  - `func (m *Mock) SetGetError(path string, err error)` — `Get(path)` returns `err`.

- [ ] **Step 1: Write failing tests.**
  - `TestStatusErrorIsNotFound` (server_test.go): handler returns 404 for `/missing`, 500 for `/boom`; `c.Get` on `/missing` → `IsNotFound(err)` true and `err.Error()` still equals `"GET /missing: status 404"`; on `/boom` (after retries) → `IsNotFound` false. `IsNotFound(nil)` false; `IsNotFound(fmt.Errorf("x"))` false.
  - `TestMockSetAPIVersionAndGetError` (mock_test.go): default `APIVersion()=="100"`; after `SetAPIVersion("105")` returns `"105"`; `SetGetError("/p", sentinel)` makes `Get` return an error satisfying `errors.Is(err, sentinel)`.
- [ ] **Step 2:** `go test ./internal/pmaxclient/ -run 'StatusError|MockSetAPIVersion'` → FAIL (undefined).
- [ ] **Step 3:** Implement the signatures above in `server.go`/`mock.go`; replace the two `fmt.Errorf("GET/POST %s: status %d", …)` returns with `&StatusError{…}` (error text unchanged so existing assertions hold). Mock `Get` checks the error map before the body map.
- [ ] **Step 4:** `go test ./internal/pmaxclient/` → PASS.
- [ ] **Step 5:** Commit `feat(client): typed StatusError, Mock API version and error injection`.

---

### Task 2: FPIN collector

**Files:**
- Create: `internal/pmax/fpin.go`, `internal/pmax/fpin_test.go`
- Modify: `internal/pmax/resource.go` (add `EnhancedBase` const; register `FPIN{}` after `SRPCapacity{}` in `Registry`)

**Interfaces:**
- Consumes: `pmaxclient.Client`, `pmaxclient.IsNotFound`, `Mock.SetAPIVersion`, `Mock.SetGetError` (Task 1); `ArrayWindow`, `Sample`, `Label` (existing).
- Produces:
  - `const EnhancedBase = "/univmax/rest"` in `resource.go`.
  - `type FPIN struct{}` implementing `ResourceCollector`; `Name()` = `"fpin"`.
  - Emits `pmax_fpin_events` (count) and `pmax_fpin_last_detected_timestamp_seconds` (max `detected_time`/1000), labels `array, director, port, fpin_type`.

**Behavior decisions (from spec):**
- Gate: `strconv.Atoi(c.APIVersion())`; if it parses and is `< 105` → `return nil, nil`. Unparseable → proceed.
- URL: `EnhancedBase + "/v1/systems/" + a.ID + "/fpins?select=director_port,fpin_type,detected_time,director"`; subsequent pages append `&resume_token=<token>` (URL-escaped) taken from `paging.resume_token`; stop when empty. Page cap constant `fpinMaxPages = 20`; hitting it logs a warning (logrus, with `array`) and keeps what was read.
- Decode both `results` and `fpins` arrays (concatenate); item fields: `director_port` string, `fpin_type` string, `detected_time` `*float64`, `director.id` string.
- Labels: `director` = `director.id`, else `director_port` before `:`; `port` = `director_port` after `:`; items with neither a director nor a port are skipped; empty `fpin_type` → skipped.
- `IsNotFound(err)` on any page → return samples so far as none: `return nil, nil` (feature absent). Other errors → return `out, err`.
- Aggregation key = (director, port, fpin_type); sample order sorted by key for determinism.

- [ ] **Step 1: Write failing tests** in `fpin_test.go` (Mock path keys include the query string exactly as built above):
  - `TestFPINSkipsBefore105`: `SetAPIVersion("104")`, no GET registered → `Collect` returns 0 samples, nil error (an unregistered GET would error, proving the endpoint is never called).
  - `TestFPINNotFoundIsSilent`: version 105, `SetGetError(path, &pmaxclient.StatusError{Method:"GET", Path:path, Code:404})` → 0 samples, nil error.
  - `TestFPINOtherErrorFails`: same with `Code: 500` → non-nil error.
  - `TestFPINAggregatesByPortAndType`: body `{"results":[{"director_port":"FA-1D:4","fpin_type":"Congestion","detected_time":1708451234000,"director":{"id":"FA-1D"}},{"director_port":"FA-1D:4","fpin_type":"Congestion","detected_time":1708451299000},{"director_port":"FA-2D:5","fpin_type":"Delivery","detected_time":1708000000000}]}` → `pmax_fpin_events{director=FA-1D,port=4,fpin_type=Congestion}`=2, `pmax_fpin_last_detected_timestamp_seconds` for it = 1708451299, second key has events 1; labels include `array`.
  - `TestFPINAcceptsFpinsKey`: same data under `"fpins"` → same samples.
  - `TestFPINEmptyListEmitsNothing`: `{"results":[]}` → 0 samples.
  - `TestFPINFollowsResumeToken`: page 1 returns `"paging":{"resume_token":"tok1"}`, page 2 (path with `&resume_token=tok1`) returns the rest → counts sum across pages.
  - `TestFPINMalformedItemsSkipped`: items with `director_port:""` and no director, or empty `fpin_type` → skipped, no panic.
- [ ] **Step 2:** `go test ./internal/pmax/ -run FPIN` → FAIL.
- [ ] **Step 3:** Implement `fpin.go` to the decisions above; add `EnhancedBase` and the Registry entry. Add `TestRegistryIncludesFPIN` asserting a collector named `fpin` is in `Registry(1, VolumeOptions{})`.
- [ ] **Step 4:** `go test -race ./internal/pmax/` → PASS (existing tests unaffected: other collectors' Registry-driven tests must still pass — if any counts collectors, update the count).
- [ ] **Step 5:** Commit `feat(fpin): collect Unisphere 10.5 FPIN events per port`.

---

### Task 3: Spec gate for the Enhanced FPIN endpoint

**Files:**
- Modify: `internal/pmax/catalog_spec_helpers_test.go` (add `enhancedSpecPath = "../../docs/swagger/14406-10.5.0.json"`), `internal/pmax/catalog_spec_test.go`

**Interfaces:**
- Produces: `TestFPINInSpec`.

- [ ] **Step 1/3: Write `TestFPINInSpec`** (it passes once Task 2's field set is fixed; it is the gate going forward): read the 14406 spec; assert `paths["/v1/systems/{id}/fpins"].get` exists; assert `components.schemas.Fpin.properties` contains, compared via `strings.ToLower` on the spec keys, each of `director_port`, `fpin_type`, `detected_time`, `director`; assert `components.schemas.pagingInfoType.properties` contains `resume_token`. Failures via `reportf`.
- [ ] **Step 2/4:** `go test ./internal/pmax/ -run FPINInSpec -v` → PASS. Temporarily misspell one expected field to confirm the test fails, then revert.
- [ ] **Step 5:** Commit `test: gate FPIN endpoint and fields against the 10.5 enhanced spec`.

---

### Task 4: Dashboard row, metrics docs, changelog, ADR, CLAUDE.md

**Files:**
- Modify: `grafana/dashboards/pmax-overview.json`, `docs/metrics.md`, `CHANGELOG.md`, `docs/adr/0015-unisphere-10.5-spec-and-api-version-detection.md`, `CLAUDE.md`

**Interfaces:**
- Consumes: metric names/labels from Task 2.

- [ ] **Step 1: Dashboard.** In `pmax-overview.json` insert, between the "Back-end & replication"/"Storage groups" rows and "Capacity & efficiency" (row id 105, y=29), a collapsed row `"SAN fabric (FPIN)"` (id 106) holding two panels (ids 70, 71; ids above the current max 105 are NOT required — pick unused ids), shifting later rows' `gridPos.y` by 1:
  - timeseries "FPIN events by type": `sum by (fpin_type) (pmax_fpin_events{server=~"$server", array=~"$array"})`, legend `{{fpin_type}}`.
  - table "FPIN by port": instant queries `pmax_fpin_events{…}` and `time() - pmax_fpin_last_detected_timestamp_seconds{…}`, merged by `array,director,port,fpin_type` (`merge` transformation), unit `s` on the age column.
  Follow the existing panel JSON shape (datasource `{"type":"prometheus","uid":"prometheus"}`, `$server`/`$array` filters). Validate with `jq . grafana/dashboards/pmax-overview.json`.
- [ ] **Step 2: Docs.** `docs/metrics.md`: add the two metrics (labels `server, array, director, port, fpin_type`; source `GET /univmax/rest/v1/systems/{id}/fpins`, Unisphere ≥ 10.5, silent below). CHANGELOG `### Added` entry. ADR-0015: add a Decision item "FPIN adopted (collector `fpin`); real-time endpoints remain out" and drop FPIN from the "not adopted" list. CLAUDE.md architecture: one line on the FPIN collector and the `/univmax/rest` Enhanced root.
- [ ] **Step 3: Verify.** `go test -race ./...` (includes `TestDashboardRefsEmitted`), `make ci`, `make docs` (strict) → all pass; `helm lint charts/pmax-exporter` unaffected.
- [ ] **Step 4:** Commit `feat(dashboard): SAN fabric (FPIN) row; document FPIN metrics`.

---

## Self-Review

- **Spec coverage:** collector + gate + 404 + pagination + both keys + labels (Tasks 1–2); path/fields spec gate (3); dashboard row, metrics.md, CHANGELOG, ADR-0015, CLAUDE.md (4). Spec "non-goals" (initiator label, rates) intentionally absent.
- **Types consistent:** `StatusError`/`IsNotFound`/`SetAPIVersion`/`SetGetError` defined in Task 1 and used verbatim in Task 2; `EnhancedBase` and `FPIN{}` defined in Task 2.
- **Open risk (not a task):** the `results` vs `fpins` key and event-retention semantics are unvalidated against a live 10.5 array (ADR-0009); `pmax_fpin_events` is a snapshot count, not a rate.
