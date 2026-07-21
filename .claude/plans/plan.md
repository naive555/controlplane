# Phase 6 — Frontend (Next.js dashboard) — Implementation Plan

> **Status: 🔲 Not started.** Target executor: **Sonnet**. This plan is
> prescriptive: file paths, exact commands, and copy-paste-ready snippets.
> Read `docs/04-migration-plan.md` §"Phase 6", `docs/03-target-architecture.md`
> (frontend rows + Open questions), and `docs/02-api-contract.md` (the route/
> header/status contract the client must speak) **before** writing code.
>
> **⚠️ Next.js 16 caveat — read this first.** `apps/frontend/AGENTS.md` warns:
> *"This is NOT the Next.js you know. This version has breaking changes — APIs,
> conventions, and file structure may all differ from your training data. Read
> the relevant guide in `node_modules/next/dist/docs/` before writing any
> code."* This is binding. The scaffold is **Next 16.2.10 + React 19.2 +
> Tailwind v4**. Do **not** assume App Router conventions from memory (async
> `params`/`searchParams`, caching defaults, `next.config` shape, metadata,
> font imports, route handlers all changed across 15→16). For every Next API
> you touch, open the matching file under `apps/frontend/node_modules/next/dist/docs/`
> and confirm the current signature first.

## Decisions locked for this phase (owner-approved 2026-07-21)

1. **Networking + token model = Client tokens + Next proxy.** Access token
   held **in memory** (module-level var), refresh token in **`localStorage`**,
   **single-flight** refresh on 401 → retry once. The browser only ever calls
   **same-origin** `/api/*`; `next.config.ts` **rewrites** proxy `/api/:path*`
   → `${BACKEND_URL}/:path*`. **No backend CORS is added** (the browser never
   makes a cross-origin request). This matches the migration plan's "API
   client (refresh single-flight, org header)".
2. **Add `GET /organizations/members`** to the Go backend (Step 1). Resolves
   `docs/03` Open question #2. Documented as an intentional deviation. The
   members page needs a roster; deriving it from audit logs is rejected.
3. **UI stack = shadcn/ui + TanStack Query** (the decided stack in `CLAUDE.md`).
   Forms with **react-hook-form + zod**; toasts with **sonner**.

### Non-goals (defer / out of scope)

- No changes to any existing backend handler/service **behavior** except the
  single **new** `GET /organizations/members` route in Step 1.
