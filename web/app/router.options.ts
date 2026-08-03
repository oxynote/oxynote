import type { RouterConfig } from "@nuxt/schema"

const SCROLL_MAX_WAIT_MS = 4000
const SCROLL_OFFSET_PX = 150 // approx height of document header + some padding

export default {
	scrollBehavior: async (to, _from, savedPosition) => {
		if (savedPosition) {
			return savedPosition
		}

		if (to.name === "document-editor" && to.hash) {
			let rawHash = to.hash.replace(/^#/, "")
			if (!rawHash) {
				return { left: 0, top: 0 }
			}

			try {
				rawHash = decodeURIComponent(rawHash)
			} catch {
				// Use the raw hash when decoding fails.
			}

			const targetEl = await waitForHtmlElementById(rawHash, SCROLL_MAX_WAIT_MS)
			if (targetEl) {
				requestAnimationFrame(() => {
					highlightTiptapScrollElement(rawHash, targetEl)
				})

				return {
					left: 0,
					top:
						targetEl.getBoundingClientRect().top +
						window.scrollY -
						SCROLL_OFFSET_PX,
				}
			}

			return { left: 0, top: 0 }
		}

		return { left: 0, top: 0 }
	},
} satisfies RouterConfig
