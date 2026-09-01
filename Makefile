# where CI layers its build cache, kept out of the definitions below so
# that config never leaks into a local build. The compose one is an extra
# file (e2e/docker-compose.dev.ci.yaml); the prod one is docker build
# flags, because compose has no part in that image.
E2E_DEV_COMPOSE_EXTRA ?=
PROD_BUILD_EXTRA ?=

# one compose invocation per stack. Only the dev stack is a project name
# away from the others; the e2e stacks are named by their own files.
COMPOSE := docker-compose -p oxynote -f docker/docker-compose.dev.yaml
E2E_DEV_COMPOSE := docker compose -f e2e/docker-compose.dev.yaml $(E2E_DEV_COMPOSE_EXTRA)
E2E_PROD_COMPOSE := docker compose -f e2e/docker-compose.prod.yaml
PROD_COMPOSE := docker compose -f docker/prod/docker-compose.example.yaml \
	-f docker/prod/docker-compose.local.yaml

# a tag names the stack an image belongs to. The e2e targets override these
# to :e2e-dev and :e2e-prod so a run cannot replace the image `make start`
# is serving from.
CORE_IMAGE_TAG ?= dev
PROD_IMAGE_TAG ?= prod

# the version a published image is tagged with, without a leading v. Set
# only by the release workflow; prod-publish refuses to run without it.
RELEASE_VERSION ?=

QUIET := scripts/run-quietly.sh

# backend containers + web dev server on the host with hot reload. Ctrl-c
# stops the web dev server; `make stop` stops the containers.
.PHONY: dev
dev: check-env build-go sync-changedetection-key
	$(COMPOSE) up --build -d
	cd web && NUXT_PUBLIC_APP_BASE_URL=http://localhost:3000 pnpm run start:dev:web

# foreground: streams all logs, ctrl-c stops the stack
.PHONY: run
run: check-env build-go sync-changedetection-key
	$(COMPOSE) --profile web up --build

# background
.PHONY: start
start: check-env build-go sync-changedetection-key
	$(COMPOSE) --profile web up --build -d

.PHONY: stop
stop:
	$(COMPOSE) --profile web down

# the e2e stack runs on its own ports with throwaway state, so it can be
# up at the same time as the dev stack.
#
# the playwright config owns the whole cycle: globalSetup builds the stack
# and brings it up, globalTeardown stops it and drops its volumes. That is
# what makes the play button in the Playwright VS Code extension do exactly
# what this target does, so this target only starts the run. The build
# stays a make target because the hook cannot express it — goreleaser
# produces core's image, which compose cannot build at all — and the hook
# shells back out to e2e-dev-stack-build, which is also what keeps
# E2E_DEV_COMPOSE_EXTRA working from CI.
#
# Every run is therefore a full build and teardown. To iterate without
# paying that, run e2e-dev-stack-build once and drive the tests directly.
.PHONY: e2e-dev
e2e-dev:
	@( cd e2e && pnpm run test:dev )

.PHONY: e2e-dev-stack-build
e2e-dev-stack-build:
	@$(QUIET) "building go images" $(MAKE) build-go CORE_IMAGE_TAG=e2e-dev
	@$(QUIET) "building stack images" $(E2E_DEV_COMPOSE) build

.PHONY: e2e-dev-stack-stop
e2e-dev-stack-stop:
	@$(QUIET) "stopping the stack" $(E2E_DEV_COMPOSE) down -v

# the same suite against the production all-in-one image, plus the
# trust-boundary tests that only mean something there. It publishes its own
# ports, so it coexists with both the dev stack and the e2e stack.
.PHONY: e2e-prod
e2e-prod:
	@( cd e2e && pnpm run test:prod )

# only the all-in-one image is built here; the backing services are pinned
# third-party images that `up` pulls on its own. Pulling them here would
# also try to pull the image just built, which has no registry to come from.
.PHONY: e2e-prod-stack-build
e2e-prod-stack-build:
	@$(QUIET) "building the prod image" $(MAKE) prod-build PROD_IMAGE_TAG=e2e-prod

.PHONY: e2e-prod-stack-stop
e2e-prod-stack-stop:
	@$(QUIET) "stopping the stack" $(E2E_PROD_COMPOSE) down -v

