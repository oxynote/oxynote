COMPOSE := docker-compose -p oxynote -f docker/docker-compose.dev.yaml
# extra compose files, so CI can layer its build cache on top without
# that config leaking into local builds (see e2e/docker-compose.ci.yaml)
E2E_COMPOSE_EXTRA ?=
E2E_COMPOSE := docker compose -f e2e/docker-compose.yaml $(E2E_COMPOSE_EXTRA)

QUIET := scripts/run-quietly.sh

# backend containers + web dev server on the host with hot reload. Ctrl-c
# stops the web dev server; `make stop` stops the containers.
.PHONY: dev
dev: build-go sync-changedetection-key
	$(COMPOSE) up --build -d
	cd web && NUXT_PUBLIC_APP_BASE_URL=http://localhost:3000 pnpm run start:dev:web

# foreground: streams all logs, ctrl-c stops the stack
.PHONY: run
run: build-go sync-changedetection-key
	$(COMPOSE) --profile web up --build

# background
.PHONY: start
start: build-go sync-changedetection-key
	$(COMPOSE) --profile web up --build -d

.PHONY: stop
stop:
	$(COMPOSE) --profile web down

# the e2e stack runs on its own ports with throwaway state, so it can be
# up at the same time as the dev stack.
#
# make only ever builds; starting the stack belongs to the suite's own
# setup hook, so there is exactly one thing that runs `compose up`.
# Building has to live here because the hook cannot do it: goreleaser
# produces core's image, which compose cannot build at all, and the two
# images compose does own are only refreshed by an explicit build.
# Iterate with `make e2e-stack-build` once, then `cd e2e && pnpm test`.
.PHONY: e2e
e2e: e2e-stack-build
	@( cd e2e && pnpm run test ); status=$$?; \
	$(MAKE) --no-print-directory e2e-stack-stop; \
	exit $$status

.PHONY: e2e-stack-build
e2e-stack-build:
	@$(QUIET) "building go images" $(MAKE) build-go
	@$(QUIET) "building stack images" $(E2E_COMPOSE) build

.PHONY: e2e-stack-stop
e2e-stack-stop:
	@$(QUIET) "stopping the stack" $(E2E_COMPOSE) down -v

# lint gates of every component that has one, from one place. Fixing and
# checking mirror each component's own vocabulary: web, auth-realtime and
# e2e pair lint/check-lint, and core wraps golangci-lint. Datagen has no
# lint setup and is deliberately absent.
.PHONY: lint
lint:
	@$(QUIET) "linting web" sh -c 'cd web && pnpm run lint'
	@$(QUIET) "linting auth-realtime" sh -c 'cd server/auth-realtime && pnpm run lint'
	@$(QUIET) "linting core" sh -c 'cd server/core && make lint'
	@$(QUIET) "linting e2e" sh -c 'cd e2e && pnpm run lint'

.PHONY: check-lint
check-lint:
	@$(QUIET) "checking web" sh -c 'cd web && pnpm run check-lint'
	@$(QUIET) "checking auth-realtime" sh -c 'cd server/auth-realtime && pnpm run check-lint'
	@$(QUIET) "checking core" sh -c 'cd server/core && make check-lint'
	@$(QUIET) "checking e2e" sh -c 'cd e2e && pnpm run check-lint'

.PHONY: setup
setup:
	cd web && pnpm run setup
	cd server/auth-realtime && pnpm install
	cd e2e && pnpm run setup
	cd server/core && go mod download
	cd datagen && go mod download
	for f in core auth-realtime web; do \
		test -f docker/env/$$f.local.env || cp docker/env/$$f.example.env docker/env/$$f.local.env; \
	done
	test -f web/.env || cp docker/env/web.example.env web/.env

.PHONY: deps
deps:
	cd web && pnpm run deps
	cd server/auth-realtime && pnpm install
	cd e2e && pnpm install
	cd server/core && go mod download
	cd datagen && go mod download

# goreleaser snapshot builds the go binaries + their :dev images
.PHONY: build-go
build-go:
	cd server/core && make build
	cd datagen && make build

# changedetection.io generates its API key on first boot; start it alone and
# copy the key into core.local.env before the rest of the stack (core reads
# its env only at container creation).
.PHONY: sync-changedetection-key
sync-changedetection-key:
	$(COMPOSE) up -d changedetection
	./docker/sync-changedetection-key.sh