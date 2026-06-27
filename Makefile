SHELL := /bin/sh

.PHONY: all check compile test test-solidity test-scripts test-go lint lint-go fmt fmt-go docker-build clean

all: check

check: compile test-solidity test-scripts test-go lint-go fmt-check

compile:
	npm run compile

test: test-solidity test-go

test-solidity:
	npx hardhat test solidity

test-scripts:
	npm run test:scripts

test-go:
	go test ./...

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
