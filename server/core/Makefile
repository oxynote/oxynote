# Minimum total statement coverage. Set from the measured baseline and
# ratcheted up as suites grow — never lowered.
COVERAGE_MIN = 93.1

# Build the Docker image
# dockers_v2 snapshot builds load into the local daemon with an arch suffix;
# retag so the dev compose stack can reference a stable :dev tag.
.PHONY: build
build:
	goreleaser release --snapshot --clean
	docker tag ghcr.io/oxynote/core:dev-amd64 ghcr.io/oxynote/core:dev

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
