.PHONY: all build clean test lint run-agent run-orch docker-up docker-down dev goreleaser

BINARY=bin/strata-rmm
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

all: build

build:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY) .

build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-amd64 .

build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-windows-amd64.exe .

build-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags="$(LDFLAGS)" -o $(BINARY)-linux-arm64 .

goreleaser:
	goreleaser release --clean

test:
	go test ./... -v -count=1

lint:
	go vet ./...

clean:
	rm -rf bin/ dist/

dev: docker-up
	docker compose -f deploy/docker/docker-compose.yml up -d nats postgres
	@echo "Waiting for services..."
	@sleep 5
	go run . orchestrator --nats-url nats://localhost:4222 --timescale-dsn "postgres://strata:strata_dev@localhost:5432/strata_rmm?sslmode=disable" --api-addr :8080

docker-up:
	docker compose -f deploy/docker/docker-compose.yml up -d

docker-down:
	docker compose -f deploy/docker/docker-compose.yml down

docker-build:
	docker compose -f deploy/docker/docker-compose.yml build

run-agent:
	go run . agent --tenant-id $$TENANT_ID --enrollment-token $$ENROLLMENT_TOKEN --nats-url nats://localhost:4222

run-orch:
	go run . orchestrator --nats-url nats://localhost:4222 --timescale-dsn "postgres://strata:strata_dev@localhost:5432/strata_rmm?sslmode=disable" --api-addr :8080

tidy:
	go mod tidy

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html