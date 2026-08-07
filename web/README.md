# Web

Nuxt 4 + Vue 3 frontend of Oxynote. Ships two ways: a server-rendered web app
and an Electron desktop app.

**Tools and dependencies**:
- Favicon processing: https://realfavicongenerator.net
- [Nuxt documentation](https://nuxt.com/docs/getting-started/introduction)
- [Vue documentation](https://vuejs.org/guide/introduction.html)
- [VueUse documentation](https://vueuse.org/guide/)
- [Tiptap documentation](https://tiptap.dev/docs)
- [Shadcn-vue documentation](https://www.shadcn-vue.com/docs/introduction.html)

# I18N

All text needs to be placed in the `i18n/` directory.

We use a separate package for validation and it uses its own internal
error messages: https://vee-validate.logaretm.com/v4/guide/i18n/

## Setup

Install dependencies and run the prepare step:

```bash
pnpm run setup
```

Environment variables are read from `.env` — `make setup` at the repository
root creates it from `docker/env/web.example.env`.

## Development

Start the web development server on `http://localhost:3000` (the backend
stack must be running — `make dev` at the repository root does both):

```bash
pnpm run start:dev:web
```

Run the desktop app against the same dev server:

```bash
pnpm run start:dev:desktop
```

## Production

Build the web application:

```bash
pnpm run build:web
```

Package the desktop application (or build installers with `make:desktop`):

```bash
pnpm run package:desktop
```

## QA

```bash
pnpm run qa      # check-types + check-lint + check-fmt
pnpm run qa-fix  # check-types + lint --fix + prettier --write
```
