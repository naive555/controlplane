# Phase 5 — Docs, Deploy, CI Parity — Implementation Plan

> Target executor: **Sonnet**. This plan is prescriptive: file paths, exact
> commands, and copy-paste-ready snippets. Read `docs/04-migration-plan.md`
> §"Phase 5" and `docs/03-target-architecture.md` lines 17, 53, 62–63 first.
> No business-logic changes in this phase — routes and behavior are frozen by
> Phase 4. This phase only adds API docs, a production image, k8s manifests,
> and CI/release parity.

## Scope

Phase 5 = **Docs, deploy, CI parity** (migration-plan §Phase 5):

1. **Swagger** at `/swagger` via **swaggo** (the decided stack — `CLAUDE.md`).
2. **Production Dockerfile**: distroless runner + working `HEALTHCHECK`.
3. **k8s manifests** ported from `../controlplane-api/k8s/` (api image → Go, env parity).
4. **GitHub Actions**: add `lint` (golangci-lint) + `docker build`, mirror source `release.yml` (ghcr push).
5. **Docs**: README + `docs/03` open-question / status updates; record deviations.

### Non-goals (defer)

- Frontend k8s `web` Deployment and frontend Docker/CI depth → **Phase 6**
  (frontend is only scaffolded; do not wire it into k8s here). Leave the
  commented `web:` block in `compose.yaml` as-is.
- Any change to handlers/services/DTO behavior. Swagger annotations are
  comments only — they must not alter runtime behavior.

## Ground truth captured from the codebase

- Go 1.26.2, Echo v4, module `github.com/controlplane/backend`.
- Route inventory (16 routes), from `internal/module/*/handler.go`:

  | Method | Path | Guard | Handler |
  | --- | --- | --- | --- |
  | GET | `/health` | public | `health.check` |
  | POST | `/auth/register` | public | `auth.register` |
  | POST | `/auth/login` | public | `auth.login` |
  | POST | `/auth/refresh` | public | `auth.refresh` |
  | POST | `/auth/logout` | public (reads bearer) | `auth.logout` |
  | POST | `/organizations` | RequireAuth | `organization.create` |
  | GET | `/organizations` | RequireAuth | `organization.list` |
  | POST | `/organizations/invite` | RequireOrg | `organization.invite` |
  | DELETE | `/organizations/members/:userId` | RequireOrg | `organization.removeMember` |
  | GET | `/rbac/roles` | RequireOrg | `rbac.listRoles` |
  | POST | `/rbac/roles` | RequireOrg | `rbac.createRole` |
  | PUT | `/rbac/roles/:roleId/permissions` | RequireOrg | `rbac.updatePermissions` |
  | POST | `/rbac/assign` | RequireOrg | `rbac.assignRole` |
  | GET | `/subscription` | RequireOrg | `subscription.get` |
  | POST | `/subscription/assign` | RequireOrg | `subscription.assign` |
  | GET | `/audit-logs` | RequireOrg | `auditlog.query` |

- Error body is `{ "message": string }`. Header `x-organization-id` gates
  `RequireOrg` routes; `Authorization: Bearer` gates auth'd routes.
- Existing Dockerfile: `apps/backend/Dockerfile` (alpine + curl HEALTHCHECK).
- Existing CI: `.github/workflows/ci.yml` (backend: vet/build/migrate/test;
  frontend: lint/build). No lint job, no docker build, no release workflow.
- Source k8s tree to port: `../controlplane-api/k8s/`.
- Env parity gotcha: source configmap uses `NODE_ENV`; the Go app uses
  **`APP_ENV`** (+ `APP_NAME`, `LOG_LEVEL`, `PORT`, `JWT_*`).

## Decisions locked for this phase

1. **Swagger tool = swaggo** (`swaggo/swag` + `swaggo/echo-swagger`). Generated
   `docs/` package is **committed** so CI needs no `swag` binary.
2. **Runner image = distroless** (`gcr.io/distroless/static-debian12:nonroot`)
   with a tiny **`cmd/healthcheck`** Go binary for `HEALTHCHECK`.
3. **Release workflow** ports source `release.yml` (ghcr push on CI success).
4. UI at `/swagger` (redirect) + `/swagger/*` (echo-swagger handler).

---

## Step 1 — Swagger annotations (swaggo) — ✅ DONE (2026-07-21)

### What shipped

- Added deps: `github.com/swaggo/swag`, `github.com/swaggo/echo-swagger`.
  Installed the `swag` CLI via `go install github.com/swaggo/swag/cmd/swag@latest`.
- `cmd/api/main.go`: added the general-info doc-comment block (`@title`,
  `@version`, `@BasePath`, `@securityDefinitions.apikey BearerAuth`, etc.)
  above `func main()`.
