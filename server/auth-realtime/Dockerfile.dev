# inspired by: https://hono.dev/docs/getting-started/nodejs#dockerfile
FROM node:22-alpine AS base

FROM base AS builder

RUN apk add --no-cache gcompat
RUN corepack enable
WORKDIR /server

# pnpm-workspace.yaml carries the allowBuilds settings — without it the
# install refuses to run the esbuild install script.
COPY package.json pnpm-lock.yaml pnpm-workspace.yaml tsconfig.json src ./

RUN pnpm install --frozen-lockfile && \
    pnpm run build && \
    pnpm prune --prod

FROM base AS runner
WORKDIR /server

RUN addgroup --system --gid 1001 nodejs
RUN adduser --system --uid 1001 hono

COPY --from=builder --chown=hono:nodejs /server/node_modules /server/node_modules
COPY --from=builder --chown=hono:nodejs /server/dist /server/dist
COPY --from=builder --chown=hono:nodejs /server/package.json /server/package.json

ENV NODE_ENV=production
USER hono
EXPOSE 8081

CMD ["node", "--import", "/server/dist/instrument.js", "/server/dist/index.js"]
