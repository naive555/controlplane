# CLAUDE.md

## What this repo is

**controlplane** — a monorepo rewrite of `controlplane-api` (Bun + ElysiaJS, located at `../controlplane-api`) into **Go (backend) + Next.js (frontend)**. It is a multi-tenant B2B SaaS platform template: JWT auth with refresh-token rotation, organizations/memberships, custom RBAC with wildcard permissions, immutable audit logs, and subscription plan-limit enforcement. Business domains get added on top of this core.

**Status**: Phase 3 (org + guards) complete. `/auth/{register,login,refresh,logout}` are live (Phase 2): bcrypt (cost 12) password hashing, HS256 JWT access/refresh pair issuance, session create + rotation with token-family reuse detection (a reused/revoked refresh token revokes its whole family), Redis-backed access-token blacklist and login rate limiting (5 attempts / 15 min), and best-effort audit logging of `user.register`/`user.login`. `RequireAuth`/`RequireOrg` middleware now guard the org routes (`RequirePermission`/RBAC lands in Phase 4): `POST /organizations` (create + owner membership in a tx), `GET /organizations` (caller's memberships with embedded org), `POST /organizations/invite` (role check + `max_members` plan-limit enforcement), `DELETE /organizations/members/:userId` (role check + cannot-remove-owner) — with best-effort `org.created`/`org.member.invited` audit logging. RBAC, subscription HTTP endpoints, and the audit-log query endpoint land in Phase 4. See [`README.md`](README.md) for the quickstart to run it. `docs/` holds the analysis and migration plan. Read `docs/` before implementing anything.

## Decided stack (do not re-litigate without the owner)

- **Backend**: Go, **Echo** framework, **sqlc + pgx/v5**, **goose** migrations, go-redis v9, golang-jwt/v5, bcrypt (cost 12), slog logging, swaggo (`/swagger`)
- **Frontend**: **Next.js (App Router) + TypeScript + Tailwind + shadcn/ui**, TanStack Query
- **Infra**: PostgreSQL 16, Redis 7, root **Makefile** + **docker-compose**, k8s manifests, GitHub Actions

## Layout

```
apps/backend/    Go API — cmd/{api,migrate,seed}, internal/{config,server,middleware,module,infra,shared}, migrations/
apps/frontend/   Next.js dashboard
docs/            01-source-analysis · 02-api-contract · 03-target-architecture · 04-migration-plan
```

## Ground rules

- **`docs/02-api-contract.md` is the source of truth** for routes, headers, status codes, and error messages. The Go backend must match it exactly; intentional deviations are listed in `docs/03` "Open questions" and must be documented when resolved.
- Module convention (mirrors the source): handler → service → sqlc queries per module (`auth`, `organization`, `rbac`, `auditlog`, `subscription`, `health`). Services return `apperror` codes; a single Echo `HTTPErrorHandler` maps codes to HTTP responses. No HTTP concerns inside services.
- Auth guards are middleware: `RequireAuth`, `RequireOrg` (needs `x-organization-id` header + membership), `RequirePermission(action)`. Permission semantics: `*` > exact `resource:verb` > `resource:*` wildcard.
- DB schema must stay byte-identical to the source migrations (the Drizzle SQL in `../controlplane-api/src/infrastructure/database/migrations/`) unless a deviation is agreed.
- Audit-log writes are best-effort: log failures, never fail the request.
- Redis keys: `blacklist:<accessToken>` (15 min), `login:attempts:<email>` (max 5 per 15 min).
- Multi-step writes (org create + owner membership; session rotation) run in transactions.

## Commands (once scaffolded)

```
make up        # start db + redis
make dev       # backend (air) + frontend (next dev)
make migrate   # goose up          make seed   # default plans
make sqlc      # regen query code  make test   # go test + frontend tests
make lint      # golangci-lint + eslint
```

Backend-only during early phases: `cd apps/backend && go run ./cmd/api`, `go test ./...`. Regenerating sqlc code requires the `sqlc` CLI (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`); building/running the API does not.

## Environment

Copy `.env.example` → `.env`. Required: `DATABASE_URL`, `REDIS_URL`, `JWT_ACCESS_SECRET`/`JWT_REFRESH_SECRET` (min 32 chars), `JWT_ACCESS_EXPIRES_IN` (duration, default 15m), `JWT_REFRESH_EXPIRES_IN` (**seconds**, default 604800), `PORT` (3000), `LOG_LEVEL`, `APP_ENV`.

## Testing expectations

- Unit tests per service with interface mocks (mirrors source's mocked-infra tests).
- Integration/parity tests against real postgres+redis (CI service containers) encoding the contract in `docs/02` — every route × happy path × every error code.
- Auth edge cases that must be covered: refresh rotation, token-family reuse → revoke family, rate limit at 5 attempts, logout revokes all sessions + blacklists access token.