# lint gates of every component, from one place. Fixing and checking mirror
# each component's own vocabulary: web, auth-realtime and e2e pair
# lint/check-lint, and core and datagen wrap golangci-lint.
.PHONY: lint
lint:
	@$(QUIET) "linting web" sh -c 'cd web && pnpm run lint'
	@$(QUIET) "linting auth-realtime" sh -c 'cd server/auth-realtime && pnpm run lint'
	@$(QUIET) "linting core" sh -c 'cd server/core && make lint'
	@$(QUIET) "linting datagen" sh -c 'cd datagen && make lint'
	@$(QUIET) "linting e2e" sh -c 'cd e2e && pnpm run lint'
	@$(QUIET) "linting launcher" sh -c 'cd docker/prod/launcher && pnpm run lint'

.PHONY: check-lint
check-lint:
	@$(QUIET) "checking web" sh -c 'cd web && pnpm run check-lint'
	@$(QUIET) "checking auth-realtime" sh -c 'cd server/auth-realtime && pnpm run check-lint'
	@$(QUIET) "checking core" sh -c 'cd server/core && make check-lint'
	@$(QUIET) "checking datagen" sh -c 'cd datagen && make check-lint'
	@$(QUIET) "checking e2e" sh -c 'cd e2e && pnpm run check-lint'
	@$(QUIET) "checking launcher" sh -c 'cd docker/prod/launcher && pnpm run check-lint'

.PHONY: setup
setup:
	cd web && pnpm run setup
	cd server/auth-realtime && pnpm install
	cd docker/prod/launcher && pnpm install
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
	cd docker/prod/launcher && pnpm install
	cd e2e && pnpm install
	cd server/core && go mod download
	cd datagen && go mod download

# goreleaser snapshot builds the go binaries + their images
.PHONY: build-go
build-go:
	cd server/core && make build IMAGE_TAG=$(CORE_IMAGE_TAG)
	cd datagen && make build

# the all-in-one image. Core must come from goreleaser, so the binary is
# built first and staged where the Dockerfile's COPY expects it — a bare
# `docker build` is not supported.
.PHONY: prod-build
prod-build:
	@$(QUIET) "building the core binary" sh -c 'cd server/core && make build-binary-snapshot'
	mkdir -p docker/prod/.build
	cp server/core/bin/oxynote-core docker/prod/.build/oxynote-core
	docker build --platform linux/amd64 $(PROD_BUILD_EXTRA) -f docker/prod/Dockerfile -t ghcr.io/oxynote/oxynote:$(PROD_IMAGE_TAG) .

# the published image: the same Dockerfile as prod-build, but core's binary
# is built from the tag instead of as a snapshot — a snapshot would report
# itself as version dev in a dev environment — and the result is pushed as
# latest and the version rather than loaded locally. These are the only two
# tags the registry gets. The version and commit also become OCI labels,
# which is what links the package to this repository on ghcr.
.PHONY: prod-publish
prod-publish:
	@test -n "$(RELEASE_VERSION)" || { echo "RELEASE_VERSION is required, e.g. 1.2.3"; exit 1; }
	@$(QUIET) "building the core binary" sh -c 'cd server/core && make build-binary'
	mkdir -p docker/prod/.build
	cp server/core/bin/oxynote-core docker/prod/.build/oxynote-core
	docker buildx build --platform linux/amd64 $(PROD_BUILD_EXTRA) \
		-f docker/prod/Dockerfile \
		--build-arg IMAGE_VERSION=$(RELEASE_VERSION) \
		--build-arg IMAGE_REVISION=$$(git rev-parse HEAD) \
		-t ghcr.io/oxynote/oxynote:latest \
		-t ghcr.io/oxynote/oxynote:$(RELEASE_VERSION) \
		--push .

# build the image, then run the example deployment against it on
# http://localhost:8080 (ctrl-c stops it). The local override adds the mail
# sink the example only describes in a comment: signup requires a verified
# address, so without it the verification link is only logged. Read the
# delivered mail at http://localhost:8025.
.PHONY: prod-run
prod-run: prod-build
	OXYNOTE_IMAGE_TAG=$(PROD_IMAGE_TAG) $(PROD_COMPOSE) up

.PHONY: prod-stop
prod-stop:
	$(PROD_COMPOSE) down

# changedetection.io generates its API key on first boot; start it alone and
# copy the key into core.local.env before the rest of the stack (core reads
# its env only at container creation).
.PHONY: sync-changedetection-key
sync-changedetection-key:
	$(COMPOSE) up -d changedetection
	./docker/sync-changedetection-key.sh

# a variable renamed in a template never reaches an env file that already
# exists, so the stack starts against a name nobody reads. check-env fails
# before that happens; sync-env rewrites the local files from the
# templates, keeping the value of every variable they still list.
.PHONY: check-env
check-env:
	./docker/sync-env.sh

.PHONY: sync-env
sync-env:
	./docker/sync-env.sh --fix