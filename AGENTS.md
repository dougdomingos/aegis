# AGENTS.md

Guidance for AI agents working on the Aegis codebase. Read this file before
writing or modifying code. Its goal is to let agents build features quickly
while matching the existing architecture, code style, and quality standards.

## 1. Project Overview & Scope

Aegis is a centralized management and distributed network filtering system that
enforces dynamic internet access policies across Linux computer laboratories
using a two-tier architecture:

- **Orchestrator** (`cmd/orchestrator`): Central control plane exposing a REST
  API (chi + SQLite) for rule storage, policy definition, logical group
  management, and real-time config distribution.
- **Sentinel** (not yet implemented): Lightweight per-endpoint daemon that
  receives policies and enforces kernel-level filtering.

### 1.1. Environment & Tech Stack

- **Module Path:** `dougdomingos.com/aegis`
- **Language / Runtime:** Go `1.26.5`
- **Router & DB:** `chi` HTTP router, SQLite
- **Tooling:** `golangci-lint` v1.64.x, `goimports`, `lefthook`

## 2. Architecture & Directory Structure

### 2.1. Directory Layout

| Path                | Responsibility                                                                                                |
| ------------------- | ------------------------------------------------------------------------------------------------------------- |
| `cmd/`              | Entrypoints (e.g., `cmd/orchestrator/main.go`).                                                               |
| `internal/api/`     | HTTP layer: `router.go` (DI/wiring), `handler/`, `middleware/`, `utils/`. Declares the service interfaces it consumes. |
| `internal/domain/`  | Entities, value validation, builders, and store interfaces. No I/O.                                           |
| `internal/errors/`  | Package-level sentinel errors only.                                                                           |
| `internal/schemas/` | Request/response DTOs used at the API boundary.                                                               |
| `internal/service/` | Business logic orchestration; depends on store interfaces.                                                    |
| `internal/store/`   | SQLite persistence implementing the `domain.*Store` interfaces.                                               |
| `internal/infra/`   | `database.go` (DB init), `migrations/` (registry + `sql/` files), `query/` (ORM-like interface for entities). |

### 2.2. Layers and Dependency Flow

This project uses a **Layered Clean Architecture** that enforces strict,
unidirectional dependencies between packages. **Dependencies between layers
must always be specified through interfaces**, never on concrete
implementations:

```
# Orchestrator dependency flow
API Handler ──> Service ──> Store ──> SQLite
```

Interfaces are owned by their consumers: `*ServiceInterface` contracts are
declared in `internal/api/handler/` (the layer that consumes them), while
`*Store` contracts are declared in `internal/domain/`. Services and stores
satisfy these interfaces structurally; no layer depends on a concrete
implementation.

## 3. Tools and Environment

The Makefile is your single source of truth for commands and tools in this
repository. You may use any of the following targets, accordingly to the
assigned task:

```
# Install required dependencies and tools
make init

# Execute tests with coverage profile
make test-coverage
```

### 3.1. Git Hooks (Lefthook)

- `pre-commit`: linting and import sorting on staged files only
- `pre-push`: lint and test on all packages

> **Note**: Do not re-run commands applied by Git Hooks locally

## 4. Code Conventions

### 4.1. Code Style

- Structs related to an entity must be named as the following: `Entity*` (e.g.,
  Rule -> RuleStore, RuleService, RuleHandler, etc)

### 4.2. Documentation

- Every symbol (e.g., types, functions, structs, methods, variables, constants)
  must carry a short doc comment starting with its name, as per Go convention
- When documenting interfaces, you must not add doc comments to their concrete
  method implementatios

## 5. Guidelines for implementation

### 5.1. Migrations

- Existent migrations must not be modified, except for bug fixes
- Apply new fields and/or entities through a new SQL file, following the
  `XXX_<migration_name>.sql` name pattern
- Each SQL migration file should apply a single update to the database schema

### 5.2. Implementing new features

- For new features and resources, follow the Vertical Slice approach:
  Store -> Service -> API Handler
- Work step-by-step, one layer at a time, from the least dependent layer to the
  most dependent
- Each layer must build and pass its own tests before moving to the next layer

### 5.3. Implementing tests

- Unit tests should follow the Arrange-Act-Assert pattern
- Test files live in the same directory as their targets, but in external
  packages (e.g., `rule_service_test.go` -> package `service_test`)
- Test functions must follow the `Test<Type>_<Method>_<Scenario>_<ExpectedResult>`
- Assertions should use plain `t.Errorf()` or `t.Fatalf()` with descriptive
  messages
- Test files should be written in two steps: first, create the base suite
  skeleton (i.e., arrange/seed methods + test functions signatures without
  implementation); then, implement each test only after the base code is set
- Declare helper methods (e.g., DB seeding, arrange steps) at the end of the
  file
- When dealing with multiple validations of the same scenario, arrange them
  into table-driven tests (i.e., `map[string]struct{...}`) iterated with
  `t.Run(name, ...)`

#### 5.3.1. Store tests

- Use the `arrangeStoreTest` function to setup test scenarios
- `arrangeStoreTest` applies all migrations to provision the schema, so no
  table schema should be declared or replicated in store tests

#### 5.3.2. Service tests

- Declare an `arrangeXServiceTest` method at the end of the file
- Implement a `mapOutputToX` method to convert entity structs into DTO schemas

#### 5.3.3. API Handler tests

- Declare a service mock that implements the target service interface
- Each scenario must override the method in the mock, returning the expected
  service response for that respective scenario
- Assertions should only verify the HTTP response code

## 6. Do's and Don'ts (Guardrails)

### 6.1. Git Discipline & Execution Rules

- **Never commit automatically.** Prepare changes and leave them for developer review
- **Open PRs only on explicit developer request** using `.github/PULL_REQUEST_TEMPLATE.md`. References tracking issue (`Fixes #N` / `Closes #N`)
- Proceed with any Git change (stage, commit, branch, PR) **only** when all tests pass and coverage meets the 75% gate
- Use the **issue-based branch naming pattern** `feat/issue-N-slug` (e.g., `feat/issue-2-rules`)