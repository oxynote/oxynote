# Build the Docker image
# dockers_v2 snapshot builds load into the local daemon with an arch suffix;
# retag so the dev compose stack can reference a stable :dev tag.
build:
	goreleaser release --snapshot --clean
	docker tag ghcr.io/oxynote/datagen:dev-amd64 ghcr.io/oxynote/datagen:dev
