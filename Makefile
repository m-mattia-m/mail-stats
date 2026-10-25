.PHONY: build test cover cover-html image clean

# Packages that count towards coverage - mirrors ci.yaml.
COVER_PKGS = $(shell go list ./... | grep -Ev '/(cmd|imaptest)')

build:
	go build -o bin/mail-stats ./cmd/mail-stats

# Quick local test run, no coverage.
test:
	go test ./...

# Same command and threshold base as CI's 80% gate.
cover:
	go test -race -covermode=atomic -coverprofile=coverage.out $(COVER_PKGS)
	@echo "-------------------------------"
	@go tool cover -func=coverage.out | tail -1
	@echo "-------------------------------"

cover-html: cover
	go tool cover -html=coverage.out

image:
	podman build -f .container/Containerfile --build-arg VERSION=dev -t mail-stats:dev .

clean:
	rm -rf bin coverage.out