- Annotated all 16 handler methods across `auth`, `organization`, `rbac`,
  `subscription`, `auditlog`, `health` with `@Summary`/`@Tags`/`@Param`/
  `@Success`/`@Failure`/`@Router`, referencing the existing DTO structs by
  name and the contract's exact status codes/messages from `docs/02`.
- `internal/server/server.go`: mounted `GET /swagger` (301 redirect to
  `/swagger/index.html`) and `GET /swagger/*` (`echoSwagger.WrapHandler`),
  importing the generated `docs` package with `_`.
- Makefile: added `make swagger` target running `swag init`.

### Deviation from the original draft — discovered during implementation

The original draft planned an exported `server.ErrorResponse` (alias or
rename of the existing unexported `errorBody`) referenced from every
handler's `@Failure` annotations as `server.ErrorResponse`. **This does not
work with swaggo.** Two problems surfaced running `swag init
--parseDependency --parseInternal`:

1. With `--parseDependency`, swag namespaces cross-package type refs by
   their *import path* when resolving `{object}` annotations, so a bare
   `server.ErrorResponse` reference from a file in another package silently
   fails with `cannot find type definition: server.ErrorResponse` — even
   though the type exists and is exported. Adding `--useStructName` changes
   how swag *emits* names but does not fix resolution.
2. The real constraint: swag only resolves a `pkgname.Type` comment
   reference in a given file if that package is one **the file already
   imports** as `pkgname`. `internal/server` is never imported by any
   handler file (only by `cmd/api/main.go`), so no handler file could ever
   reference `server.ErrorResponse` regardless of flags.

**Fix applied**: moved the shared error-response type out of `internal/server`
into `internal/shared/httpx` (new file `httpx/response.go`), which every
handler file already imports for `httpx.BindAndValidate`. `server.go` now
uses `httpx.ErrorResponse` instead of a locally-defined `errorBody`/
`ErrorResponse` type (behaviorally identical — same `{"message": string}`
JSON shape). All `@Failure` annotations reference `httpx.ErrorResponse`.
`swag init` runs with `--parseDependency --parseInternal --useStructName`
(the last flag keeps generated definition names short, e.g. `ErrorResponse`
instead of a mangled full-import-path name).

**Takeaway for later steps/phases**: any future DTO shared across module
packages for Swagger purposes must live in a package that's *already
imported* by every referencing file (e.g. `httpx`, `apperror`), not in
`internal/server`.

### Verified

- `go build ./...`, `go vet ./...`, `go test ./...` — all clean, no behavior
  changes (confirmed via `git diff --stat`: only new annotations/comments,
  the `httpx.ErrorResponse` move, and the swagger mount in `server.go`).
- Generated `apps/backend/docs/{docs.go,swagger.json,swagger.yaml}` —
  inspected `swagger.json`: all 16 operations present across 14 paths (two
  paths carry two methods each: `/organizations` GET+POST, `/subscription`
  GET+POST), `securityDefinitions.BearerAuth` present with correct
  description.
