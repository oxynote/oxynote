# Oxynote design style book

How the Oxynote web and desktop app looks, and the rules a new screen or
component has to follow to belong in it. The document stands on its own:
it needs no access to the codebase.

The app defines its colours in `oklch()`. The hex values here are sRGB
approximations of them, for tools that cannot take oklch.

## 1. In one paragraph

A quiet, dense, neutral-grey document tool. Surfaces are near-white (or
near-black) with hairline borders and almost no shadow. The only brand
colour is a dark navy, used sparingly for primary buttons, links and
checked states. Colour otherwise appears only when it carries meaning:
status, diff, hooks, comments, tags, charts. Type is Inter throughout,
small (13px is the workhorse UI size), with medium and semibold doing the
work of hierarchy. Corners are tight (about 4 to 6px). Motion is short
opacity fades. The closest public reference is shadcn/ui "new-york" on the
neutral base, tightened and desaturated further.

## 2. Foundations and references

| Thing | Choice |
| --- | --- |
| Component base | [shadcn-vue](https://www.shadcn-vue.com), style `new-york`, base colour `neutral`, CSS variables on. |
| Primitives | [reka-ui](https://reka-ui.com) (the Vue port of Radix) |
| CSS | Tailwind v4, theme declared in CSS (`@theme`), no JS config. `tw-animate-css` for enter/exit, `@tailwindcss/typography` (`prose prose-neutral`) for document content |
| Variants | `class-variance-authority` + `cn()` (`tailwind-merge`) |
| Font | Inter, weights 400/500/600/700, on every element. Monospace is the Tailwind default `font-mono` stack |
| Icons | Iconify through `@nuxt/icon` (CSS mode) |
| Toasts | `vue-sonner`, bottom-right, 5s |
| Charts | ECharts; diagrams by Mermaid; both themed from the CSS tokens |
| Code | highlight.js `github` / `github-dark` themes, CodeMirror for PromQL |
| Editor | TipTap (ProseMirror) with Yjs collaboration |

Installed shadcn components: accordion, avatar, breadcrumb, button,
button-group, calendar, collapsible, dialog, drawer, dropdown-menu, empty,
form, input, input-group, kbd, label, popover, select, separator, sheet,
sidebar, skeleton, sonner, switch, textarea, tooltip. Reach for one of
these before drawing a new control.

## 3. Colour

Colours are semantic tokens with a light and a dark value. Use the token
name (`bg-muted`, `text-muted-foreground`), never a raw colour. Dark mode is
the `.dark` class on `<html>`; the user picks light, dark or auto.

### 3.1 Neutral surfaces and text

| Token | Light | Dark | Used for |
| --- | --- | --- | --- |
| `background` | `#fdfdfd` | `#121212` | page, document canvas, dialogs |
| `foreground` | `#1b1b1b` | `#ffffff` | body text |
| `card` | `#ffffff` | `#070707` | raised cards |
| `popover` | `#fdfdfd` | `#121212` | menus, popovers, tooltips, toasts |
| `sidebar` | `#f4f4f4` | `#070707` | app sidebar (a step away from the page) |
| `sidebar-foreground` | `#2e2e30` | `#dedede` | sidebar text |
| `muted` / `secondary` | `#f5f5f5` | `#090909` | callouts, code blocks, inset panels |
| `muted-50-real` | `#f9f9f9` | `#0d0d0d` | half-strength muted as a solid colour |
| `muted-highlight` | `#eeeeee` | `#111111` | code block title bar |
| `accent` / `sidebar-accent` | `#e8e8e8` | `#262626` | hover, pressed and selected fills |
| `muted-foreground` | `#737373` | `#a1a1a1` | secondary text, icons, placeholders |
| `border` / `input` | `#e5e5e5` | `#262626` | every hairline and field outline |
| `ring` | `#a1a1a1` | `#525252` | focus ring, chart crosshair |
| `scrollbar` | `#c8c9cc` | `#5a5b5d` | thin scrollbar thumb on transparent track |
| `background-translucent` | `#fdfdfd` 93% | `#121212` 93% | see-through bars over content |

The neutrals are pure grey (zero chroma). Dark mode is not inverted light
mode: the page is `#121212` while sidebar, cards and muted panels go
*darker* (`#070707`, `#090909`), and hover fills go lighter (`#262626`).

Text hierarchy is made with `muted-foreground` and opacity steps on it
(`/70`, `/60`, `/50`), not with extra grey tokens.

### 3.2 Brand

| Token | Light | Dark | Used for |
| --- | --- | --- | --- |
| `primary` | `#253d61` | `#253d61` | primary button, checked checkbox and switch, selected calendar day |
| `primary-foreground` | `#fafafa` | `#fafafa` | text on primary |
| `link` | `#253d61` | `#c2d9fc` | text links (`.link-primary`: underlined, offset 2, 80% on hover) |

Navy is the only brand hue (oklch hue 258). It is the same in both modes
as a fill; as text in dark mode it switches to the pale tint. There is no
gradient, glow or second accent colour anywhere.

### 3.3 Status and destructive

| Token | Light | Dark |
| --- | --- | --- |
| `status-info` | `#0071c2` | same |
| `status-success` | `#27a644` | same |
| `status-warning` | `#bd9f00` | same |
| `status-error` | `#e7000b` | same |
| `destructive` (fill) | `#e7000b` | `#82181a` |
| `destructive-foreground` (text) | `#e7000b` | `#fb2c36` |
| `background-warning` | `#381320` | `#d42e5b` |

Status colours tint an icon or a thin border; the surrounding box stays
neutral. Toasts are a neutral popover card with one coloured status icon.

### 3.4 Feature colours

Each belongs to one feature. Do not borrow one for something else; a new
shared meaning gets its own neutrally named token.

| Feature | Tokens | Light | Dark |
| --- | --- | --- | --- |
| Diff | `diff-added` / `diff-removed` (block wash) | `#18831f` 30% / `#e31010` 30% | `#15ab26` 25% / `#b8170f` 25% |
| | `diff-field-added` / `-removed` (field wash) | same hues at 20% | 20% / 22% |
| | `diff-added-foreground` / `diff-removed-foreground` | `#0d5e13` / `#a20808` | `#85dd83` / `#f7a598` |
| Hooks | `hook-status-fresh` (+ `-foreground`) | `#25984d` / `#0f5025` | `#6fd087` / `#b0ecbc` |
| | `hook-status-triggered` (+ `-foreground`) | navy / navy | `#87b0ed` / `#c2d9fc` |
| | `hook-status-needs-attention` (+ `-foreground`) | `#bc8218` / `#69440a` | `#f1bf5b` / `#fadea4` |
| Comments | `comment-highlight` | `#ffcb00` | same |
| Drag and drop | `drag-target` | `#c7e1f8` | `#3699e2` 36% |
| API docs | `http-method-get/post/put/patch/delete/…` | get `#045ad0`, post `#217005`, delete `#b13600`, rest muted | get `#2b9df6`, post `#3eae20`, delete `#f27400` |
| | `url-path-param` | `#8caf8c` | `#6e8a6e` |

Recurring pattern for a tinted feature surface: the feature colour at
5–15% as the fill, 20–30% as the border, and the matching `-foreground`
token for text. Example, a triggered hook notice:
`bg-hook-status-triggered/10 text-hook-status-triggered-foreground`.
Comment marks: 12% fill with a 2-unit bottom border at 35%, rising to 26%
when focused. A block with both a triggered and a needs-attention hook is
striped at -45° in the two colours (`bg-hook-status-mixed`).

### 3.5 Palettes

- **Selectable colours** (tags, user-picked chart colours): 16 Tailwind
  `-600` shades in hue order: red, orange, amber, yellow, lime, green,
  emerald, teal, cyan, sky, blue, indigo, violet, purple, fuchsia, pink.
  Default is green-600.
- **Chart series**: 16 Tailwind `-500` shades, ordered for contrast between
  neighbours: amber, teal, indigo, fuchsia, emerald, cyan, sky, blue,
  violet, lime, yellow, purple, orange, pink, green, red.
- **Collaborator carets**: Tailwind 500–800 of 17 hues (the 16 above plus
  rose).
- **Tag pills** derive from one stored colour. Light: 13% of the colour as
  fill, text is the colour mixed 80% with black, no border. Dark: 18% fill,
  40% border, text mixed 60% with white.

## 4. Typography

Inter everywhere. Sizes, with the extra in-between steps this app adds to
Tailwind's scale:

| Class | Size / line | Role |
| --- | --- | --- |
| `text-2xs` | 11 / 13px | section labels (uppercase, `tracking-wide`, semibold), tiny counters |
| `text-xs` | 12 / 16px | secondary lines, pills, kbd, notices |
| **`text-2sm`** | **13 / 15px** | **default UI text**: menus, sidebar rows, toasts, tooltips, small buttons |
| `text-sm` | 14 / 20px | default buttons, inputs, callout text, code |
| `text-2base` | 15 / 19px | document body, modal section headings, large inputs |
| `text-base` | 16 / 24px | inputs on phones (avoids iOS zoom) |
| `text-lg` | 18px | auth page headings |
| `text-2xl` | 24px | document title, settings modal title |

- Weights: `font-medium` (500) for control labels and item titles,
  `font-semibold` (600) for headings, pills and emphasis, 400 for body.
  Bold (700) is rare.
- Document content: body `text-2base`; H1 1.5em, H2 1.3em, H3 1.1em (about
  22.5 / 19.5 / 16.5px), all in `foreground`. Blocks are separated by
  1.25rem. Blockquotes are upright, without quote marks. Links in prose are
  foreground-coloured.
- Inline code: `rounded-sm border border-border bg-muted`, normal weight,
  padding `0.075em 0.25em`, no backticks.
- No letter-spacing changes except the uppercase section labels.

## 5. Shape: radius, borders, shadows

**Radius.** The base is `--radius: 0.35rem` (5.6px).

| Class | Value | Used for |
| --- | --- | --- |
| `rounded-sm` | 1.6px | inline code, kbd, small highlights |
| `rounded-md` | 3.6px | **default**: buttons, inputs, list rows, notices, diff overlays |
| `rounded` | 4px | menu and select items |
| `rounded-lg` | 5.6px | containers: dialogs, popovers, menus, toasts, callouts, code blocks |
| `rounded-xl` | 9.6px | almost unused |
| `rounded-full` | pill | avatars, tag pills, count pills, switches |

Rule of thumb: controls are `md`, the box that holds them is `lg`, pills
and people are `full`. Nothing is rounder than 10px except pills.

**Borders.** One hairline (`border` = 1px, colour `border`) is the main way
surfaces are separated: header bottom edge, dialog and popover outlines,
field outlines, dividers (`h-px bg-border`). Every element defaults to
`border-border`. Dashed borders mark something empty or not yet set up
(empty states, a data source with no status). Stacked avatars and status
badges are cut out from what is behind them with a `background`-coloured
border or ring. Thicker (2-unit) borders are rare: comment marks, the data
source status ring, the diff before/after frames.

**Shadows.** Minimal, and never the only separator: a shadowed surface also
has a border.

| Shadow | Used for |
| --- | --- |
| `shadow-xs` | filled and outline buttons, select triggers, switches |
| `shadow-md` | popovers, dropdown menus, toasts |
| `shadow-lg` | dialogs, sheets |
| `shadow-fully-clear` (`0 0.3125rem 1.25rem rgb(0 0 0 / .15)`) | floating elements over content |
| none | cards, callouts, code blocks, sidebar, header |

**Overlays.** Dialog scrim is black at 40%, sheet scrim black at 28%. No
backdrop blur.

## 6. Spacing, sizing and layout

Tailwind's 0.25rem unit. All lengths are `rem` or viewport proportions
(`dvh`, `dvw`, `%`), never `px`. Quarter steps (`0.75`, `1.25`, `1.75`) are
used freely for optical tuning.

- **Gaps**: `gap-1` and `gap-2` dominate; `gap-1.5` between an icon and its
  label; `gap-3` to `gap-6` between groups; `gap-8` between modal sections.
- **Padding**: rows and small controls `px-2 py-1`; menus and popovers `p-1`
  around their items; cards and panels `px-3 py-2.5` or `px-4 py-3`;
  dialogs `p-6`.
- **Control heights**: 28px (`h-7`, compact and header controls), 32px
  (`h-8`, small), 36px (`h-9`, default), 40px (`h-10`, auth pages and the
  document header bar).
- **Icon buttons**: 36, 26, 20 and 15px squares (`icon`, `icon-sm`,
  `icon-xsm`, `icon-2xsm`).
- **Icons**: 16px default; 14px and 12px in dense rows and pills; 32px+
  only in empty states. Stacked people avatars are 24px.

Page skeleton:

- **Sidebar**: left, `sidebar` colour, 14.5rem wide by default, resizable
  12–18rem by a hairline handle that thickens on hover; 14rem sheet on
  phones.
- **Document header**: sticky, 2.5rem tall, `background` with a bottom
  hairline; breadcrumbs on the left, actions on the right.
- **Document canvas**: `background`, content has 1.25rem side gutters
  (3.125rem from `lg`); the compact view caps the column at 80rem and
  centres it.
- **Side panels**: the AI chat is a right-hand column in the `sidebar`
  colour with a left hairline, 30rem wide, opening over 200ms. The
  notification and variables panels share one 20–26.25rem column beside
  the sidebar, and opening one closes the other. On small screens they
  become 20rem sheets.
- **Dialogs**: centred, `rounded-lg border bg-background p-6 shadow-lg`.
  Settings is 60rem wide and at most 80dvh; search is 40rem wide, pinned at
  15dvh from the top, at most 70dvh.
- **Auth pages**: a single centred column about 16.75rem wide, logo at
  3rem, full-width 40px buttons stacked with `gap-3`.

Breakpoints are Tailwind's (`sm` 40rem, `md` 48rem, `lg` 64rem); `lg` is
the main desktop switch. A 30rem cut handles phones, and
`@media (hover: none)` hides keyboard hints and hover-only affordances on
touch devices.

Stacking order, low to high: editor content, navbar (10), sidebar and drag
handle (20), comment indicators (25), dropdowns and popovers (30), sheets
(40), modals and tooltips (50), toasts and dragged items (500). Use the
named `z-*` utilities (`z-navbar`, `z-popover`, `z-modal`, …).

## 7. Components

**Buttons.** Base: `rounded-md text-sm font-medium`,
icon 16px, `gap-2`. Hover and pressed are opacity steps of the same fill,
not a new colour. Disabled is 50% opacity.

| Variant | Look |
| --- | --- |
| `default` | navy fill, near-white text; 80% on hover, 60% pressed |
| `secondary` | `secondary` grey fill |
| `outline` | hairline border on `background`; `accent/70` on hover |
| `outline-no-effect` | outline with no hover state |
| `ghost` | no fill; `accent/70` on hover, `accent` pressed, `accent/50` when active |
| `ghost-plain`, `outline-transparent` | no fill ever; only the text dims on hover |
| `dim`, `outline-dim` | text at 30%, for controls that should recede until needed |
| `destructive` | red fill, white text |
| `ghost-plain-destructive` | red text, no fill |
| `link` | link colour, underline on hover |

Sizes: `default` 36px, `sm` 32px, `2sm` 28px with 13px text, `lg` 40px.

**Inputs.** `rounded-md border border-input bg-transparent px-3 py-1`,
`muted-foreground` placeholder. Focus: border turns `ring` with a 3px
`ring/50` halo. Invalid: `destructive` border. Text is 16px on phones and
14px from `md`. Textareas match inputs: no fill and no shadow. In dark
mode outline buttons get a faint `input/30` fill.

**Checkbox.** 16px square, `rounded-md`, hairline; checked is navy with a
square-capped white tick that fades in over 150ms. A checked task-list
item's text gets a strike-through and drops to 50% opacity.

**Switch.** Pill, about 18 × 32px, navy when on, `input` grey when off.

**Menus, selects, popovers.** Container `rounded-lg border bg-popover p-1
shadow-md`. Items `rounded px-2 py-1.25 text-2sm`, icon `muted-foreground`
16px, `gap-1.5`; highlighted `accent/50`, pressed `accent`. Destructive
items are red text with a `destructive/10` highlight. Separators are a
full-bleed hairline.

**Tooltips.** `rounded-md border bg-popover px-3 py-1.5 text-2sm`; neutral,
not inverted. Shortcuts shown inside use Kbd.

**Kbd.** `h-5 min-w-5 rounded-sm border bg-muted px-1 text-xs font-medium
text-muted-foreground`.

**Sidebar rows.** 28–32px tall, `rounded-md`, 13px text. Hover
`sidebar-accent/50`, active document `sidebar-accent/50`, pressed
`sidebar-accent`. Hovering a row swaps its icon for a collapse chevron and
fades in its actions menu. Group labels are 12px medium at 70%.

**List and result rows** (search, notifications). `rounded-md`, selected
`accent/50`, pressed `accent`; hover styling only on devices that hover.
Matched text is a `comment-highlight/40` mark.

**Toasts.** `rounded-lg border bg-popover px-3 py-2.5 shadow-md`, at least
23.75rem wide: 16px filled status icon, 13px medium title, 13px muted
description, plain close button.

**Pills.** `rounded-full px-1.75 py-0.5 text-xs font-semibold`. Tag pills
are tinted (section 3.5); a neutral pill is `bg-muted
text-muted-foreground`. Diff count pills are a bordered capsule split into
a red and a green half.

**Settings sections.** A 15px heading over a `rounded-md border bg-muted/50
px-4 py-3` panel of rows.

**Avatars.** Circles, 32px by default; generated default avatars for users
and workspaces without a picture.

**Empty states.** Centred, a 32px icon at 50% opacity over one line of
15px muted text.

**Loading.** One spinner everywhere: `svg-spinners:blocks-shuffle-3`.
Skeletons are `rounded-md bg-primary/10` with a pulse.

**Editor blocks.**

- Callout: `rounded-lg bg-muted`, no border, icon at the top left, 14px
  text.
- Code block: `rounded-lg bg-muted`, monospace 13–14px, no border. With a
  title: a `muted-highlight` title bar, and the whole block gains a
  hairline.
- Hook notice: `rounded-md` tinted box (section 3.4), 12px text, 14px
  status icon, optional full-width action footer divided by a top border.
- Diff: added and removed blocks get a `rounded-md` translucent green or
  red wash behind them; changed text gets the same wash inline, and a link
  whose target alone changed gets the green wash, its popover showing old
  and new target; a changed block shows a pill marker that opens a
  before/after card.
- Comments: yellow underline-and-wash marks in text, a yellow wash over
  commented blocks, indicators in the right margin.

## 8. Icons

- Chrome (toolbars, menus, buttons, sidebar) uses **outline stroke icons**.
  Lucide is what most of the existing chrome uses, but the app is moving
  off it: prefer **MingCute** `-line` icons for anything new, and match the
  stroke style of the neighbours.
- **Filled** MingCute icons (`-fill`) are for status glyphs (check, close,
  info and warning circles), file types and selected states.
- Brand marks come from `simple-icons`; language and tool logos from
  `devicon`.
- Icons inherit text colour; in menus they are `muted-foreground`.
- The app logo is a custom SVG mark (with an "ink" variant for the
  single-colour version).
- A block type has exactly one icon, used everywhere that block is named.

## 9. Motion

- Hover and state changes: `transition-colors` or `transition-opacity`,
  150ms.
- Overlays (dialog, popover, menu, tooltip): fade in and out only. No
  zoom, no slide.
- Sheets slide from their edge: 500ms in, 300ms out.
- Sidebar resize and collapse: 200ms. Accordions and collapsible groups:
  200ms ease-out.
- Page content mounts hidden and fades in over 300ms once every block has
  settled, so layout never jumps in view.
- Things that appear on hover (row actions, drag handle, block buttons)
  fade; they do not move.
- No bounces, springs or decorative animation in the chrome.

## 10. States and accessibility

- **Hover** `accent` at 50–70%; **pressed** full `accent`; **selected or
  active** `accent/50`. Filled buttons step their own fill to 80% and 60%.
- **Focus**: 3px `ring/50` halo plus `ring` border on fields; 2px ring on
  sidebar and icon controls.
- **Disabled**: 50% opacity, `not-allowed` cursor.
- **Invalid**: `destructive` border, faint destructive ring.
- Buttons, menu items and checkboxes show `cursor-pointer`.
- Every dialog and sheet has a title and a description (visually hidden
  when nothing visible plays the role). Icon-only buttons carry an
  `sr-only` label.
- Scrollbars are thin, `scrollbar` thumb on a transparent track.

## 11. Rules for new work

1. Use tokens and the type scale above. No raw hex, no one-off greys, no
   new font sizes.
2. Lengths in `rem`, `dvh`, `dvw` or `%`. Never `px`.
3. Tailwind utilities in the template. A custom rule goes in the one global
   stylesheet, only when utilities cannot express it; no `<style>` blocks,
   no static inline styles. A value one component uses stays in its classes and does
   not become a CSS variable.
4. A feature's colour, helper or wording stays with that feature. Shared
   meaning gets a shared, neutrally named token.
5. Design both modes. A dark value is chosen, not derived by inversion.
6. Separate surfaces with a hairline or a one-step grey change before
   reaching for a shadow.
7. Colour means something. Decoration stays grey.
8. Start from an installed shadcn component and an existing variant.
9. All visible text comes from translation files, never inline.
10. Product vocabulary: the main/draft split is a **branch** (not a version
    or a mode); a hook is **triggered** (not fired).

## 12. Brief for a design tool

> Oxynote: a collaborative documentation app. Visual language is shadcn/ui
> "new-york" on the neutral palette, tightened. Inter only. UI text 13px,
> document body 15px, medium and semibold for hierarchy. Pure-grey
> surfaces: page `#fdfdfd`, sidebar `#f4f4f4`, muted panels `#f5f5f5`,
> hover `#e8e8e8`, hairline borders `#e5e5e5`, text `#1b1b1b`, secondary
> text `#737373`. Dark: page `#121212`, sidebar and panels `#070707`,
> hover and borders `#262626`, text `#ffffff`, secondary `#a1a1a1`. One
> brand colour, navy `#253d61`, for primary buttons, links and checked
> states (links are `#c2d9fc` in dark). Radius 4px on controls, 6px on
> containers, full on pills and avatars. 1px borders everywhere; shadows
> only on floating layers (menus, toasts, dialogs) and always with a
> border. Controls 28–36px tall, 16px outline icons (MingCute line style),
> filled icons only for status. Colour is reserved for meaning: green and
> red translucent washes for diffs, yellow `#ffcb00` for comments, green,
> navy and amber for hook status, Tailwind 600 shades for tags. Motion is
> 150ms fades; overlays fade without zoom. Dense, calm, no gradients, no
> glows, no decorative illustration.
