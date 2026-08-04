setup:
	cd web && pnpm run setup
	cd server/auth-realtime && npm install

# go modules (server/core, datagen) vendor their deps — no install step needed
deps:
	cd web && pnpm run deps
	cd server/auth-realtime && npm install
