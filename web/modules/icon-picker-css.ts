import { readFileSync } from "node:fs"
import { createRequire } from "node:module"
import type { IconifyJSON } from "@iconify/types"
import type { ModuleOptions as IconOptions } from "@nuxt/icon"
import { getIconCSS, getIconData } from "@iconify/utils"
import { addVitePlugin, defineNuxtModule } from "@nuxt/kit"
import { selectableIconList } from "../app/utils/icon"

// the icon picker styles every selectable icon from one stylesheet built
// here out of the installed icon packs, so scrolling it never waits on a
// per-icon fetch or style injection. The selectors are the ones @nuxt/icon
// generates for <Icon>, so a picked icon and its sidebar rendering share a
// rule. Served as the virtual module "virtual:icon-picker.css"
const VIRTUAL_ID = "virtual:icon-picker.css"

export default defineNuxtModule({
	meta: { name: "icon-picker-css" },
	setup(_options, nuxt) {
		const icon: Partial<IconOptions> = nuxt.options.icon || {}
		const { cssSelectorPrefix = "i-", cssLayer, cssWherePseudo = true } = icon

		addVitePlugin({
			name: "icon-picker-css",
			resolveId(id) {
				if (id === VIRTUAL_ID) {
					return VIRTUAL_ID
				}
			},
			load(id) {
				if (id === VIRTUAL_ID) {
					return buildStylesheet(cssSelectorPrefix, cssLayer, cssWherePseudo)
				}
			},
		})
	},
})

function buildStylesheet(
	selectorPrefix: string,
	layer: string | undefined,
	wherePseudo: boolean,
) {
	const require = createRequire(import.meta.url)
	const collections = new Map<string, IconifyJSON>()

	const rules = selectableIconList().map((id) => {
		const [collection, name] = id.split(":") as [string, string]
		let set = collections.get(collection)

		if (!set) {
			set = JSON.parse(
				readFileSync(
					require.resolve(`@iconify-json/${collection}/icons.json`),
					"utf8",
				),
			) as IconifyJSON
			collections.set(collection, set)
		}

		const data = getIconData(set, name)
		if (!data) {
			throw new Error(`icon ${id} is not in its installed collection`)
		}

		// the class name escaping @nuxt/icon applies to the same selector
		const selector = "." + (selectorPrefix + id).replace(/([^\w-])/g, "\\$1")

		return getIconCSS(data, {
			iconSelector: wherePseudo ? `:where(${selector})` : selector,
			format: "compressed",
		})
	})

	const css = rules.join("")

	return layer ? `@layer ${layer}{${css}}` : css
}
