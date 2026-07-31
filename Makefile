GOCACHE ?= /tmp/runnarr-go-cache

.PHONY: all check test test-race vet fmt-check web-test web-build e2e testbed deployment-check

all: check

check: fmt-check vet test web-test web-build deployment-check

test:
	GOCACHE="$(GOCACHE)" go test ./...

test-race:
	GOCACHE="$(GOCACHE)" go test -race ./...

vet:
	GOCACHE="$(GOCACHE)" go vet ./...

fmt-check:
	test -z "$$(gofmt -l cmd internal)"

web-test:
	cd web && npm test

web-build:
	cd web && npm run build

e2e:
	cd web && npm run e2e

testbed:
	cd web && npm run testbed

deployment-check:
	bash deploy/test.sh
