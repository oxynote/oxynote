# the production all-in-one image: caddy + web (nuxt SSR) + core +
# auth-realtime, supervised by the launcher under tini. Build it with
# `make prod-build` from the repository root — the core binary comes
# from goreleaser and is staged into docker/prod/.build/ first, so a
# bare `docker build` fails on that COPY by design.
#
# every FROM is pinned by digest, and the third-party binaries (caddy,
# tini) are downloaded at pinned versions and checksum-verified.
#
# the runtime ships no node_modules at all — auth-realtime and the
# launcher are single esbuild bundles and the web server is nitro's
# self-contained output — so the runtime libc (musl) cannot clash with
# anything; the web builder still guards against a native addon ever
# sneaking into the output.

# mirrors web/Dockerfile.dev (the dev/e2e image); change them together.
FROM node:24.19.0-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS web-builder

RUN npm install -g pnpm

WORKDIR /app

# the vite client build exceeds node's default heap.
ENV NODE_OPTIONS="--max-old-space-size=4096"

# dependency layer that survives ordinary source edits. Scripts are skipped
# because the root prepare builds the promql grammar and runs nuxt prepare,
# and neither has its sources yet.
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
COPY web/packages/lezer-promql/package.json ./packages/lezer-promql/
RUN pnpm install --frozen-lockfile --ignore-scripts

COPY web/ .

# the second install only runs the previously skipped scripts. The find
# guard fails the build if a native addon ever lands in the output — the
# runtime is musl and a glibc .node file would crash at boot instead.
RUN pnpm install --frozen-lockfile \
	&& NITRO_PRESET=node-server NITRO_MINIFY=true pnpm run build:web \
	&& test -z "$(find .output -name '*.node' -print -quit)"

# the runtime ships the package's own single-file esbuild bundle (see
# build:bundle and src/bundle.ts — the same artifact Dockerfile.dev and
# the e2e stack run) instead of the ~83MB production node_modules tree.
FROM node:24.19.0-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS auth-realtime-builder

RUN npm install -g pnpm

WORKDIR /server

COPY server/auth-realtime/package.json server/auth-realtime/pnpm-lock.yaml server/auth-realtime/pnpm-workspace.yaml server/auth-realtime/tsconfig.json server/auth-realtime/tsconfig.build.json ./
COPY server/auth-realtime/src ./src

# check-types gates (esbuild does not look at types); the import proves
# the bundle loads — with no environment set it must reject with its own
# validation error, so an unresolvable import fails the build here rather
# than when the container starts.
RUN pnpm install --frozen-lockfile && pnpm run check-types && pnpm run build:bundle \
	&& node -e "import('/server/dist/bundle.mjs').then(() => { console.error('bundle skipped env validation'); process.exit(1) }, (err) => { if (!String(err).includes('invalid environment')) { console.error(err); process.exit(1) } })"

FROM node:24.19.0-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS launcher-builder

RUN npm install -g pnpm

WORKDIR /launcher

COPY docker/prod/launcher/package.json docker/prod/launcher/pnpm-lock.yaml docker/prod/launcher/pnpm-workspace.yaml docker/prod/launcher/tsconfig.json ./
RUN pnpm install --frozen-lockfile

COPY docker/prod/launcher/src ./src

# the central telemetry DSNs, baked at build time only — empty for local
# builds (sentry disabled), set by the release pipeline. There is no
# runtime override; OXYNOTE_ERROR_REPORTING_DISABLED is the off switch.
# The args reach build:bundle as environment variables, where the
# package's own script turns them into esbuild defines.
ARG SENTRY_WEB_DSN=""
ARG SENTRY_CORE_DSN=""
ARG SENTRY_AUTH_REALTIME_DSN=""
ARG SENTRY_LAUNCHER_DSN=""

RUN pnpm run check-types && pnpm run build:bundle

