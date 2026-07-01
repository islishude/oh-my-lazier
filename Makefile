SHELL := /bin/sh

INTEGRATION_COMPOSE = docker compose -p oh-my-lazier-integration -f docker-compose.integration.yml
INTEGRATION_TMP_DIR = tmp/integration
INTEGRATION_POSTGRES_URL = postgres://laz_worker:laz_worker@localhost:55432/laz_worker?sslmode=disable
INTEGRATION_RUSTACK_ENDPOINT = http://localhost:4566

.PHONY: all check compile typecheck generate-lzabi check-lzabi generate-pricing-abi check-pricing-abi test test-solidity test-scripts test-go test-kms-rustack integration-up integration-down test-integration security-check runbook-check migration-evidence-check lint lint-go fmt fmt-go docker-build docker-smoke clean

all: check

ci: check

check: compile typecheck check-lzabi check-pricing-abi test-solidity test-scripts test-go runbook-check migration-evidence-check lint-go fmt-check

compile:
	npm run compile

typecheck:
	npm run typecheck

generate-lzabi:
	npm run generate:lzabi

check-lzabi:
	npm run check:lzabi

generate-pricing-abi:
	npm run generate:pricing-abi

check-pricing-abi:
	npm run check:pricing-abi

test: test-solidity test-go

test-solidity:
	npx hardhat test solidity

test-scripts:
	npm run test:scripts

test-go:
	go test ./...

test-kms-rustack:
	@if [ -z "$$RUSTACK_KMS_ENDPOINT" ]; then \
		echo "RUSTACK_KMS_ENDPOINT is required"; \
		exit 1; \
	fi
	go test ./go/internal/signer/kms -run TestRustackKMSIntegrationSignsEthereumTransaction -count=1

integration-up:
	mkdir -p $(INTEGRATION_TMP_DIR)/postgres
	$(INTEGRATION_COMPOSE) up -d --wait

integration-down:
	-$(INTEGRATION_COMPOSE) down -v --remove-orphans
	rm -rf $(INTEGRATION_TMP_DIR)

test-integration:
	mkdir -p $(INTEGRATION_TMP_DIR)/postgres
	@set -e; \
	cleanup() { \
		$(INTEGRATION_COMPOSE) down -v --remove-orphans; \
		rm -rf $(INTEGRATION_TMP_DIR); \
	}; \
	trap cleanup EXIT INT TERM; \
	$(INTEGRATION_COMPOSE) up -d --wait; \
	TEST_POSTGRES_URL="$(INTEGRATION_POSTGRES_URL)" go test ./go/internal/db ./go/internal/txmgr -count=1; \
	RUSTACK_KMS_ENDPOINT="$(INTEGRATION_RUSTACK_ENDPOINT)" go test ./go/internal/signer/kms -run TestRustackKMSIntegrationSignsEthereumTransaction -count=1

security-check:
	npm run check:security-review
	npm run check:npm-audit-disposition
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

runbook-check:
	npm run check:runbooks

migration-evidence-check:
	MIGRATION_EVIDENCE=docs/deployments/testnet-migration-evidence.example.json npm run check:migration-evidence

lint: lint-go

lint-go:
	golangci-lint run ./...

fmt: fmt-go fmt-sol

fmt-check: fmt-go-check fmt-sol-check

fmt-go:
	gofmt -w go

fmt-go-check:
	gofmt -l go

fmt-sol:
	forge fmt contracts

fmt-sol-check:
	forge fmt --check contracts

docker-build:
	docker build -t oh-my-lazier-worker:local .

docker-smoke: docker-build
	docker run --rm oh-my-lazier-worker:local -h

clean:
	npx hardhat clean
