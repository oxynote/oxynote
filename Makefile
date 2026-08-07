COMPOSE := docker-compose -p oxynote -f docker/docker-compose.dev.yaml

# goreleaser snapshot builds the go binaries + their :dev images
build-go:
	cd server/core && make build
	cd datagen && make build

# changedetection.io generates its API key on first boot; start it alone and
# copy the key into core.local.env before the rest of the stack (core reads
# its env only at container creation).
sync-changedetection-key:
	$(COMPOSE) up -d changedetection
	./docker/sync-changedetection-key.sh

# foreground: streams all logs, ctrl-c stops the stack
run: build-go sync-changedetection-key
	$(COMPOSE) --profile web up --build

# background
start: build-go sync-changedetection-key
	$(COMPOSE) --profile web up --build -d

# backend containers + web dev server on the host with hot reload. Ctrl-c
# stops the web dev server; `make stop` stops the containers.
dev: build-go sync-changedetection-key
	$(COMPOSE) up --build -d
	cd web && NUXT_PUBLIC_APP_BASE_URL=http://localhost:3000 pnpm run start:dev:web

stop:
	$(COMPOSE) --profile web down

setup:
	cd web && pnpm run setup
	cd server/auth-realtime && pnpm install
	cd server/core && go mod download
	cd datagen && go mod download
	for f in core auth-realtime web; do \
		test -f docker/env/$$f.local.env || cp docker/env/$$f.example.env docker/env/$$f.local.env; \
	done
	test -f web/.env || cp docker/env/web.example.env web/.env

deps:
	cd web && pnpm run deps
	cd server/auth-realtime && pnpm install
	cd server/core && go mod download
	cd datagen && go mod download
