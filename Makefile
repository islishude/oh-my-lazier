SHELL := /bin/sh

INTEGRATION_COMPOSE = docker compose -p oh-my-lazier-integration -f docker-compose.integration.yml
INTEGRATION_POSTGRES_URL = postgres://laz_worker:laz_worker@localhost:55432/laz_worker?sslmode=disable
INTEGRATION_RUSTACK_ENDPOINT = http://localhost:4566
E2E_COMPOSE = docker compose -p oh-my-lazier-e2e -f docker-compose.e2e.yml
E2E_TMP_DIR = tmp/e2e
E2E_DEPLOYER_PRIVATE_KEY ?= 0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80
E2E_WORKER_PRIVATE_KEY ?= 0x59c6995e998f97a5a0044966f0945389dc9e86dae88c7a8412f4603b6b78690d
E2E_KEYSTORE_PASSWORD ?= local-e2e-password
E2E_WORKER_UP_FLAGS ?= --build

.PHONY: all check compile typecheck generate-lzabi check-lzabi generate-pricing-abi check-pricing-abi test test-solidity test-scripts test-go test-kms-rustack integration-up integration-down test-integration e2e-up e2e-down e2e-local security-check runbook-check migration-evidence-check lint lint-go fmt fmt-go docker-build docker-smoke clean

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
	$(INTEGRATION_COMPOSE) up -d --wait

integration-down:
	-$(INTEGRATION_COMPOSE) down -v --remove-orphans

test-integration:
	@set -e; \
	cleanup() { \
		$(INTEGRATION_COMPOSE) down -v --remove-orphans; \
	}; \
	trap cleanup EXIT INT TERM; \
	$(INTEGRATION_COMPOSE) up -d --wait; \
	TEST_POSTGRES_URL="$(INTEGRATION_POSTGRES_URL)" go test ./go/internal/db ./go/internal/txmgr -count=1; \
	RUSTACK_KMS_ENDPOINT="$(INTEGRATION_RUSTACK_ENDPOINT)" go test ./go/internal/signer/kms -run TestRustackKMSIntegrationSignsEthereumTransaction -count=1

e2e-up:
	$(E2E_COMPOSE) up -d --wait postgres anvil-a anvil-b

e2e-down:
	-$(E2E_COMPOSE) --profile worker down -v --remove-orphans

e2e-local:
	rm -rf $(E2E_TMP_DIR)
	mkdir -p $(E2E_TMP_DIR)
	@set -e; \
	cleanup() { \
		$(E2E_COMPOSE) --profile worker down -v --remove-orphans; \
		rm -rf $(E2E_TMP_DIR); \
	}; \
	trap cleanup EXIT INT TERM; \
	npm run compile; \
	$(E2E_COMPOSE) up -d --wait postgres anvil-a anvil-b; \
	E2E_TMP_DIR="$(E2E_TMP_DIR)" E2E_DEPLOYER_PRIVATE_KEY="$(E2E_DEPLOYER_PRIVATE_KEY)" E2E_WORKER_PRIVATE_KEY="$(E2E_WORKER_PRIVATE_KEY)" npm run e2e:deploy-local; \
	E2E_WORKER_PRIVATE_KEY="$(E2E_WORKER_PRIVATE_KEY)" E2E_KEYSTORE_PASSWORD="$(E2E_KEYSTORE_PASSWORD)" go run ./go/cmd/e2ekeystore -out "$(E2E_TMP_DIR)/worker-keystore.json"; \
	E2E_KEYSTORE_PASSWORD="$(E2E_KEYSTORE_PASSWORD)" go run ./go/cmd/configcheck -config "$(E2E_TMP_DIR)/worker.host.yaml"; \
	$(E2E_COMPOSE) --profile worker up -d $(E2E_WORKER_UP_FLAGS) --wait worker; \
	E2E_TMP_DIR="$(E2E_TMP_DIR)" E2E_DEPLOYER_PRIVATE_KEY="$(E2E_DEPLOYER_PRIVATE_KEY)" npm run e2e:run-local

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
