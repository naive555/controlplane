.PHONY: up down dev api web build test lint tidy fmt migrate migrate-down migrate-status

## Start Postgres + Redis in the background
up:
	docker compose up -d db redis

## Stop and remove all compose services
down:
	docker compose down

## Run backend and frontend together.
## No process manager is used in Phase 0 — open two terminals instead:
##   make api
##   make web
dev:
	@echo "Run these in two separate terminals:"
	@echo "  make api   # Go API on :3000"
	@echo "  make web   # Next.js dev server"

## Run the Go API (requires `make up` first and a .env file)
api:
	cd apps/backend && go run ./cmd/api

## Apply all pending database migrations
migrate:
	cd apps/backend && go run ./cmd/migrate up

## Roll back the most recent migration
migrate-down:
	cd apps/backend && go run ./cmd/migrate down

## Show migration status
migrate-status:
	cd apps/backend && go run ./cmd/migrate status

## Run the Next.js dev server
web:
	cd apps/frontend && pnpm dev

## Build backend binary and frontend production build
build:
	cd apps/backend && go build -o bin/api ./cmd/api
	cd apps/frontend && pnpm build

## Run backend tests
test:
	cd apps/backend && go test ./...

## Vet the backend (golangci-lint added in a later phase)
lint:
	cd apps/backend && go vet ./...

## Tidy go.mod/go.sum
tidy:
	cd apps/backend && go mod tidy

## Format Go source
fmt:
	cd apps/backend && go fmt ./...
