# Minimum total statement coverage. Set from the measured baseline and
# ratcheted up as suites grow — never lowered.
COVERAGE_MIN = 93.1

# the tag the snapshot image carries; goreleaser reads it as the snapshot
# version. The e2e stack passes e2e-dev so its run cannot clobber the image
# the dev stack is using.
IMAGE_TAG ?= dev

# the architecture of the Docker engine the image and binary are built for:
# on macOS and Windows that is its Linux VM's, not the host's.
DOCKER_ARCH ?= $(shell docker version --format '{{.Server.Arch}}' 2>/dev/null)

# Build the Docker image
# dockers_v2 snapshot builds load into the local daemon with an arch suffix;
# retag so the compose stacks can reference a stable tag.
.PHONY: build
build:
	CORE_SNAPSHOT_TAG=$(IMAGE_TAG) DOCKER_ARCH=$(DOCKER_ARCH) goreleaser release --snapshot --clean
	docker tag ghcr.io/oxynote/core:$(IMAGE_TAG)-$(DOCKER_ARCH) ghcr.io/oxynote/core:$(IMAGE_TAG)

# The binary on its own, for the all-in-one image: its Dockerfile stages the
# binary and builds everything else itself, so core's own docker image is
# waste on this path. `goreleaser build` compiles and stops there, where
# `release` above also runs the dockers_v2 block — which on a real tag would
# publish ghcr.io/oxynote/core alongside the image being released.
#
# The snapshot variant is for local and e2e builds, where there may be no
# tag.
#
# Both build DOCKER_ARCH alone and copy the binary to OUTPUT, so a caller
# never depends on goreleaser's dist layout.
.PHONY: build-binary
build-binary:
	GOOS=linux GOARCH=$(DOCKER_ARCH) goreleaser build --clean --single-target -o $(OUTPUT)

.PHONY: build-binary-snapshot
build-binary-snapshot:
	GOOS=linux GOARCH=$(DOCKER_ARCH) goreleaser build --snapshot --clean --single-target -o $(OUTPUT)

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

# Rewrite internal/db/schema.sql from the migrations. Test_Schema fails
# until it matches.
.PHONY: schema
schema:
	@UPDATE_GOLDEN=1 go test ./internal/db -run '^Test_Schema$$' -count=1

# Run the linters and apply automatic fixes.
.PHONY: lint
lint:
	@golangci-lint run --fix --timeout 5m

# Run the linters without modifying anything.
.PHONY: check-lint
check-lint:
	@golangci-lint run --timeout 5m
