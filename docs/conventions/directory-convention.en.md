# Directory Convention

## Purpose

This document explains the intended directory structure, package responsibilities, and dependency boundaries of this repository.

It is written for two audiences:

- humans reading and extending the codebase
- coding agents that need stable context about why the project is structured this way

This document should be updated when the ownership or responsibility of a directory changes in a meaningful way.

## Project Goal

This repository is a reverse proxy POC.

Current implementation direction:

- load app-level bootstrap config from `configs/app.json`
- load all reverse proxy config files from `configs/proxy/*.json`
- validate configs at startup
- build one global route table
- build one global upstream registry
- keep active runtime state in memory
- proxy requests using the active runtime snapshot

Out of scope for the current phase:

- automatic reload
- file watch
- config save / backup
- runtime health check execution
- dashboard write APIs

## Top-Level Layout

### `go.mod`

Single-module Go project definition.

Rules:

- keep one module during the POC phase
- do not split into multiple modules unless there is a strong reason
- keep internal implementation under `internal/`

### `main.go`

Program entrypoint.

Responsibilities:

- determine config path
- initialize logger
- load app config
- create app
- run servers

Rules:

- keep `main.go` thin
- do not move routing policy or runtime orchestration logic into `main.go`

### `configs/`

Configuration files used by the application.

Current structure:

- `configs/app.json`
- `configs/proxy/*.json`

Intent:

- `app.json` stores server bootstrap configuration
- `proxy/*.json` stores reverse proxy desired-state documents

### `docs/`

Official home for human-readable architecture, convention, and harness notes.

Intent:

- preserve project structure intent
- help future contributors understand package boundaries quickly
- help coding agents recover architecture context

Current examples:

- `docs/architecture/architecture.ko.md`
- `docs/api/dashboard-api.ko.md`
- `docs/conventions/directory-convention.ko.md`
- `docs/conventions/type-reference.ko.md`
- `docs/harness/agent-contract.md`
- `docs/harness/harness.md`

Note:

- the legacy `convention/` directory may remain as a compatibility path while the repository transitions to `docs/`

### `plan/`

Working notes and implementation plans.

Status:

- intentionally git-ignored
- useful for temporary planning, not for permanent repository convention

Recommended directory:

- `plan/tasks/`

Intent:

- store per-task specs based on `docs/harness/task-template.md`
- capture task goals, test plans, and documentation impact notes

### `scripts/`

Repository-local developer harness and helper commands.

Current examples:

- `scripts/agent-check.sh`
- `scripts/agent-commit.sh`
- `scripts/install-hooks.sh`
- `scripts/validate-commit-msg.sh`

### `.githooks/`

Repository-local Git hooks.

Current examples:

- `.githooks/pre-commit`
- `.githooks/commit-msg`

## `internal/` Package Intent

All main implementation packages live under `internal/`.

General rule:

- business logic stays under `internal/`
- package names should be short and responsibility-driven
- avoid vague names such as `utils`, `helpers`, or `common`

## Package Responsibilities

### `internal/app`

Application wiring and startup orchestration.

Responsibilities:

- connect config loading with runtime building
- construct runtime snapshot
- create proxy handler and dashboard handler
- create HTTP servers
- own application lifecycle flow

What belongs here:

- app construction
- startup wiring
- shutdown flow
- reload orchestration

What should not live here:

- detailed routing match logic
- upstream balancing logic
- raw config schema definitions

### `internal/config`

App-level bootstrap configuration only.

Current role:

- define `AppConfig`
- load `configs/app.json`
- apply defaults
- validate app-level config

Examples of data that belong here:

- proxy listen address
- dashboard listen address
- proxy config directory path

Examples of data that do not belong here:

- route definitions
- upstream pool definitions
- runtime health state

Reason:

The app bootstrap config changes more rarely than reverse proxy desired state and has different lifecycle semantics.

### `internal/proxyconfig`

Raw reverse proxy file schema and file loading.

Current role:

- define schema for `configs/proxy/*.json`
- load one file or all files from a directory
- validate a single proxy config file
- preserve file metadata such as source name and file path

Important distinction:

- this package owns config-file representation
- this package does not own runtime routing behavior

### `internal/route`

Runtime routing policy.

Current role:

- compile `proxyconfig` routes into runtime routes
- assign global route IDs
- assign global upstream pool references
- compile regex matchers
- build the global route table
- sort routes by precedence
- resolve request host/path to one route

Important rule:

- all proxy config files are merged into one global route table
- route matching is based on fixed precedence, not JSON array order

Current precedence:

1. exact
2. prefix
3. regex
4. any

Prefix semantics:

- segment-based, not plain string-prefix semantics

### `internal/upstream`

Runtime upstream registry and balancing.

Current role:

- compile upstream pools from all proxy config files
- assign global pool IDs
- build the global registry
- select a target from a pool

Current balancing:

- simple round-robin

Important distinction:

- config schema for upstream pools belongs to `internal/proxyconfig`
- runtime pool registry and target selection belongs to `internal/upstream`

### `internal/runtime`

Active in-memory state.

Current role:

- hold the active app config
- hold loaded proxy config metadata
- hold global route table
- hold global upstream registry
- expose snapshot reads
- support atomic snapshot swap

Important intent:

- runtime state is not a source-of-truth replacement
- runtime state is the active compiled view of the desired configuration

### `internal/proxy`

Actual reverse proxy request forwarding.

Current role:

- read current runtime snapshot
- resolve request against route table
- select upstream target
- forward request to selected upstream

Important boundary:

- `internal/proxy` should not define routing policy
- `internal/proxy` consumes routing and upstream decisions

### `internal/dashboard`

Read-oriented management HTTP endpoints for the current phase.

Current role:

- expose active config and runtime state
- return structured views for app config, loaded proxy configs, routes, and upstreams

Current scope:

- read APIs only
- no config mutation APIs yet

### `internal/middleware`

Cross-cutting HTTP middleware.

Current role:

- shared middleware such as request logging

Rule:

- only shared HTTP concerns belong here

## Dependency Direction

Intended dependency direction:

- `main` -> `app`
- `app` -> `config`, `proxyconfig`, `route`, `upstream`, `runtime`, `proxy`, `dashboard`
- `proxy` -> `runtime`, `route`
- `route` -> `proxyconfig`
- `upstream` -> `proxyconfig`
- `dashboard` -> `runtime`

Packages that should stay decoupled:

- `route` should not depend on `dashboard`
- `upstream` should not depend on `dashboard`
- `config` should not depend on HTTP or UI packages

## Namespace Rule

Each proxy config file gets a source name derived from its file name without extension.

Example:

- `configs/proxy/default.json` -> source `default`

Global IDs are built using this source:

- route ID: `<source>:<route.id>`
- upstream pool ID: `<source>:<pool.id>`

Reason:

- allow repeated local IDs across different files
- keep runtime IDs globally unique

## Design Intent Summary

The codebase intentionally separates three layers:

1. file schema layer
2. runtime policy layer
3. application wiring layer

Mapping:

- file schema layer -> `internal/config`, `internal/proxyconfig`
- runtime policy layer -> `internal/route`, `internal/upstream`, `internal/runtime`
- application wiring layer -> `internal/app`, `internal/proxy`, `internal/dashboard`

This separation should be preserved unless there is a strong reason to change it.
