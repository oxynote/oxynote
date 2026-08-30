# Minimum total statement coverage. Set from the measured baseline and
# ratcheted up as suites grow — never lowered.
COVERAGE_MIN = 93.1

# the tag the snapshot image carries; goreleaser reads it as the snapshot
# version. The e2e stack passes e2e-dev so its run cannot clobber the image
# the dev stack is using.
IMAGE_TAG ?= dev

# Build the Docker image
# dockers_v2 snapshot builds load into the local daemon with an arch suffix;
# retag so the compose stacks can reference a stable tag.
.PHONY: build
build:
	CORE_SNAPSHOT_TAG=$(IMAGE_TAG) goreleaser release --snapshot --clean
	docker tag ghcr.io/oxynote/core:$(IMAGE_TAG)-amd64 ghcr.io/oxynote/core:$(IMAGE_TAG)

# The binary on its own, for the all-in-one image: its Dockerfile stages the
# binary and builds everything else itself, so core's own docker image is
# waste on this path. `goreleaser build` compiles and stops there, where
# `release` above also runs the dockers_v2 block — which on a real tag would
# publish ghcr.io/oxynote/core alongside the image being released.
#
# The snapshot variant is for local and e2e builds, where there may be no
# tag and the version does not matter. A published image must use the other
# one: a snapshot stamps the binary as version dev in a dev environment.
.PHONY: build-binary
build-binary:
	goreleaser build --clean

.PHONY: build-binary-snapshot
build-binary-snapshot:
	goreleaser build --snapshot --clean

# Run all tests with the race detector.
.PHONY: test
test:
	@go test -race ./...

# Run all tests with coverage and fail below the minimum.
.PHONY: check-coverage
check-coverage:
	@go test -race -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | tail -n 1 | awk -v min=$(COVERAGE_MIN) \
		'{ print; if ($$3 + 0 < min) { printf "total coverage is below the %s%% minimum\n", min; exit 1 } }'

# Run the linters and apply automatic fixes.
.PHONY: lint
lint:
	@golangci-lint run --fix --timeout 5m

# Run the linters without modifying anything.
.PHONY: check-lint
check-lint:
	@golangci-lint run --timeout 5m
