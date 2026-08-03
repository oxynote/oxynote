# Bifrost

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

Make sure to install dependencies:

```bash
pnpm install
```

## Development Server

Start the development server on `http://localhost:3000`:

```bash
pnpm run dev
```

## Production

Build the application for production:

```bash
pnpm run build
```

Locally preview production build:

```bash
pnpm run preview
```

Check out the [deployment documentation](https://nuxt.com/docs/getting-started/deployment) for more information.
