# controlplane

A monorepo rewrite of [`controlplane-api`](../controlplane-api) (Bun + ElysiaJS) into **Go (backend) + Next.js (frontend)** — a multi-tenant B2B SaaS platform template (auth, organizations, RBAC, audit logs, subscription limits). See [`docs/`](docs/) for the full analysis, API contract, target architecture, and migration plan, and [`CLAUDE.md`](CLAUDE.md) for ground rules.

**Status**: Phase 1 (data layer) complete — schema migrated via goose, default plans seeded, sqlc queries for users/sessions/plans. No auth/business logic yet.

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
