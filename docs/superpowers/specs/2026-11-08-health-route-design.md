# GET /health: design

Status: approved scope, not implemented. Origin: R1 in `orchestrator-api-proposal.md`.

## Intent

The admin UI shows an "Orchestrator Online" badge. Today `probeConnection()` calls `GET /groups`, which returns the whole list and queries SQLite on every probe. It also treats any non-2xx response, even a 500, as "offline". A dedicated cheap probe fixes both.

## Goals

- `GET /health` returns `200` with body `{"status":"ok"}`.
- A connection error or timeout means the Orchestrator is down. Any HTTP response from this route means the process is reachable.
- The route needs no authentication and must stay public if authentication is added later.

## Non-goals

- No dependency checks. `GET /ready` was dropped: SQLite is an embedded file and a readiness check adds little. It can be added later as a separate route without changing this one.
- No version, uptime or build information in the body.

## Anti-goals

- The handler must not touch SQLite, any store or any lock. If it ever does, a database problem would make the badge report "offline", which is the original complaint.
- It must not log at a level that floods the log on every UI poll. The existing logger middleware stays as is.

## Constraints

- Structure follows the handler pattern: a `HealthHandler` in `internal/orchestrator/api/handler/health_handler.go` with `RegisterRoutes(r chi.Router)`, registered through `registerHandlers` in `api/router.go`. It takes no dependencies.
- Response encoding uses the existing `utils.EncodeToJSON`.
- The route is registered at the root (`/health`), not under a prefix.

## Decisions

- Registering through `registerHandlers` keeps one wiring style. Any future auth middleware is applied to specific route groups, so `/health` stays outside it by registering it on the root router, as the other handlers do today.
- The client change (`probeConnection()` calling `/health`) is a separate task in the client repo, outside this spec.

## Left to the builder

- Test fixture shape.
