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
- Route inventory to document (16 routes), from `internal/module/*/handler.go`:

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

- Request/response DTOs already exist per module in `dto.go`; swag will
  reference them by type name in annotations — do **not** create new DTOs.
- Error body is `{ "message": string }` (`server.errorBody`). Header
  `x-organization-id` gates `RequireOrg` routes; `Authorization: Bearer` gates
  auth'd routes.
- Existing Dockerfile: `apps/backend/Dockerfile` (alpine + curl HEALTHCHECK).
- Existing CI: `.github/workflows/ci.yml` (backend: vet/build/migrate/test;
  frontend: lint/build). No lint job, no docker build, no release workflow.
- Source k8s tree to port: `../controlplane-api/k8s/` — namespace, configmap,
  secret(.example), api/{deployment,service,ingress}, postgres/{statefulset,
  service,secret(.example)}, redis/{deployment,service}.
- Env parity gotcha: source configmap uses `NODE_ENV`; the Go app uses
  **`APP_ENV`** (+ `APP_NAME`, `LOG_LEVEL`, `PORT`, `JWT_*`). See `.env.example`.

## Decisions locked for this phase (build to these)

1. **Swagger tool = swaggo** (`swaggo/swag` + `swaggo/echo-swagger`). Decided in
   `CLAUDE.md` stack. Generated `docs/` package is **committed** so CI needs no
   `swag` binary.
2. **Runner image = distroless** (`gcr.io/distroless/static-debian12:nonroot`).
   The arch doc calls for distroless (line 53). Distroless has no shell/curl, so
   ship a tiny **`cmd/healthcheck`** Go binary for the `HEALTHCHECK`. (Fallback
   if the owner objects: keep the current alpine runner — but default is distroless.)
3. **Release workflow** = port source `release.yml` (ghcr push on CI success).
   Path/context adjusted to `apps/backend`.
4. Serve UI at `/swagger` (redirect) and `/swagger/*` (echo-swagger handler),
   matching the source's `/swagger` path.

---

## Step 1 — Swagger annotations (swaggo)

### 1a. Add dependencies

```bash
cd apps/backend
go get github.com/swaggo/swag@latest
go get github.com/swaggo/echo-swagger@latest
go install github.com/swaggo/swag/cmd/swag@latest   # CLI for codegen (local + Makefile only)
```

### 1b. General API info — `cmd/api/main.go`

Add a doc-comment block above `func main()` (mirrors source `info` + bearerAuth):

```go
// @title                      Controlplane API
// @version                    0.1.0
// @description                Multi-tenant B2B SaaS control-plane API (Go port).
// @BasePath                   /
// @securityDefinitions.apikey BearerAuth
// @in                         header
// @name                       Authorization
// @description                Type "Bearer {token}" — the JWT access token.
```

### 1c. Per-handler annotations

Add swaggo comments directly above each handler method (the 16 routes in the
table). Template — fill `@Tags` per module, reference the **existing** DTO
structs by fully-qualified type for body/response:

```go
// register creates a new user and returns the auth token pair.
// @Summary  Register a new user
// @Tags     auth
// @Accept   json
// @Produce  json
// @Param    body  body      auth.RegisterRequest  true  "Registration payload"
// @Success  201   {object}  auth.AuthResponse
// @Failure  409   {object}  server.errorBody  "EMAIL_TAKEN"
// @Failure  422   {object}  server.errorBody  "Validation failed"
// @Router   /auth/register [post]
func (h *Handler) register(c echo.Context) error { ... }
```

Rules for the annotation pass:
- **Tags**: `health`, `auth`, `organizations`, `rbac`, `subscription`, `audit-logs`.
- **Security**: add `// @Security BearerAuth` to every guarded route (all except
  `/health` and the four `/auth/*`). For `RequireOrg` routes also add a header
  param: `// @Param x-organization-id header string true "Active organization ID"`.
- **Path params**: `// @Param userId path string true "User ID"` for
  `/organizations/members/:userId`; `roleId` for the permissions route.
- **Query params** on `/audit-logs`: `userId` (string, optional), `action`
  (string, optional), `limit` (int, optional) — match `auditlog/dto.go`.
