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

# Run the linters and apply automatic fixes.
.PHONY: lint
lint:
	@golangci-lint run --fix --timeout 5m

# Run the linters without modifying anything.
.PHONY: check-lint
check-lint:
	@golangci-lint run --timeout 5m