# caddy and tini at pinned versions, checksum-verified before use. The
# caddy release tarball also carries the Apache-2.0 LICENSE the final image
# ships (the name "Caddy" is trademarked; the binary is bundled unmodified
# and unrebranded).
FROM node:24.19.0-bookworm-slim@sha256:a9f5f7c91a432850b2a8a7797adf5eadb6c733ceed61167806cee7ea7fbc29df AS fetch

ARG CADDY_VERSION=2.11.4
ARG CADDY_SHA512=8220d1f013b6f27510247b2360c9e0ca9f018feebd82515f07635318b34ff9777ccc8fd0b6e6f2486ce3a33fe389fbb7db12d05baa474f4587509fb4f5ebf1c9
ARG TINI_VERSION=0.19.0
ARG TINI_SHA256=c5b0666b4cb676901f90dfcb37106783c5fe2077b04590973b885950611b30ee

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*

WORKDIR /fetch

RUN curl -fsSLo caddy.tar.gz "https://github.com/caddyserver/caddy/releases/download/v${CADDY_VERSION}/caddy_${CADDY_VERSION}_linux_amd64.tar.gz" \
	&& echo "${CADDY_SHA512}  caddy.tar.gz" | sha512sum -c - \
	&& tar -xzf caddy.tar.gz caddy LICENSE \
	&& curl -fsSLo tini "https://github.com/krallin/tini/releases/download/v${TINI_VERSION}/tini-static-amd64" \
	&& echo "${TINI_SHA256}  tini" | sha256sum -c - \
	&& chmod 0755 caddy tini

# the runtime node is alpine's own build — v24 linked against shared
# system libraries, roughly half the size of the official node binary.
# The base is digest-pinned; nodejs then tracks alpine 3.23's package
# repo, which only ships security bumps within the branch.
FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40

# the exact pins make the build fail — instead of silently shifting —
# when alpine's v3.23 branch rotates a package (they keep only the
# newest build per branch); bump the ARGs together with the base digest.
# icu-data-full replaces the English-only default so Intl formats every
# locale during SSR.
ARG NODE_APK_VERSION=24.18.1-r0
ARG ICU_DATA_APK_VERSION=76.1-r1

# the uid is explicit and never changes: the data volume's files are owned
# by it, so a shift between releases would lock an existing deployment out
# of its own secrets. The three directories are the volume mount point
# (which the launcher writes generated secrets into), the mount point for
# a GitHub App private key, and caddy's scratch dir.
RUN apk add --no-cache "nodejs=${NODE_APK_VERSION}" "icu-data-full=${ICU_DATA_APK_VERSION}" \
	&& adduser -D -H -u 65532 oxynote \
	&& mkdir -p /oxynote/data /oxynote/github /tmp/caddy \
	&& chown -R oxynote /oxynote /tmp/caddy

COPY --from=fetch /fetch/tini /tini
COPY --from=fetch /fetch/caddy /usr/local/bin/caddy
COPY --from=fetch /fetch/LICENSE /oxynote/licenses/caddy/LICENSE

# staged by `make prod-build` from goreleaser's output.
COPY docker/prod/.build/oxynote-core /oxynote/core/server

COPY --from=web-builder /app/.output /oxynote/web
COPY --from=auth-realtime-builder /server/dist/bundle.mjs /oxynote/auth-realtime/index.mjs
COPY --from=launcher-builder /launcher/dist/launcher.mjs /oxynote/prod/launcher.mjs
COPY docker/prod/healthcheck.mjs /oxynote/prod/healthcheck.mjs
COPY docker/prod/Caddyfile /oxynote/prod/Caddyfile

ENV NODE_ENV=production
ENV HOME=/oxynote

USER oxynote

# generated secrets persist here even when no volume is mounted explicitly.
VOLUME /oxynote/data

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --retries=5 --start-period=180s \
	CMD ["/usr/bin/node", "/oxynote/prod/healthcheck.mjs"]

ENTRYPOINT ["/tini", "--", "/usr/bin/node", "/oxynote/prod/launcher.mjs"]