- No SSR data fetching of protected resources / no RSC calls to the API with
  server-held tokens (that's the BFF model we did **not** pick). All API calls
  are client-side through the proxy.
- No i18n, no dark-mode toggle beyond what shadcn ships, no E2E/Playwright
  (component/unit tests + build/lint/typecheck are the CI gate — Step 11).
- k8s `web` Deployment is **optional stretch** (Step 11e); compose + CI are the
  required infra deliverables.

## Ground truth captured from the codebase

- Scaffold state (`apps/frontend/`): Next 16.2.10, React 19.2.4, Tailwind v4,
  ESLint 9 flat config, `output: "standalone"`, `pnpm@11.14.0`, path alias
  `@/*` → `./*`. **No** shadcn, TanStack Query, API client, or real pages yet
  (`app/page.tsx` is a placeholder). A `Dockerfile` (node:22-alpine,
  standalone) and `.dockerignore` already exist. `.env.local.example` has only
  `NEXT_PUBLIC_API_URL=http://localhost:3000`.
- **Dev-port conflict**: the Go API listens on **:3000**, and `next dev`
  defaults to **:3000** too. The frontend dev server MUST run on a different
  port (**:4000** — matches the commented compose `web` port mapping
  `4000:3000`).
- Backend contract (`docs/02-api-contract.md`), 16 routes + the new members
  route. Error body is always `{ "message": string }`. Guard headers:
  `Authorization: Bearer <accessToken>`, `x-organization-id: <uuid>`.
  `x-request-id` is echoed if sent.
- Backend org module lives at `apps/backend/internal/module/organization/`
  (`handler.go`, `service.go`, `dto.go`, `service_test.go`). Routes registered
  in `handler.go`'s `Register`: `POST ""`, `GET ""`, `POST /invite`,
  `DELETE /members/:userId`. sqlc queries under
  `apps/backend/internal/infra/database/` (generated `db` package).
- Schema (`apps/backend/migrations/`): `memberships(id, organization_id,
  user_id, role default 'member', created_at)`; `users(id, email,
  display_name, created_at, ...)`. A member's role for invite/remove is the
  `memberships.role` text column (`owner`/`admin`/`member`).
- Compose `web:` service block exists **commented** in `compose.yaml`
  (lines ~47–55, maps `4000:3000`). CI (`.github/workflows/ci.yml`) already has
  a `frontend` job (lint + build) — verify/extend, don't duplicate.

---

## Step 1 — Backend: `GET /organizations/members` — 🔲

Add one org-guarded route returning the org's member roster. Mirror the
existing org module's structure exactly.

### 1a. sqlc query

Add to the organization queries `.sql` file (find it with
`grep -rl "ListMemberships\|memberships" apps/backend/internal/infra/database/query*`;
it's the same file that backs `GET /organizations`). Add:

```sql
-- name: ListOrganizationMembers :many
SELECT m.user_id, u.email, u.display_name, m.role, m.created_at AS joined_at
FROM memberships m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = $1
ORDER BY m.created_at ASC;
```

Regenerate: `cd apps/backend && sqlc generate` (or `make sqlc` from root).
Requires the `sqlc` CLI (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest`).
Commit the regenerated `db` package.

### 1b. Service method

In `apps/backend/internal/module/organization/service.go`, add:

```go
// ListMembers returns the roster for an organization. Guarded by RequireOrg,
// so the caller is already verified as a member.
func (s *Service) ListMembers(ctx context.Context, orgID uuid.UUID) ([]db.ListOrganizationMembersRow, error) {
	return s.queries.ListOrganizationMembers(ctx, orgID)
}
```

(Match the receiver/field names already used in this file — inspect the
existing `List`/`Invite` methods for the exact `s.queries` vs `s.store`
accessor and error-wrapping style.)

### 1c. DTO + handler

In `dto.go`, add a `MemberResponse` struct and a mapper:

```go
type MemberResponse struct {
	UserID      string  `json:"userId"`
	Email       string  `json:"email"`
	DisplayName *string `json:"displayName"`
	Role        string  `json:"role"`
	JoinedAt    string  `json:"joinedAt"` // RFC3339
}
```

In `handler.go`, register the route in `Register` (org-guarded) and add the
handler, following the swaggo annotation pattern of the sibling handlers:

```go
g.GET("/members", h.listMembers, guards.RequireOrg())
```

```go
// listMembers returns the active organization's member roster.
// @Summary  List organization members
// @Tags     organizations
// @Security BearerAuth
// @Produce  json
// @Param    x-organization-id  header    string  true  "Active organization id"
// @Success  200  {array}   MemberResponse
// @Failure  400  {object}  httpx.ErrorResponse  "Missing x-organization-id header"
// @Failure  401  {object}  httpx.ErrorResponse  "Unauthorized"
// @Failure  403  {object}  httpx.ErrorResponse  "Not a member of this organization"
// @Router   /organizations/members [get]
func (h *Handler) listMembers(c echo.Context) error {
	rows, err := h.service.ListMembers(c.Request().Context(), appmw.OrgID(c))
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, toMemberResponses(rows))
}
```

Confirm the org-id accessor name (`appmw.OrgID(c)` vs similar) against
`internal/middleware/` — use whatever `RequireOrg` actually sets.

### 1d. Tests, swagger, docs

- Add a `service_test.go` case for `ListMembers` following the existing mocked-
  queries pattern in that file (happy path + a query-error passthrough).
- Regenerate swagger: `make swagger` (from root) or
  `cd apps/backend && swag init -g cmd/api/main.go -o docs --parseDependency --parseInternal --useStructName`.
  Commit the updated `docs/`.
- Update `docs/02-api-contract.md` (add the row under Organizations) and
  `docs/03-target-architecture.md` (mark Open question #2 resolved, note the
  deviation). Update `CLAUDE.md`'s status paragraph's org-routes sentence to
  mention the members route.

### 1e. Verify

```bash
cd apps/backend && go build ./... && go vet ./... && go test ./...
```

**Acceptance**: `GET /organizations/members` with a valid token + valid
`x-organization-id` returns `200` + `[{userId,email,displayName,role,joinedAt}]`;
missing header → `400 Missing x-organization-id header`; non-member → `403`.

---

## Step 2 — Frontend foundation (deps, shadcn, proxy, dev port) — 🔲

> Run all `pnpm` commands from `apps/frontend/`. Read
> `node_modules/next/dist/docs/` for `next.config` + route-handler shapes
> before editing config.

### 2a. Dev port (fix the :3000 conflict)

`apps/frontend/package.json` → change the dev script:

```json
"dev": "next dev -p 4000",
```

### 2b. Proxy rewrites + env

`apps/frontend/next.config.ts` — add `rewrites` (keep `output: "standalone"`).
**Verify the `rewrites` signature in the local Next 16 docs first.**

```ts
import type { NextConfig } from "next";

const BACKEND_URL = process.env.BACKEND_URL ?? "http://localhost:3000";

const nextConfig: NextConfig = {
  output: "standalone",
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${BACKEND_URL}/:path*` }];
  },
};

export default nextConfig;
```

`.env.local.example` — replace with:

```
# Server-side only: where the Next.js rewrite proxy forwards /api/* .
# The browser never calls this directly (same-origin /api/* only).
BACKEND_URL=http://localhost:3000
```

The browser always calls `/api/...` (same-origin, no CORS). `BACKEND_URL` is a
**server** env (no `NEXT_PUBLIC_` prefix). Drop the old `NEXT_PUBLIC_API_URL`.

### 2c. Install runtime deps

```bash
pnpm add @tanstack/react-query react-hook-form zod @hookform/resolvers sonner
pnpm add -D vitest @testing-library/react @testing-library/jest-dom jsdom @vitejs/plugin-react
```

### 2d. shadcn/ui init (Tailwind v4)

Use the current CLI (verify flags against shadcn docs — it supports Tailwind
v4 / React 19; pass `--force` if it warns about React 19 peer deps):

```bash
pnpm dlx shadcn@latest init
```

Answer prompts for App Router + the existing `app/globals.css` + `@/*` alias.
Then add the components this phase uses:

```bash
pnpm dlx shadcn@latest add button input label card table dialog \
  dropdown-menu form select badge sonner skeleton
```

Commit the generated `components/ui/*`, `lib/utils.ts`, and the
`components.json`. If shadcn rewrites `globals.css`/`tsconfig` paths, keep its
version.

**Acceptance**: `pnpm build` succeeds; `pnpm dev` serves on `:4000`; a smoke
page importing one shadcn `<Button>` renders.

---

## Step 3 — Auth/token layer (the core of this phase) — 🔲

Create `apps/frontend/lib/` modules. This is where correctness matters most —
mirror the backend's rotation/blacklist semantics.

### 3a. Token store — `lib/auth/token-store.ts`

- `accessToken`: module-level variable (memory only — lost on refresh, that's
  intended).
- `refreshToken`: persisted in `localStorage` under a fixed key
  (`cp.refreshToken`). Getter/setter/clear.
- `activeOrgId`: persisted in `localStorage` (`cp.activeOrgId`) — see Step 6.
- Export `getAccessToken`, `setTokens({accessToken, refreshToken})`,
  `clearTokens`, `getRefreshToken`.

### 3b. API client — `lib/api/client.ts`

A typed `fetch` wrapper. Requirements:

- Base path `/api` (same-origin → proxy). Never hardcode the backend origin.
- Injects `Authorization: Bearer <accessToken>` when present.
- Injects `x-organization-id: <activeOrgId>` for org-scoped calls (accept an
  `org: boolean` option, or always send when an active org is set).
- Parses `{ message }` error bodies into a thrown `ApiError { status, message }`.
- **Single-flight refresh on 401**: on a `401` (that isn't itself the refresh
  call), call a shared `refresh()` promise (deduped via a module-level
  `let refreshing: Promise<...> | null`), then **retry the original request
  once** with the new access token. If refresh fails (401 from
  `/auth/refresh`), `clearTokens()` and reject so the UI redirects to
  `/login`. Do **not** retry a second time.

```ts
// sketch — refresh single-flight
let refreshing: Promise<void> | null = null;

async function ensureRefreshed(): Promise<void> {
  if (!refreshing) {
    refreshing = doRefresh().finally(() => { refreshing = null; });
  }
  return refreshing;
}
```

`doRefresh()` POSTs `/api/auth/refresh` with `{ refreshToken }`, then
`setTokens(...)` with the rotated pair. A `401`/`REFRESH_TOKEN_REUSE` here is
terminal → `clearTokens()`.

### 3c. Endpoint functions — `lib/api/endpoints.ts`

Thin typed wrappers per contract route, e.g. `login`, `register`, `logout`,
`refresh`, `listOrganizations`, `createOrganization`, `listMembers`, `invite`,
`removeMember`, `listRoles`, `createRole`, `updatePermissions`, `assignRole`,
`getSubscription`, `assignSubscription`, `getAuditLogs`. Each maps 1:1 to
`docs/02-api-contract.md` (method, path, body, response). Co-locate TS types
for request/response bodies (define from the contract — don't import Go).

### 3d. Session bootstrap + query provider

- `app/providers.tsx` (client component): wraps children in
  `QueryClientProvider` (+ `<Toaster />` from sonner). Mount it in
  `app/layout.tsx`.
- `lib/auth/use-session.ts`: a hook/context that on mount, if a refresh token
  exists but no access token, calls `refresh()` to rehydrate the session
  (bootstraps a page reload). Exposes `{ status: 'loading'|'authed'|'anon',
  user }`. Decode `user` from the access-token JWT payload (`sub`, `email`) —
  no `/me` endpoint exists, so decode the JWT (base64 payload) client-side; do
  not verify (informational only).

### 3e. Tests (Vitest)

`lib/api/client.test.ts` — mock `fetch` and assert:
- a `401` triggers exactly one refresh then one retry;
- concurrent 401s share a single refresh (single-flight);
- a failed refresh clears tokens and rejects.

Add `vitest.config.ts` (jsdom env) + `"test": "vitest run"` to
`package.json` scripts.

**Acceptance**: `pnpm test` green; manual: log in, delete the in-memory access
token (or wait for 401), next call transparently refreshes.

---

## Step 4 — Routing, app shell, auth guard — 🔲

> Confirm route-group + layout conventions in the local Next 16 docs.

- **Public group** `app/(auth)/`: `login/page.tsx`, `register/page.tsx` with a
  minimal centered layout.
- **Protected group** `app/(dashboard)/`: `layout.tsx` renders the app shell
  (sidebar nav: Organizations, Members, Roles, Audit, Subscription + an org
  switcher in the header + a logout button). The layout is a client component
  that reads `use-session`: while `loading` show a skeleton; if `anon`
  redirect to `/login`; if `authed` render nav + `children`.
- Redirect `/` → `/organizations` when authed, `/login` when anon.
- Nav items that require an active org (Members/Roles/Audit/Subscription) are
  disabled until an org is selected (Step 6).

**Acceptance**: hitting a dashboard route while logged out redirects to
`/login`; logged in shows the shell.

---

## Step 5 — Auth pages — 🔲

- **Login** (`(auth)/login`): react-hook-form + zod (`email`, `password`).
  Submit → `login()` → `setTokens()` → invalidate/refetch → redirect to
  `/organizations`. Map contract errors to inline/toast messages:
  `401 Invalid email or password`, `429 Too many login attempts...`.
- **Register** (`(auth)/register`): fields `email`, `password` (min 8),
  `displayName?` (min 1). On success stores tokens (register returns a token
  pair) → redirect to `/organizations` (empty state prompts "create your first
  org"). Handle `409 Email already taken`, `422 Validation failed`.
- Cross-link login ↔ register.

**Acceptance**: register a new user → lands authenticated; login with wrong
password → shows the exact contract message.

---

## Step 6 — Organizations + org switcher — 🔲

- **Data**: `useQuery(['organizations'], listOrganizations)` →
  memberships with embedded org objects (per contract). Derive the org list +
  the caller's role per org from this.
- **Active org**: a small context/store backed by `localStorage`
  (`cp.activeOrgId`). The API client reads it for `x-organization-id`. On login
  or when the list loads, default to the first org if none selected. Switching
  orgs updates the store and invalidates all org-scoped queries
  (`queryClient.invalidateQueries()`), since the header changed.
- **Org switcher**: `dropdown-menu` in the header listing the caller's orgs +
  a "Create organization" item.
- **Organizations page** (`(dashboard)/organizations`): a `table` of the
  caller's orgs (name, slug, role) + a "Create organization" `dialog`
  (react-hook-form: `name` min 1, `slug` min 2 `^[a-z0-9-]+$`). Create →
  `createOrganization()` → invalidate `['organizations']` → optionally
  auto-select the new org. Handle `409 SLUG_TAKEN`.

**Acceptance**: create an org, see it in the list + switcher, select it →
Members/Roles/Audit/Subscription nav enables and subsequent calls carry the
right `x-organization-id`.

---

## Step 7 — Members page — 🔲

Route `(dashboard)/members`. Uses the Step 1 endpoint.

- **Roster**: `useQuery(['members', activeOrgId], listMembers)` → `table`
  (email, display name, role badge, joined date).
- **Invite** `dialog`: react-hook-form (`email`, `role: select admin|member`)
  → `invite()` → invalidate `['members']`. Map errors: `403 Insufficient
  permissions` (caller is a `member`), `403 Plan limit exceeded`
  (`max_members`), `404 User not found`, `409 User is already a member`.
- **Remove**: per-row action (guarded confirm `dialog`) → `removeMember(userId)`
  → invalidate. Hide/disable the action for the `owner` row and map
  `403 Cannot remove organization owner`.
- Invite/remove controls are disabled when the caller's role in the active org
  is `member` (derive role from Step 6's org list).

**Acceptance**: invite a member (create a 2nd user first), see them in the
roster, remove them; owner row can't be removed; a `member`-role caller sees
disabled controls and the API still enforces `403`.

---

## Step 8 — Roles (RBAC) page — 🔲

Route `(dashboard)/roles`.

- **List** roles: `useQuery(['roles', activeOrgId], listRoles)` → `table`
  (name, description, permission count/badges).
- **Create role** `dialog`: `name` (min 1), `description?`, `permissions`
  (string list — a tag/textarea input; permissions are free-form
  `resource:verb`/`resource:*`/`*` strings per contract) → `createRole()`.
- **Edit permissions**: per-row `dialog` → `updatePermissions(roleId, perms)`
  (replaces the set). Map `404 Role not found`.
- **Assign role**: a `dialog` (`userId` from the members roster select +
  `roleId` from the roles select) → `assignRole()`. Reuse the Step 7 members
  query to populate the user select instead of a raw uuid field.

**Acceptance**: create a role, edit its permissions, assign it to a member;
duplicate-permission input surfaces the backend `500` gracefully (toast, no
crash — note the contract keeps this as bug-for-bug `500`).

---

## Step 9 — Audit logs page — 🔲

Route `(dashboard)/audit`.

- **Table**: `useQuery(['audit', activeOrgId, filters], getAuditLogs)` →
  newest-first list (action, actor userId, target, timestamp, metadata).
- **Filters**: `userId?`, `action?` (a `select` of known actions:
  `user.login`, `user.register`, `org.created`, `org.member.invited`,
  `org.member.removed`, `role.created`, `role.assigned`), `limit` (1–100,
  default 50). Filters feed the query key + are sent as query params.

**Acceptance**: after doing actions in earlier steps, the audit page lists
`org.created`/`org.member.invited` etc.; filtering by action/userId narrows the
list.

---

## Step 10 — Subscription page — 🔲

Route `(dashboard)/subscription`.

- **View**: `useQuery(['subscription', activeOrgId], getSubscription)` →
  `card` with the org's plan (nullable → "No plan assigned" empty state) and
  its limits (`max_members`, etc. from the embedded plan).
- **Assign plan**: a `select` of plan ids → `assignSubscription(planId)` →
  invalidate `['subscription']`. Plan ids: the backend has no "list plans"
  endpoint — seed plans are known (`make seed` inserts the 3 defaults). Provide
  the seeded plan ids/names as a static list in the frontend (document this in
  a comment referencing `cmd/seed`), or, if trivial, note as a follow-up that a
  `GET /plans` endpoint would remove the hardcoding (**do not** add it this
  phase — out of scope).
- Note the contract quirk (no admin check on assign) in a code comment; the UI
  still shows it to any org member per parity.

**Acceptance**: assign a seeded plan → subscription card reflects it and the
Members page's `max_members` enforcement changes accordingly.

---

## Step 11 — Infra: compose, CI, Makefile, Docker — 🔲

### 11a. Makefile `web` target

Confirm/adjust the root `Makefile` `web` target runs the frontend on :4000:

```make
web:
	cd apps/frontend && pnpm install && pnpm dev
```

(`make dev` already prints the two-terminal instructions — verify `make web`
exists and points here.)

### 11b. compose `web` service

Uncomment the `web:` block in `compose.yaml` (lines ~47–55). Ensure it:
- builds `./apps/frontend`,
- maps `4000:3000`,
- sets `environment: BACKEND_URL=http://api:3000` (service-name DNS inside the
  compose network — **not** localhost),
- `depends_on: [api]`.

### 11c. Dockerfile

The existing `apps/frontend/Dockerfile` (node:22-alpine, standalone) is fine.
Verify it builds after the new deps: `docker build -t controlplane-web:dev ./apps/frontend`.
The `HEALTHCHECK wget http://127.0.0.1:3000` hits the in-container port 3000
(correct — the container listens on 3000; host maps 4000).

### 11d. CI frontend job

`.github/workflows/ci.yml` already has a `frontend` job (lint + build). Extend
it to also run typecheck + tests:

```yaml
- run: pnpm install --frozen-lockfile
- run: pnpm lint
- run: pnpm exec tsc --noEmit
- run: pnpm test
- run: pnpm build
  env:
    BACKEND_URL: http://localhost:3000   # build-time; no live backend needed
```

Keep pnpm setup consistent with the existing job (corepack / `pnpm/action-setup`).

### 11e. (Stretch, optional) k8s `web`

Only if time allows: add `k8s/web/{deployment,service}.yaml` mirroring
`k8s/api/`, image `controlplane-web`, env `BACKEND_URL=http://<api-svc>:3000`,
and update `k8s/README.md`'s "Not here yet" note. Otherwise leave the note as
Phase-6-optional.

**Acceptance**: `docker compose up -d --build` brings up db, redis, api, web;
the web container serves the dashboard on host :4000 and its `/api/*` calls
reach the api service.

---

## Step 12 — Docs — 🔲

- **`apps/frontend/README.md`**: how to run (`pnpm install`, `pnpm dev` →
  :4000, `BACKEND_URL`), the proxy model (same-origin `/api/*`), token model,
  test/build commands.
- **Root `README.md`**: update status to "Phase 6 ... complete"; add a
  "Frontend" section (dev URL :4000, the `BACKEND_URL` proxy, `make web`).
  Update the `Layout` `apps/frontend/` line.
- **`CLAUDE.md`**: status paragraph → Phase 6 complete; note the new members
  endpoint, the Next-proxy/token model, and that the frontend now exercises
  every module.
- **`docs/03-target-architecture.md`**: mark Open question #2 (members
  endpoint) resolved with the deviation note; record the frontend
  networking/token decision (client tokens + Next proxy).
- **`docs/02-api-contract.md`**: the members row from Step 1.

---

## Step 13 — Verify end-to-end — 🔲

```bash
# infra + backend
make up && make migrate && make seed
cd apps/backend && go run ./cmd/api &     # :3000
# frontend
cd apps/frontend && pnpm install && pnpm dev   # :4000
```

Then walk the full flow in the browser at `http://localhost:4000`:

1. Register → lands authed on an empty Organizations page.
2. Create an org → appears in list + switcher, auto-selected.
3. Register a 2nd user (separate tab/incognito) to have an invitee.
4. Members: invite the 2nd user → roster shows both; remove them; confirm the
   owner row is non-removable.
5. Roles: create a role, edit permissions, assign it.
6. Audit: see `org.created` / `org.member.invited` / `org.member.removed`;
   filter by action + userId.
7. Subscription: assign a seeded plan → card updates.
8. Token flow: let the access token expire (or clear it) → next action
   transparently refreshes (single-flight, one retry). Logout → all sessions
   revoked, redirected to `/login`, protected routes bounce.
9. `pnpm build`, `pnpm test`, `pnpm lint`, `pnpm exec tsc --noEmit` all green.
10. `cd apps/backend && go test ./...` green (Step 1 addition).

## Definition of done

1. 🔲 `GET /organizations/members` live, tested, swagger + contract docs
   updated; `go test ./...` green.
2. 🔲 Frontend runs on :4000, talks to the Go API through the same-origin
   `/api/*` proxy (no CORS added), with in-memory access token + localStorage
   refresh token + single-flight refresh (unit-tested).
3. 🔲 All modules have working pages: auth (login/register), organizations +
   switcher, members, roles, audit logs, subscription — each mapped to the
   contract and surfacing the exact error messages.
4. 🔲 `pnpm build`/`lint`/`tsc --noEmit`/`test` green; CI `frontend` job runs
   all four.
5. 🔲 `docker compose up -d --build` brings up the full stack incl. `web`.
6. 🔲 README + CLAUDE.md + docs updated (status, members deviation, frontend
   networking/token decision).

## Suggested commit sequence

1. `feat(api): add GET /organizations/members endpoint` ← Step 1
2. `chore(web): deps, shadcn init, dev port 4000, /api proxy rewrites` ← Step 2
3. `feat(web): api client + token store + single-flight refresh (+tests)` ← Step 3
4. `feat(web): app shell, auth guard, routing` ← Step 4
5. `feat(web): login + register pages` ← Step 5
6. `feat(web): organizations page + org switcher` ← Step 6
7. `feat(web): members page (roster/invite/remove)` ← Step 7
8. `feat(web): roles page (RBAC)` ← Step 8
9. `feat(web): audit logs page` ← Step 9
10. `feat(web): subscription page` ← Step 10
11. `ci(web): compose web service, extend frontend CI, Makefile web` ← Step 11
12. `docs: update README/CLAUDE/contract for Phase 6` ← Step 12
