# controlplane

A monorepo rewrite of [`controlplane-api`](../controlplane-api) (Bun + ElysiaJS) into **Go (backend) + Next.js (frontend)** — a multi-tenant B2B SaaS platform template (auth, organizations, RBAC, audit logs, subscription limits). See [`docs/`](docs/) for the full analysis, API contract, target architecture, and migration plan, and [`CLAUDE.md`](CLAUDE.md) for ground rules.

**Status**: Phase 6 (frontend) complete — the full stack is live. Backend: `/auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout` (bcrypt, JWT access/refresh pairs, session rotation with reuse detection, Redis blacklist + login rate limiting), `POST/GET /organizations`, `GET /organizations/members`, `POST /organizations/invite`, `DELETE /organizations/members/:userId`, `GET/POST /rbac/roles`, `PUT /rbac/roles/:roleId/permissions`, `POST /rbac/assign`, `GET /subscription`, `POST /subscription/assign`, `GET /plans`, and `GET /audit-logs` are all live and documented at [`/swagger`](#swagger--api-docs) (`GET /organizations/members` and `GET /plans` are Phase 6 additions not present in the source app — see `docs/03-target-architecture.md` "Deviations resolved during Phase 6"). The backend ships as a distroless Docker image with k8s manifests and CI lint/build/release workflows. Frontend: a Next.js dashboard (App Router + shadcn/ui + TanStack Query) covering every module — auth, organizations + switcher, members, RBAC roles, audit logs, subscription — talking to the API through a same-origin runtime proxy (see [Frontend](#frontend) below).

## Prerequisites

- Go 1.26+
- Node 22+ with [Corepack](https://nodejs.org/api/corepack.html) enabled (`corepack enable`) — this repo uses **pnpm**, pinned via `apps/frontend/package.json`'s `packageManager` field
- Docker + Docker Compose v2

## Quickstart

```bash
cp .env.example .env
make up        # start Postgres + Redis (docker compose)
make migrate   # apply database schema (goose)
make seed      # insert default plans (free/pro/enterprise)

make api       # terminal 1 — Go API on :3000
make web       # terminal 2 — Next.js dev server on :4000

curl localhost:3000/health
open http://localhost:4000   # dashboard — register a user to get started
```

Regenerating sqlc query code (only needed after editing `apps/backend/internal/infra/database/queries/*.sql`) requires the `sqlc` CLI: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`, then `make sqlc`.

## Swagger / API docs

With `make api` running, open [`localhost:3000/swagger`](http://localhost:3000/swagger) for interactive Swagger UI covering every route (request/response schemas, status codes, and the `BearerAuth` security scheme). The raw spec is at `localhost:3000/swagger/doc.json`.

The spec is generated from Go doc-comments via [swaggo](https://github.com/swaggo/swag) and committed to `apps/backend/docs/`. After changing a handler's annotations, regenerate with:

```bash
go install github.com/swaggo/swag/cmd/swag@latest   # once
make swagger
```

### Try the auth flow

With `make api` running:

```bash
# register (returns { accessToken, refreshToken })
curl -s localhost:3000/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"password123"}'

# login
curl -s localhost:3000/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"password123"}'

# refresh (rotates the refresh token; reusing the old one after this fails with 401)
curl -s localhost:3000/auth/refresh \
  -H 'Content-Type: application/json' \
  -d '{"refreshToken":"<refreshToken from above>"}'

# logout (blacklists the access token, revokes all sessions)
curl -s localhost:3000/auth/logout \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <accessToken>' \
  -d '{"refreshToken":"<refreshToken>"}'
```

### Try the organizations flow

Org-scoped routes need both an `Authorization` header and, past creation, an `x-organization-id` header naming an org the caller belongs to:

```bash
# create an org (caller becomes its "owner"; returns the org row)
curl -s localhost:3000/organizations \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <accessToken>' \
  -d '{"name":"Acme Corp","slug":"acme-corp"}'

# list the caller's orgs (each membership with its organization embedded)
curl -s localhost:3000/organizations \
  -H 'Authorization: Bearer <accessToken>'

# invite a member by email (caller must be owner/admin in the target org)
curl -s localhost:3000/organizations/invite \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <accessToken>' \
  -H 'x-organization-id: <orgId from above>' \
  -d '{"email":"teammate@example.com","role":"member"}'

# remove a member (owner cannot be removed)
curl -s -X DELETE localhost:3000/organizations/members/<userId> \
  -H 'Authorization: Bearer <accessToken>' \
  -H 'x-organization-id: <orgId>'
```

### Try RBAC, subscription, and audit logs

All three route groups need `Authorization` + `x-organization-id`, same as the org-scoped routes above:

```bash
# create a role (returns the raw role row — no embedded permissions)
curl -s localhost:3000/rbac/roles \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <accessToken>' \
  -H 'x-organization-id: <orgId>' \
  -d '{"name":"editor","permissions":["project:create","project:*"]}'

# list roles (each with its permissions embedded)
curl -s localhost:3000/rbac/roles \
  -H 'Authorization: Bearer <accessToken>' \
  -H 'x-organization-id: <orgId>'

# replace a role's permission set
curl -s -X PUT localhost:3000/rbac/roles/<roleId>/permissions \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <accessToken>' \
  -H 'x-organization-id: <orgId>' \
  -d '{"permissions":["doc:read"]}'

