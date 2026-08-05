COMPOSE := docker-compose -p oxynote -f docker/docker-compose.dev.yaml

# goreleaser snapshot builds the go binaries + their :dev images
build-go:
	cd server/core && make build
	cd datagen && make build

# foreground: streams all logs, ctrl-c stops the stack
run: build-go
	$(COMPOSE) up --build

# background
start: build-go
	$(COMPOSE) up --build -d

stop:
	$(COMPOSE) down

setup:
	cd web && pnpm run setup
	cd server/auth-realtime && pnpm install
	cd server/core && go mod download
	cd datagen && go mod download
	for f in core auth-realtime web; do \
		test -f docker/env/$$f.local.env || cp docker/env/$$f.example.env docker/env/$$f.local.env; \
	done

deps:
	cd web && pnpm run deps
	cd server/auth-realtime && pnpm install
	cd server/core && go mod download
	cd datagen && go mod download
