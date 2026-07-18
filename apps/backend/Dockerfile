# syntax=docker/dockerfile:1

########################
# Builder
########################
FROM golang:1.26-alpine AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

########################
# Runner
########################
FROM alpine:3.20 AS runner

RUN apk add --no-cache ca-certificates curl && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /out/api ./api

USER app

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD curl -f http://127.0.0.1:3000/health || exit 1

ENTRYPOINT ["/app/api"]
