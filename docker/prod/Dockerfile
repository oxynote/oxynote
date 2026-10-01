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
#
# the image is published for linux/amd64 and linux/arm64. Every stage but
# the last runs on the build machine's own architecture ($BUILDPLATFORM)
# whatever the target: their output is JavaScript, or binaries downloaded
# for the target, so a multi-arch build produces it once instead of again
# under emulation. That holds only while nothing a builder executes lands
# in its output.

# mirrors web/Dockerfile.dev (the dev/e2e image); change them together.
FROM --platform=$BUILDPLATFORM node:24.20.0-bookworm-slim@sha256:ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e AS web-builder

RUN npm install -g pnpm@11.1.3

WORKDIR /app

# the vite client build exceeds node's default heap.
ENV NODE_OPTIONS="--max-old-space-size=4096"

# dependency layer that survives ordinary source edits. Scripts are skipped
# because the root prepare builds the promql grammar and runs nuxt prepare,
# and neither has its sources yet.
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
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
FROM --platform=$BUILDPLATFORM node:24.20.0-bookworm-slim@sha256:ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e AS auth-realtime-builder

RUN npm install -g pnpm@11.1.3

WORKDIR /server

COPY server/auth-realtime/package.json server/auth-realtime/pnpm-lock.yaml server/auth-realtime/pnpm-workspace.yaml server/auth-realtime/tsconfig.json server/auth-realtime/tsconfig.build.json ./
COPY server/auth-realtime/src ./src

# check-types gates (esbuild does not look at types); the import proves
# the bundle loads — with no environment set it must reject with its own
# validation error, so an unresolvable import fails the build here rather
# than when the container starts.
RUN pnpm install --frozen-lockfile && pnpm run check-types && pnpm run build:bundle \
	&& node -e "import('/server/dist/bundle.mjs').then(() => { console.error('bundle skipped env validation'); process.exit(1) }, (err) => { if (!String(err).includes('invalid environment')) { console.error(err); process.exit(1) } })"

FROM --platform=$BUILDPLATFORM node:24.20.0-bookworm-slim@sha256:ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e AS launcher-builder

RUN npm install -g pnpm@11.1.3

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

# caddy and tini at pinned versions, checksum-verified before use. Both
# licenses ship in the final image: caddy's Apache-2.0 rides in its release
# tarball, tini's MIT is fetched from the same tag as its binary. The name
# "Caddy" is trademarked; the binary is bundled unmodified and unrebranded.
# Each architecture pins its own checksums; a version bump updates them all.
FROM --platform=$BUILDPLATFORM node:24.20.0-bookworm-slim@sha256:ba849c60be29959425b8734d57b8b4b7d56f98edd9504c9af091d5281095a71e AS fetch

