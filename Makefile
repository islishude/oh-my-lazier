SHELL := /bin/sh

.PHONY: all check compile typecheck test test-solidity test-scripts test-go security-check lint lint-go fmt fmt-go docker-build clean

all: check

check: compile typecheck test-solidity test-scripts test-go lint-go fmt-check

compile:
	npm run compile

typecheck:
	npm run typecheck

test: test-solidity test-go

test-solidity:
	npx hardhat test solidity

test-scripts:
	npm run test:scripts

test-go:
	go test ./...

security-check:
	@tmp="$$(mktemp)"; \
	npm audit --json > "$$tmp" || true; \
	node -e 'const fs = require("node:fs"); const report = JSON.parse(fs.readFileSync(process.argv[1], "utf8")); const count = report.metadata?.vulnerabilities?.critical ?? 0; if (count > 0) { console.error(`npm audit critical vulnerabilities: ${count}`); process.exit(1); } console.log("npm audit critical vulnerabilities: 0");' "$$tmp"; \
	rm -f "$$tmp"
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

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

clean:
	npx hardhat clean
