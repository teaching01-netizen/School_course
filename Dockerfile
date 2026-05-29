# syntax=docker/dockerfile:1

FROM node:22-bookworm AS web-build
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY index.html vite.config.ts tsconfig.json ./
COPY src ./src
RUN npm run build

FROM golang:1.25 AS go-build
WORKDIR /app/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/cleanup-idempotency ./cmd/cleanup-idempotency

FROM gcr.io/distroless/base-debian12:nonroot
WORKDIR /app
COPY --from=web-build /app/dist ./dist
COPY --from=go-build /out/server ./server
COPY --from=go-build /out/migrate ./migrate
COPY --from=go-build /out/cleanup-idempotency ./cleanup-idempotency
COPY backend/db/migrations ./db/migrations
ENV STATIC_DIR=/app/dist
EXPOSE 8080
ENTRYPOINT ["/app/server"]