- **Statuses/messages** must match `docs/02-api-contract.md` exactly (e.g.
  register `201`, login `200`, invite `403 FORBIDDEN` / `403 MEMBER_LIMIT_REACHED`,
  etc.). Do not invent codes; copy from the contract table.
- `server.errorBody` is unexported → swag can still reference it as
  `server.errorBody`. If swag rejects the lowercase type, add an exported alias
  `type ErrorResponse = errorBody` in `server.go` and reference that instead.
- Keep the existing human-readable comment as the first line; swag uses it.

### 1d. Generate the docs package

```bash
cd apps/backend
swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
```

- Produces `apps/backend/docs/{docs.go,swagger.json,swagger.yaml}`, package `docs`.
- No collision with the repo-root `docs/` (different directory).
- **Commit** the generated files.

### 1e. Mount the UI — `internal/server/server.go`

Add imports and route (after `health.NewHandler().Register(e)`):

```go
import (
    echoSwagger "github.com/swaggo/echo-swagger"
    _ "github.com/controlplane/backend/docs" // generated OpenAPI spec
)

// ... inside New(), before module wiring:
e.GET("/swagger", func(c echo.Context) error {
    return c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
})
e.GET("/swagger/*", echoSwagger.WrapHandler)
```

`go mod tidy` after wiring.

### 1f. Makefile target

Add to `.PHONY` and append:

```make
## Regenerate Swagger/OpenAPI docs (requires: go install github.com/swaggo/swag/cmd/swag@latest)
swagger:
	cd apps/backend && swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
```

---

## Step 2 — Production Dockerfile (distroless + healthcheck binary)

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

---

## Step 3 — golangci-lint

### 3a. `apps/backend/.golangci.yml`

```yaml
run:
  timeout: 5m
linters:
  enable:
    - govet
    - errcheck
    - staticcheck
    - ineffassign
    - unused
    - gofmt
    - goimports
    - misspell
issues:
  exclude-rules:
    - path: _test\.go
      linters: [errcheck]
```

### 3b. Update Makefile `lint`

```make
## Lint backend (golangci-lint if installed, else go vet)
lint:
	cd apps/backend && (command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || go vet ./...)
```

### 3c. Run once locally and fix findings

```bash
cd apps/backend && golangci-lint run
```

Fix anything the new linters surface (likely unchecked `errcheck` on
`c.Redirect`/`resp.Body.Close` etc.). Keep fixes mechanical; no behavior change.

---

## Step 4 — k8s manifests

Create `./k8s/` at repo root, porting `../controlplane-api/k8s/`. Copy files
verbatim then apply these edits:

### 4a. `k8s/configmap.yaml` — env parity

Replace `NODE_ENV: production` with the Go app's vars:

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
`DATABASE_URL`, `REDIS_URL` (min 32-char secrets). Copy to `secret.yaml` with
`.gitignore` covering the real one (source already ignores `k8s/**/secret.yaml`
— replicate in root `.gitignore` if not present).

### 4c. `k8s/api/deployment.yaml`

- `image: controlplane-api:latest` (unchanged name is fine; or `ghcr.io/<repo>:latest`).
- Keep `imagePullPolicy: Never` for local kind/minikube, or `IfNotPresent` for ghcr.
- `envFrom` configmap + secret already correct.
- Liveness/readiness probes already hit `/health` — keep.

### 4d. Copy unchanged

`namespace.yaml`, `api/service.yaml`, `api/ingress.yaml`,
`postgres/{statefulset,service,secret.example}.yaml`,
`redis/{deployment,service}.yaml`. Verify postgres/redis images match compose
(postgres:16-alpine, redis:7-alpine).

### 4e. `web` Deployment — **defer** to Phase 6

Add a short note in `k8s/README.md` (new): "web/ manifests land in Phase 6 with
the frontend image."

---

## Step 5 — CI + release parity

### 5a. `.github/workflows/ci.yml` — add lint + docker build

- New `lint` job (backend):

```yaml
  lint:
    name: Lint
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: apps/backend
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest
          working-directory: apps/backend
```

