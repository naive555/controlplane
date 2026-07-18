# syntax=docker/dockerfile:1

########################
# Dependencies
########################
FROM node:22-alpine AS deps
WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

########################
# Build
########################
FROM node:22-alpine AS builder
WORKDIR /app

COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN npm run build

########################
# Runner
########################
FROM node:22-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production
RUN addgroup -S app && adduser -S app -G app

COPY --from=builder /app/public ./public
COPY --from=builder --chown=app:app /app/.next/standalone ./
COPY --from=builder --chown=app:app /app/.next/static ./.next/static

USER app

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1:3000 || exit 1

CMD ["node", "server.js"]