# assign a role to a member
curl -s localhost:3000/rbac/assign \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <accessToken>' \
  -H 'x-organization-id: <orgId>' \
  -d '{"userId":"<memberUserId>","roleId":"<roleId>"}'

# get the org's subscription (null if none assigned yet)
curl -s localhost:3000/subscription \
  -H 'Authorization: Bearer <accessToken>' \
  -H 'x-organization-id: <orgId>'

# assign/upsert a plan (planId from the seeded free/pro/enterprise plans)
curl -s localhost:3000/subscription/assign \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer <accessToken>' \
  -H 'x-organization-id: <orgId>' \
  -d '{"planId":"<planId>"}'

# query audit logs (all filters optional: userId, action, limit 1-100 default 50)
curl -s 'localhost:3000/audit-logs?action=org.created&limit=10' \
  -H 'Authorization: Bearer <accessToken>' \
  -H 'x-organization-id: <orgId>'
```

## Frontend

With `make up` (db + redis) and `make api` running, start the dashboard:

```bash
make web   # cd apps/frontend && pnpm dev — Next.js on :4000
```

Open [`localhost:4000`](http://localhost:4000) and register a user — the app redirects `/` → `/login`/`/organizations` depending on session state, and every page (Organizations, Members, Roles, Audit Logs, Subscription) is wired to the live API.

The browser never calls the Go API directly. `app/api/[...path]/route.ts` is a runtime reverse proxy: the browser only ever calls same-origin `/api/*`, and the proxy forwards to `BACKEND_URL` (default `http://localhost:3000`, see `apps/frontend/.env.local.example`) — read fresh on every request, not baked into the build, so the same production image works across environments (compose sets it to `http://api:3000` via the container network). This is a Route Handler rather than a `next.config.ts` `rewrites()` entry deliberately: `next.config.ts` is resolved once at `next build` time, so an env var read there can't reflect a different value at container runtime.

Auth tokens: the access token lives in memory only (lost on a full page reload by design); the refresh token persists in `localStorage` and is used to silently re-authenticate on load. The API client single-flights concurrent 401s through one `/auth/refresh` call and retries the original request once. See `apps/frontend/README.md` for the full breakdown.

```bash
cd apps/frontend
pnpm install
pnpm dev                # :4000
pnpm build               # production build (also runs typecheck)
pnpm exec tsc --noEmit   # typecheck only
pnpm test                # vitest
pnpm lint                # eslint
```

## Docker

`apps/backend/Dockerfile` is a multi-stage build: a `golang:1.26-alpine` builder compiles the API and a small `healthcheck` binary, and the runner is [`gcr.io/distroless/static-debian12:nonroot`](https://github.com/GoogleContainerTools/distroless) — no shell, so `HEALTHCHECK` runs the dedicated `healthcheck` binary instead of `curl`. `apps/frontend/Dockerfile` builds the Next.js standalone output on `node:22-alpine`; the runner sets `HOSTNAME=0.0.0.0` explicitly (the standalone server otherwise binds to the container's assigned network IP, not all interfaces — a loopback-based `HEALTHCHECK` fails silently otherwise).

```bash
docker build -t controlplane-api:dev ./apps/backend
docker build -t controlplane-web:dev ./apps/frontend

# full stack: db, redis, api, web — web waits for api's HEALTHCHECK
docker compose up -d --build
open http://localhost:4000
```

## Kubernetes

Manifests live in [`k8s/`](k8s/), ported from the source app with env-var parity fixes (`APP_ENV`/`APP_NAME` instead of `NODE_ENV`). See [`k8s/README.md`](k8s/README.md) for the full layout and apply instructions:

```bash
cp k8s/secret.example.yaml k8s/secret.yaml
cp k8s/postgres/secret.example.yaml k8s/postgres/secret.yaml
# edit both secret.yaml files with real values, then:
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/ -R
```

The frontend's `web` Deployment/Service aren't ported yet — the `api`/`postgres`/`redis` manifests are current; `web` is a follow-up (compose already runs the full stack, see Docker above).

## Layout

```
apps/backend/    Go API (Echo)
apps/frontend/   Next.js dashboard (App Router + shadcn/ui + TanStack Query)
docs/            Migration analysis, API contract, architecture, plan
k8s/             Kubernetes manifests (api/postgres/redis; web/ not yet ported)
```

## Common commands

```
make up              # start db + redis
make down            # stop all compose services
make migrate         # apply all pending migrations (goose up)
make migrate-down    # roll back the most recent migration
make migrate-status  # show migration status
make seed            # seed default plans (free/pro/enterprise) — idempotent
make sqlc            # regenerate sqlc query code (requires sqlc installed)
make swagger         # regenerate Swagger/OpenAPI docs (requires swag installed)
make api             # run the Go API (go run)
make web             # run the Next.js dev server (pnpm dev)
make build           # build backend binary + frontend production build
make test            # go test ./...
make lint            # golangci-lint if installed, else go vet ./...
make tidy            # go mod tidy
make fmt             # go fmt ./...
```

Frontend-specific commands (`pnpm lint`, `pnpm test`, `pnpm exec tsc --noEmit`) aren't wired into the root Makefile yet — run them from `apps/frontend/`, see [Frontend](#frontend) above.

Full container stack (db, redis, api, web) comes up with `docker compose up -d --build` — see [Docker](#docker) above.
