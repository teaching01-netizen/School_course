.PHONY: dev build test test-scheduling-db lint fmt migrate-up migrate-down migrate-status migrate-validate sqlc tools

dev:
	export $$(grep -v '^\s*#' ../.env 2>/dev/null | grep -v '^\s*$$' | sed 's/[[:space:]]*#.*//' | xargs) 2>/dev/null; \
	ADDR=$${ADDR:-:8080} \
	STATIC_DIR=$${STATIC_DIR:-../dist} \
	OTP_HMAC_KEY=$${OTP_HMAC_KEY:-dev-otp-hmac-key-change-in-production-32bytes} \
	go run ./cmd/server

build:
	go build -o bin/server ./cmd/server

test:
	go test ./...

test-scheduling-db:
	@test -n "$$TEST_DATABASE_URL" || { echo "TEST_DATABASE_URL is required"; exit 1; }
	go test -p 1 ./internal/scheduling ./internal/httpapi/sessionshttp ./internal/httpapi/serieshttp ./internal/httpapi/availabilityhttp ./internal/httpapi/schedulinghttp ./internal/legacysync ./internal/db -run '^TestScheduleDB_' -count=1 -timeout=120s

fmt:
	gofmt -w .

lint:
	golangci-lint run ./...

tools:
	go install github.com/pressly/goose/v3/cmd/goose@v3.27.1
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.5

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

migrate-validate:
	bash ../scripts/validate-migrations.sh

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate
