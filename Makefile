GOCACHE ?= /tmp/runnarr-go-cache

.PHONY: all check test test-race vet fmt-check web-test web-build e2e visual-review visual-review-check testbed deployment-check

all: check

check: fmt-check vet test web-test web-build visual-review-check deployment-check

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

visual-review:
	bash scripts/visual-review.sh

visual-review-check:
	node --test scripts/visual-review-profiles.test.mjs
	bash scripts/visual-review-catalog-check.sh

testbed:
	cd web && npm run testbed

deployment-check:
	bash deploy/test.sh
