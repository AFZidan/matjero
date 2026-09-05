# Core `pkg/` to `modules/` Structural Refactor Report

## Executive Summary

Performed a pure structural rename of the Core domain directory `/pkg` to `/modules` in `matjeroapps/core`. All Go import paths, CI configuration, build scripts, and current architecture documentation were updated. No business logic, database migrations, or API contracts were modified. P5.5 and P5.6 have NOT been started.

## Key Metadata

- **Base SHA:** `58a90771467ccb04798c66825ccacd75bd0281c0`
- **Branch:** `refactor/rename-pkg-to-modules`
- **Old root:** `pkg/`
- **New root:** `modules/`
- **Directories moved:**
  - `pkg/actorapi` → `modules/actorapi`
  - `pkg/actorhttp` → `modules/actorhttp`
  - `pkg/api` → `modules/api`
  - `pkg/catalog` → `modules/catalog`
  - `pkg/commerce` → `modules/commerce`
  - `pkg/contracts` → `modules/contracts`
  - `pkg/markets` → `modules/markets`
  - `pkg/openapi` → `modules/openapi`
  - `pkg/storefront` → `modules/storefront`
  - `pkg/themes` → `modules/themes`
- **Number of Go imports updated:** 43 files
- **CI references updated:** `.github/workflows/ci.yml` (gofmt path `find apps cmd packages modules internal`)
- **Scripts/build references updated:** All Dockerfiles (`go-app.Dockerfile`) build using path relative to root; verified with Docker builds
- **Docs updated:** `README.md`, `docs/implementation/*.md`
- **Historical pkg references intentionally retained:** Historical ADR and audit log notes describing the prior monorepo migration history
- **Behavior changes:** NONE
- **Migration changes:** NONE
- **OpenAPI contract changes:** NONE (0 drift verified via `go run ./cmd/openapi-gen` and `git diff --exit-code -- docs/api`)

## Architecture Rule Enforced

```text
apps
  ↓
modules
  ↓
packages
```

- `modules/`: Core-specific domain and application modules (`commerce`, `catalog`, `markets`, `storefront`, `themes`, `contracts`, `openapi`, `actorapi`, `actorhttp`, `api`).
- `packages/`: Shared technical, platform, and infrastructure primitives (`database`, `events`, `outbox`, `money`, etc.).

Verified zero dependency inversions (`packages/*` contains 0 imports from `modules/*`).

## Verification Results

| Validation | Tool / Command | Result |
| :--- | :--- | :--- |
| Old Directory Removed | `test ! -d pkg` | PASS |
| New Directory Present | `test -d modules` | PASS |
| Import Path Search | `grep -rnI --exclude-dir=.git "github.com/matjeroapps/core/pkg/" .` | 0 source matches |
| Go Workspace / List | `GOWORK=off go list ./...` | PASS (all `modules/*` paths listed) |
| Go Dependency List | `GOWORK=off go list -m all` | PASS |
| Go Vet | `GOWORK=off go vet ./...` | PASS |
| Go Test Suite | `GOWORK=off TEST_DATABASE_URL=... go test ./...` | PASS (100% green across all packages) |
| OpenAPI Contract Drift | `go run ./cmd/openapi-gen && git diff --exit-code -- docs/api` | PASS (0 drift) |
| Docker Compose Config | `docker compose config --quiet` | PASS |
| Docker Build `core-api` | `docker build -f docker/go-app.Dockerfile --build-arg APP_PATH=./apps/core-api ...` | PASS |
| Docker Build `general-worker` | `docker build -f docker/go-app.Dockerfile --build-arg APP_PATH=./apps/workers/general-worker ...` | PASS |
| Docker Build `scheduler` | `docker build -f docker/go-app.Dockerfile --build-arg APP_PATH=./apps/workers/scheduler ...` | PASS |

## Phase Status

- **P5.5 started:** NO
- **P5.6 started:** NO