- Did **not** get a live `/swagger` UI smoke test — Docker Desktop wasn't
  running locally (`docker compose up -d db redis` failed: "cannot connect
  to the Docker daemon"). Static validation of the generated spec was used
  instead. **Follow-up**: once infra is available, run `make up && make
  migrate && make seed && (cd apps/backend && go run ./cmd/api)` and hit
  `GET /swagger` (expect 301 → `/swagger/index.html`) and
  `GET /swagger/doc.json`.

### Files touched

- `apps/backend/cmd/api/main.go` (general API info)
- `apps/backend/internal/server/server.go` (swagger mount, `httpx.ErrorResponse` swap)
- `apps/backend/internal/shared/httpx/response.go` (new — `ErrorResponse` type)
- `apps/backend/internal/module/{auth,organization,rbac,subscription,auditlog,health}/handler.go` (annotations)
- `apps/backend/docs/{docs.go,swagger.json,swagger.yaml}` (generated, committed)
- `apps/backend/go.mod`, `go.sum` (swaggo deps)
- `Makefile` (`swagger` target)

---

## Step 2 — Production Dockerfile (distroless + healthcheck binary) — ✅ DONE (2026-07-21)

Shipped exactly as drafted below, no deviations.

### 2a. New `apps/backend/cmd/healthcheck/main.go`

Tiny binary; exits 0 if `GET http://127.0.0.1:$PORT/health` is 2xx, else 1.

```go
// Command healthcheck is a zero-dependency liveness probe for the distroless
// runtime image, where no shell or curl is available for Docker HEALTHCHECK.
package main

import (
	"net/http"
	"os"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/health")
	if err != nil || resp.StatusCode >= 300 {
		os.Exit(1)
	}
	os.Exit(0)
}
```

### 2b. Rewrite `apps/backend/Dockerfile`

```dockerfile
# syntax=docker/dockerfile:1

########################
# Builder
########################
FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/healthcheck ./cmd/healthcheck

########################
# Runner (distroless, nonroot)
########################
FROM gcr.io/distroless/static-debian12:nonroot AS runner
WORKDIR /app

COPY --from=builder /out/api ./api
COPY --from=builder /out/healthcheck ./healthcheck

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/app/healthcheck"]

ENTRYPOINT ["/app/api"]
```

Notes: distroless `:nonroot` runs as uid 65532 by default (no `USER`/`adduser`
needed). `CGO_ENABLED=0` static build is required for `static-debian12`.
`migrations/` are embedded via `embed.go`, so no need to copy SQL files.

### 2c. Verify build

```bash
docker build -t controlplane-api:dev ./apps/backend
```

**✅ Verified live (2026-07-21), once Docker Desktop was started:**

- `docker build -t controlplane-api:dev ./apps/backend` — succeeds; final
  image `57.2MB`, both `api` and `healthcheck` binaries built in the same
  builder stage.
- `docker compose up -d --build api` (against real `db`+`redis` compose
  services, migrations already applied) — container reaches
  `Up ... (healthy)` within the 10s `start-period`.
- `docker inspect controlplane-api --format='{{json .State.Health}}'` shows
  `"Status": "healthy"`, two consecutive health-check log entries with
  `"ExitCode": 0`.
- Container logs confirm the healthcheck binary's requests: `GET /health
  status=200`.
- `docker inspect --format='User: {{.Config.User}}'` → `65532` (distroless
  `nonroot`, confirming no root escalation).
- Live endpoint checks against the running container:
  - `GET /health` → `{"status":"ok","uptime":...}`
  - `GET /swagger` → `301 Moved Permanently` → `Location: /swagger/index.html`
  - `GET /swagger/index.html` → `200`
  - `GET /swagger/doc.json` → valid OpenAPI 2.0 doc, `info.title: "Controlplane API"`,
    matches the statically-inspected spec from Step 1.
- `docker exec controlplane-api /app/healthcheck` → exit `0` (confirms the
  binary is independently runnable, not just wired into `HEALTHCHECK`).
- Cleaned up with `docker compose down`; no leftover containers.

This also serves as the live `/swagger` UI smoke test that Step 1 had
deferred (Docker wasn't available at the time).

---

## Step 3 — golangci-lint — ✅ DONE (2026-07-21, not yet committed)

### Deviation from the original draft — golangci-lint v2 config format

`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`
resolves to **v2.12.2** now (the tool moved to a `/v2` module path and a
new config schema — `version: "2"` at the top, `gofmt`/`goimports` live
under a separate `formatters:` block instead of `linters.enable`, and
`issues.exclude-rules` moved to `linters.exclusions.rules`). The v1-style
config in the original draft doesn't validate under v2. Actual config
shipped:

```yaml
version: "2"

run:
  timeout: 5m

linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - ineffassign
    - unused
    - misspell
  exclusions:
    rules:
      - path: _test\.go
        linters:
          - errcheck

formatters:
  enable:
    - gofmt
    - goimports
```

Validated with `golangci-lint config verify` (no output = valid).

### 3b. Update Makefile `lint` — done, unchanged from draft

```make
## Lint the backend (golangci-lint if installed, else go vet)
lint:
	cd apps/backend && (command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || go vet ./...)
```

### 3c. Run once locally and fix findings — done, one real fix applied

`golangci-lint run` surfaced 4 issues on the first pass:

1. **`errcheck` in `cmd/migrate/main.go:48`** — `defer db.Close()` return
   value unchecked. **Fixed**: `defer func() { _ = db.Close() }()`,
   matching the existing house style for intentionally-ignored `Close()`
   errors in short-lived/cleanup paths (`internal/infra/redis/redis.go:25`
   uses the same `_ = client.Close()` pattern).
2. **3× `gofmt` findings, different files each run** — e.g. run 1 flagged
   `database.go`/`auth.go`(redis)/`bind.go`; run 2 flagged
   `database.go`/`organization/dto.go`/`organization/service.go`; run 3
   flagged `rbac/dto.go`/`rbac/service.go`/`rbac/service_test.go`.
   **Root cause, confirmed and not a real formatting problem**: this repo
   checkout has `core.autocrlf=true` (Windows), so most untouched `.go`
   files have CRLF line endings on disk while git stores LF. Verified by
   normalizing each flagged file's line endings (`sed 's/\r$//'`) and
   re-running `gofmt -d` — **zero diff** every time, meaning the actual
   (committed) content is already gofmt-clean; only the CRLF artifact
   differs. The specific 3-file subset golangci-lint's `gofmt` formatter
   picks changes nondeterministically run-to-run (raw `gofmt -l .` flags
   ~34 CRLF files consistently; golangci-lint's own check appears to sample/
   cap differently — not fully understood, but irrelevant given the root
   cause). **Not fixed** — touching line endings on ~34 unrelated files is
   out of scope for this step and would produce a huge unrelated diff.
   **Why this is safe to leave**: `.github/workflows/ci.yml`'s backend job
   runs on `ubuntu-latest`, which checks out LF-native (no `autocrlf`
   rewriting), so this flakiness is Windows-local-only and won't reproduce
   in CI.

Re-ran after the `errcheck` fix: `go build ./...`, `go vet ./...`,
`go test ./...` all clean — no behavior change.

**Not committed yet** — per instruction, this step's changes
(`.golangci.yml`, `cmd/migrate/main.go`, `Makefile`) are sitting in the
working tree.

---

## Step 4 — k8s manifests — pending

Create `./k8s/` at repo root, porting `../controlplane-api/k8s/`. Copy files
verbatim then apply these edits:

### 4a. `k8s/configmap.yaml` — env parity

```yaml
data:
  APP_ENV: production
  APP_NAME: controlplane-api
  LOG_LEVEL: info
  PORT: "3000"
  JWT_ACCESS_EXPIRES_IN: 15m
  JWT_REFRESH_EXPIRES_IN: "604800"
```

### 4b. `k8s/secret.example.yaml`

Keep `stringData` with `JWT_ACCESS_SECRET`, `JWT_REFRESH_SECRET`,
`DATABASE_URL`, `REDIS_URL`. Copy to `secret.yaml`, gitignored.

### 4c. `k8s/api/deployment.yaml`

Keep liveness/readiness probes against `/health`. Image name/pull policy per
target (kind/minikube vs ghcr).

### 4d. Copy unchanged

`namespace.yaml`, `api/service.yaml`, `api/ingress.yaml`,
`postgres/{statefulset,service,secret.example}.yaml`,
`redis/{deployment,service}.yaml`.

### 4e. `web` Deployment — defer to Phase 6

Add `k8s/README.md` noting web/ manifests land in Phase 6.

---

## Step 5 — CI + release parity — pending

### 5a. `.github/workflows/ci.yml`

Add a `lint` job (golangci-lint-action) and a docker-build step/job.

### 5b. `.github/workflows/release.yml` — new

Port source's ghcr push-on-CI-success workflow, with
`context: ./apps/backend` and `permissions.packages: write`.

---

## Step 6 — Docs updates — pending

- README: `/swagger`, Docker, Kubernetes sections; update phase status.
- CLAUDE.md status paragraph: note Swagger/distroless/k8s/CI-lint live.
- Record the Step-1 `httpx.ErrorResponse` swaggo deviation above as the
  canonical rationale if it needs citing elsewhere.

---

## Step 7 — Verify (run in order) — pending until Steps 2–6 land

```bash
cd apps/backend
go mod tidy
make swagger                          # from repo root; swag init --useStructName
git diff --exit-code docs/            # committed docs are current
go vet ./...
golangci-lint run
go build ./...
go test ./...
cd ../.. && docker build -t controlplane-api:dev ./apps/backend
```

Manual smoke (needs Docker Desktop running + infra up):

```bash
make up && make migrate && make seed
cd apps/backend && go run ./cmd/api &
curl -s localhost:3000/health
curl -sI localhost:3000/swagger            # 301 → /swagger/index.html
curl -s localhost:3000/swagger/doc.json | head
```

## Definition of done

1. ✅ `/swagger` serves Swagger UI with all 16 routes, correct tags, request/
   response schemas, and BearerAuth security; `swagger.json` matches
   `docs/02-api-contract.md` (statuses + messages). Live-verified against a
   running container (see Step 2c).
2. ✅ Generated `apps/backend/docs/` committed-ready; regeneration is
   idempotent via `make swagger`.
3. ✅ `docker build ./apps/backend` produces a distroless image (57.2MB)
   whose `HEALTHCHECK` binary passes against a running container —
   verified live via `docker compose up -d --build api` + `docker inspect`
   (see Step 2c).
4. ⬜ `k8s/` applies cleanly; env vars match Go config (`APP_ENV`, not `NODE_ENV`).
5. ⬜ CI: `lint` + backend + docker build green; `release.yml` present.
6. ✅ `go test ./...` unchanged — no behavior regressions.
7. ⬜ README + CLAUDE.md status updated.

## Suggested commit sequence

1. `feat(docs): swagger annotations + /swagger endpoint (swaggo)` ← Step 1, ready to commit
2. `feat(deploy): distroless image + healthcheck binary`
3. `chore(lint): add golangci-lint config + Makefile/CI lint`
4. `feat(deploy): port k8s manifests (env parity, api image)`
5. `ci: add lint job, docker build, and release workflow`
6. `docs: update README/CLAUDE for Phase 5`