ARG CADDY_VERSION=2.11.4
ARG CADDY_SHA512_AMD64=8220d1f013b6f27510247b2360c9e0ca9f018feebd82515f07635318b34ff9777ccc8fd0b6e6f2486ce3a33fe389fbb7db12d05baa474f4587509fb4f5ebf1c9
ARG CADDY_SHA512_ARM64=d5a7c423853c24a799765e0e8210d5c7c22a8f56ed37a3cae2fb9f58be138853c02b4efd6b59d576e6d8c7c0d30b9c1592deeaa6a536ff69bcca23b8c1ea709c
ARG TINI_VERSION=0.19.0
ARG TINI_SHA256_AMD64=c5b0666b4cb676901f90dfcb37106783c5fe2077b04590973b885950611b30ee
ARG TINI_SHA256_ARM64=eae1d3aa50c48fb23b8cbdf4e369d0910dfc538566bfd09df89a774aa84a48b9
ARG TARGETARCH

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/*

WORKDIR /fetch

RUN case "${TARGETARCH}" in \
		amd64) caddy_sha512="${CADDY_SHA512_AMD64}" tini_sha256="${TINI_SHA256_AMD64}" ;; \
		arm64) caddy_sha512="${CADDY_SHA512_ARM64}" tini_sha256="${TINI_SHA256_ARM64}" ;; \
		*) echo "no pinned checksums for ${TARGETARCH}" >&2; exit 1 ;; \
	esac \
	&& curl -fsSLo caddy.tar.gz "https://github.com/caddyserver/caddy/releases/download/v${CADDY_VERSION}/caddy_${CADDY_VERSION}_linux_${TARGETARCH}.tar.gz" \
	&& echo "${caddy_sha512}  caddy.tar.gz" | sha512sum -c - \
	&& tar -xzf caddy.tar.gz caddy LICENSE \
	&& mv LICENSE caddy-LICENSE \
	&& curl -fsSLo tini "https://github.com/krallin/tini/releases/download/v${TINI_VERSION}/tini-static-${TARGETARCH}" \
	&& echo "${tini_sha256}  tini" | sha256sum -c - \
	&& curl -fsSLo tini-LICENSE "https://raw.githubusercontent.com/krallin/tini/v${TINI_VERSION}/LICENSE" \
	&& chmod 0755 caddy tini

# the runtime node is alpine's own build — v24 linked against shared
# system libraries, roughly half the size of the official node binary.
# The base is digest-pinned; nodejs then tracks alpine 3.24's package
# repo, which only ships security bumps within the branch.
FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6

# the exact pins make the build fail — instead of silently shifting —
# when alpine's branch rotates a package (they keep only the newest build
# per branch); bump the ARGs together with the base digest, which a
# dependabot bump of the digest alone cannot do. icu-data-full replaces the
# English-only default so Intl formats every locale during SSR, and
# postgresql18 is the embedded database, with pg_dump among its tools. Its
# major is part of the package name and of the launcher's
# /usr/libexec/postgresql18 paths: a data directory only opens under the
# major that wrote it, so a major bump needs an upgrade path for existing
# volumes, not only a new pin.
ARG NODE_APK_VERSION=24.18.1-r0
ARG ICU_DATA_APK_VERSION=78.1-r0
ARG POSTGRESQL_APK_VERSION=18.6-r0

# the uid is explicit and never changes: the data volume's files are owned
# by it, so a shift between releases would lock an existing deployment out
# of its own secrets. The three directories are the volume mount point
# (which the launcher writes generated secrets into), the mount point for
# a GitHub App private key, and caddy's scratch dir.
RUN apk add --no-cache "nodejs=${NODE_APK_VERSION}" "icu-data-full=${ICU_DATA_APK_VERSION}" "postgresql18=${POSTGRESQL_APK_VERSION}" \
	&& adduser -D -H -u 65532 oxynote \
	&& mkdir -p /oxynote/data /oxynote/github /tmp/caddy \
	&& chown -R oxynote /oxynote /tmp/caddy

COPY --from=fetch /fetch/tini /tini
COPY --from=fetch /fetch/caddy /usr/local/bin/caddy
COPY --from=fetch /fetch/caddy-LICENSE /oxynote/licenses/caddy/LICENSE
COPY --from=fetch /fetch/tini-LICENSE /oxynote/licenses/tini/LICENSE

# staged per architecture by `make prod-build` and `make prod-publish` from
# goreleaser's output.
ARG TARGETARCH
COPY docker/prod/.build/${TARGETARCH}/oxynote-core /oxynote/core/server

COPY --from=web-builder /app/.output /oxynote/web
COPY --from=auth-realtime-builder /server/dist/bundle.mjs /oxynote/auth-realtime/index.mjs
COPY --from=launcher-builder /launcher/dist/launcher.mjs /oxynote/prod/launcher.mjs
COPY docker/prod/healthcheck.mjs /oxynote/prod/healthcheck.mjs
COPY docker/prod/Caddyfile /oxynote/prod/Caddyfile
COPY docker/prod/pg_hba.conf /oxynote/prod/pg_hba.conf

# what ghcr reads to link the package to this repository, and what a
# scanner reads to tell one build from another. IMAGE_VERSION and
# IMAGE_REVISION come from `make prod-publish`; a local build leaves them
# at dev/unknown.
ARG IMAGE_VERSION=dev
ARG IMAGE_REVISION=unknown
LABEL org.opencontainers.image.title="Oxynote" \
	org.opencontainers.image.description="Collaborative documentation, in one container" \
	org.opencontainers.image.source="https://github.com/oxynote/oxynote" \
	org.opencontainers.image.url="https://github.com/oxynote/oxynote" \
	org.opencontainers.image.documentation="https://github.com/oxynote/oxynote/blob/main/docker/prod/README.md" \
	org.opencontainers.image.licenses="Apache-2.0" \
	org.opencontainers.image.vendor="Oxynote" \
	org.opencontainers.image.version="${IMAGE_VERSION}" \
	org.opencontainers.image.revision="${IMAGE_REVISION}"

ENV NODE_ENV=production
ENV HOME=/oxynote

USER oxynote

# generated secrets persist here even when no volume is mounted explicitly.
VOLUME /oxynote/data

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --retries=5 --start-period=180s \
	CMD ["/usr/bin/node", "/oxynote/prod/healthcheck.mjs"]

ENTRYPOINT ["/tini", "--", "/usr/bin/node", "/oxynote/prod/launcher.mjs"]
