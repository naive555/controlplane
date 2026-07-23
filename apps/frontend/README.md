# controlplane frontend

Next.js (App Router) dashboard for the [controlplane](../../README.md) B2B SaaS template — auth, organizations + switcher, members, RBAC roles, audit logs, subscription. TypeScript + Tailwind v4 + [shadcn/ui](https://ui.shadcn.com) (`base-nova` preset) + [TanStack Query](https://tanstack.com/query).

## Running it

Requires the backend running first — see the [root README](../../README.md) (`make up && make migrate && make seed && make api`).

```bash
pnpm install
pnpm dev   # http://localhost:4000
```

`next dev` runs on **:4000**, not the framework default :3000 — the Go API already owns :3000, and both need to be up at once during development.

Copy `.env.local.example` → `.env.local` to override `BACKEND_URL` (defaults to `http://localhost:3000`, correct for local dev against `make api`).

## How the browser talks to the API

The browser **never** calls the Go API directly — it only ever calls same-origin `/api/*`. `app/api/[...path]/route.ts` is a runtime reverse proxy (a Next.js Route Handler) that forwards each request to `BACKEND_URL`.

This is deliberately **not** a `next.config.ts` `rewrites()` entry. `next.config.ts` is evaluated once during `next build` and its resolved output (including a `rewrites()` destination) is baked into the standalone server's manifest — a container-runtime env var set *after* the build (e.g. `docker compose`'s `BACKEND_URL=http://api:3000`) would never take effect. A Route Handler reads `process.env.BACKEND_URL` fresh on every request instead, so the same built image works unmodified across environments (local dev, compose, a future k8s deploy) — see [Next's own docs on runtime environment variables](https://nextjs.org/docs/app/guides/environment-variables#runtime-environment-variables) for the pattern this follows.

Same-origin-only also means **no CORS configuration exists on the backend** — it was never needed.

## Auth / token model

- **Access token**: held in memory only (a module-level variable in `lib/auth/token-store.ts`), never persisted. Lost on every full page reload by design.
- **Refresh token**: persisted in `localStorage`. On mount, `SessionProvider` (`lib/auth/use-session.tsx`) uses it to silently re-authenticate if there's no in-memory access token yet.
- **Single-flight refresh**: the API client (`lib/api/client.ts`) catches a `401`, and if it's not itself the refresh call, single-flights a shared `/auth/refresh` call across any concurrent requests, then retries the original request once. A failed refresh clears all tokens and the app falls back to anonymous.
- **Active org**: `lib/org/active-org.ts` tracks the selected organization id (also in `localStorage`, under a separate key) via a small pub-sub in `token-store.ts` + `useSyncExternalStore`, so every component reading it (nav gating, the org switcher, org-scoped queries) stays in sync without prop drilling. Selecting a different org invalidates every org-scoped TanStack Query.
- **`localStorage` is shared across tabs on the same origin** — opening a second tab does not give you a second, independent session; it inherits whatever refresh token is currently stored. Worth knowing when testing multi-user flows locally.

## Pages

| Route | Notes |
| --- | --- |
| `/login`, `/register` | `(auth)` route group; redirects to `/organizations` if already authed |
| `/organizations` | list + create (dialog) + switch active org |
| `/members` | roster, invite, remove — invite/remove disabled for `member`-role callers (mirrors the backend's own check) |
| `/roles` | RBAC: create role, edit permissions (textarea, one per line), assign to a member |
| `/audit` | filterable by action / user / limit |
| `/subscription` | current plan + limits, assign a plan (via `GET /plans`, a Phase-6-only backend addition — see root `docs/03-target-architecture.md`) |

All dashboard routes live under the `(dashboard)` route group, whose layout is the auth guard: redirects anonymous callers to `/login`, shows a skeleton while the session is resolving.

## Commands

```bash
pnpm dev                 # dev server, :4000
pnpm build                # production build (runs typecheck as part of the build)
pnpm exec tsc --noEmit    # typecheck only
pnpm test                 # vitest (lib/api/client.test.ts — single-flight refresh coverage)
pnpm lint                 # eslint
```

## Docker

```bash
docker build -t controlplane-web:dev .
```

Standalone output (`output: "standalone"` in `next.config.ts`). The runner sets `HOSTNAME=0.0.0.0` explicitly — without it the standalone server binds to the container's assigned network IP rather than all interfaces, which breaks the loopback-based `HEALTHCHECK` even though external port-forwarding still works. See the root `compose.yaml`'s `web` service for how this is wired into the full stack.
