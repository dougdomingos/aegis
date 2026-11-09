# GET /health Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dependency-free liveness route, `GET /health`.

**Architecture:** One handler with no dependencies, registered through the existing `registerHandlers` call.

**Tech Stack:** Go 1.26, chi.

**Spec:** `docs/superpowers/specs/2026-11-08-health-route-design.md`

## Global Constraints

- Follow `AGENTS.md`: doc comment on every symbol starting with its name; external test package `handler_test`; test names `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`; two-step test writing (skeleton, then bodies); helpers at the end of the file.
- Handler tests assert the HTTP status code only. The body is checked once in the router test.
- Do not commit. Leave changes for developer review (`AGENTS.md` 6.1).
- Response body is exactly `{"status":"ok"}` with status 200.

## Review Focus

- `HEAD /health` and `POST /health` are not served (405 or 404 is acceptable). Test the POST case returns a non-200 status.
- The route works with an empty database and does not require any migrations to have run (the router test builds the router over an in-memory database; the handler itself holds no database reference).
- A trailing slash `/health/` is not required to work. Do not add a special case.

---

### Task 1: HealthHandler and router registration

**Files:**
- Create: `internal/orchestrator/api/handler/health_handler.go`
- Create: `internal/orchestrator/api/handler/health_handler_test.go`
- Modify: `internal/orchestrator/api/router.go:79-85` (add to `registerHandlers`)
- Modify: `internal/orchestrator/api/router_test.go`

**Interfaces:**
- Consumes: `utils.EncodeToJSON(w http.ResponseWriter, status int, payload any)`.
- Produces: `type HealthHandler struct{}`, `func NewHealthHandler() *HealthHandler`, `func (handler *HealthHandler) RegisterRoutes(r chi.Router)`, `func (handler *HealthHandler) Check(w http.ResponseWriter, r *http.Request)`.

- [ ] **Step 1: Write the test skeleton** in `health_handler_test.go`:
  - `TestHealthHandler_Check_WithGetRequest_ReturnsStatus200`
  - `TestHealthHandler_Check_WithPostRequest_ReturnsNon200Status`

- [ ] **Step 2: Fill in the bodies.** Build a `chi.NewRouter()`, call `NewHealthHandler().RegisterRoutes(router)`, use `performRequest` from `helpers_test.go`. First test asserts `assertStatusCode(t, rec, http.StatusOK)`. Second asserts `rec.Code != http.StatusOK`.

- [ ] **Step 3: Run to confirm failure.** Run `go test ./internal/orchestrator/api/handler -run TestHealthHandler -v`. Expected: build failure, `NewHealthHandler` undefined.

- [ ] **Step 4: Implement** the handler. `Check` writes `map[string]string{"status": "ok"}` with 200. `RegisterRoutes` registers `r.Get("/health", handler.Check)`.

- [ ] **Step 5: Register it.** Add `handler.NewHealthHandler()` as the first argument group entry in the `registerHandlers` call in `router.go`.

- [ ] **Step 6: Router-level check.** Add `TestNewRouter_WithHealthRequest_ReturnsOkBody` to `router_test.go`: build the router as the neighboring tests do, `GET /health`, expect 200 and a decoded body equal to `map[string]string{"status": "ok"}`.

- [ ] **Step 7: Verify.** Run `make tests`. Expected: all packages PASS.
