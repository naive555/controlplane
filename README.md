# controlplane

A monorepo rewrite of [`controlplane-api`](../controlplane-api) (Bun + ElysiaJS) into **Go (backend) + Next.js (frontend)** — a multi-tenant B2B SaaS platform template (auth, organizations, RBAC, audit logs, subscription limits). See [`docs/`](docs/) for the full analysis, API contract, target architecture, and migration plan, and [`CLAUDE.md`](CLAUDE.md) for ground rules.

**Status**: Phase 4 (RBAC, subscription, audit query) complete — `/auth/register`, `/auth/login`, `/auth/refresh`, `/auth/logout` (bcrypt, JWT access/refresh pairs, session rotation with reuse detection, Redis blacklist + login rate limiting), `POST/GET /organizations`, `POST /organizations/invite`, `DELETE /organizations/members/:userId`, `GET/POST /rbac/roles`, `PUT /rbac/roles/:roleId/permissions`, `POST /rbac/assign`, `GET /subscription`, `POST /subscription/assign`, and `GET /audit-logs` are all live. Swagger docs and the frontend land in later phases.

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
make web       # terminal 2 — Next.js dev server (defaults to :3000; auto-shifts to :3001 if :3000 is taken)

curl localhost:3000/health
```

Regenerating sqlc query code (only needed after editing `apps/backend/internal/infra/database/queries/*.sql`) requires the `sqlc` CLI: `go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`, then `make sqlc`.

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

## Layout

```
apps/backend/    Go API (Echo)
apps/frontend/   Next.js dashboard
docs/            Migration analysis, API contract, architecture, plan
k8s/             Kubernetes manifests (ported in a later phase)
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
make api             # run the Go API (go run)
make web             # run the Next.js dev server (pnpm dev)
make build           # build backend binary + frontend production build
make test            # go test ./...
make lint            # go vet ./...
make tidy            # go mod tidy
make fmt             # go fmt ./...
```

Full container stack (including the frontend) can be built with `docker compose build`; the `web` service is defined but commented out in `compose.yaml` until Phase 6 wires up the dashboard.