- Optional **swagger drift** guard (add as a step in the backend job): run
  `swag init ...` and `git diff --exit-code docs/` to ensure committed docs are
  current. Include only if the `swag` install step is added.
- Add a **docker build** step to the backend job (mirrors source `build` job):

```yaml
      - name: Docker build
        run: docker build -t controlplane-api:ci ./apps/backend
        working-directory: .
```

  (Or a separate `build` job `needs: [backend, lint]`.)

### 5b. `.github/workflows/release.yml` — new (port source)

```yaml
name: Release
on:
  workflow_run:
    workflows: [CI]
    types: [completed]
    branches: [main]
jobs:
  release:
    name: Build & Push Docker
    runs-on: ubuntu-latest
    if: ${{ github.event.workflow_run.conclusion == 'success' }}
    permissions:
      contents: read
      packages: write
    steps:
      - uses: actions/checkout@v4
      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - uses: docker/build-push-action@v5
        with:
          context: ./apps/backend
          push: true
          tags: |
            ghcr.io/${{ github.repository }}:latest
            ghcr.io/${{ github.repository }}:${{ github.sha }}
```

Key deltas from source: `context: ./apps/backend`, added `permissions.packages: write`.

---

## Step 6 — Docs updates

- **`README.md`**: add sections — "API docs" (`/swagger`), "Docker" (`docker
  build ./apps/backend`, `docker compose up`), "Kubernetes" (`kubectl apply -f
  k8s/…`). Update the status/phase line to note Phase 5 complete.
- **`CLAUDE.md`** status paragraph: append that Swagger (`/swagger`), the
  distroless image, k8s manifests, and CI lint/release are live (Phase 5).
- **`docs/03-target-architecture.md`**: nothing to resolve in Open Questions here
  (they were Phase-4 scoped); optionally add a "Deviations resolved during
  Phase 5" note if the distroless-healthcheck-binary choice is worth recording.
- Record the two Phase-5 choices worth flagging: (a) committed generated swag
  `docs/` package (so CI needs no swag binary); (b) healthcheck binary for
  distroless.

---

## Step 7 — Verify (run in order)

```bash
cd apps/backend
go mod tidy
swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal
git diff --exit-code docs/            # committed docs are current
go vet ./...
golangci-lint run                     # clean
go build ./...
go test ./...                         # unchanged behavior — all green
cd ../.. && docker build -t controlplane-api:dev ./apps/backend
```

Manual smoke (optional, needs infra):

```bash
make up && make migrate && make seed
cd apps/backend && go run ./cmd/api &
curl -s localhost:3000/health
curl -sI localhost:3000/swagger            # 301 → /swagger/index.html
curl -s localhost:3000/swagger/doc.json | head
```

Lint the CI YAML by pushing a branch / `act` if available.

## Definition of done

1. `/swagger` serves Swagger UI with all 16 routes, correct tags, request/
   response schemas, and BearerAuth security; `/swagger/doc.json` matches
   `docs/02-api-contract.md` (statuses + messages).
2. Generated `apps/backend/docs/` committed; `swag init` produces no diff.
3. `docker build ./apps/backend` produces a distroless image whose
   `HEALTHCHECK` binary passes against a running container.
4. `k8s/` applies cleanly (dry-run `kubectl apply --dry-run=client -f k8s/ -R`);
   env vars match the Go config (`APP_ENV`, not `NODE_ENV`).
5. CI: `lint` (golangci-lint) + backend (vet/build/migrate/test) + docker build
   green; `release.yml` present and valid.
6. `go test ./...` unchanged — no behavior regressions.
7. README + CLAUDE.md status updated; Phase-5 choices recorded.

## Suggested commit sequence

1. `feat(docs): swagger annotations + /swagger endpoint (swaggo)`
2. `feat(deploy): distroless image + healthcheck binary`
3. `chore(lint): add golangci-lint config + Makefile/CI lint`
4. `feat(deploy): port k8s manifests (env parity, api image)`
5. `ci: add lint job, docker build, and release workflow`
6. `docs: update README/CLAUDE for Phase 5`
