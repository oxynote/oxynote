import type { RouterConfig } from "@nuxt/schema"

const SCROLL_MAX_WAIT_MS = 4000
const SCROLL_OFFSET_PX = 150 // approx height of document header + some padding

export default {
	// eslint-disable-next-line @typescript-eslint/no-deprecated -- the replacement, ScrollRestoration, exists only in vue-router/experimental
	scrollBehavior: async (to, from, savedPosition) => {
		if (savedPosition) {
			return savedPosition
		}

		// the page stays where it is when the url only loses its hash
		if (from.hash && to.fullPath === from.fullPath.split("#")[0]) {
			return false
		}

		if (to.name === "document-editor" && to.hash) {
			const pageReady = usePageReady()
			const router = useRouter()
			let rawHash = to.hash.replace(/^#/, "")
			if (!rawHash) {
				return { left: 0, top: 0 }
			}

			try {
				rawHash = decodeURIComponent(rawHash)
			} catch {
				// Use the raw hash when decoding fails.
			}

			// the block's position keeps moving until the page has faded in,
			// and an element found earlier may have been remounted by then
			await until(pageReady).toBe(true, { timeout: SCROLL_MAX_WAIT_MS })

			const targetEl = await waitForHtmlElementById(rawHash, SCROLL_MAX_WAIT_MS)
			if (targetEl) {
				requestAnimationFrame(() => {
					highlightTiptapScrollElement(rawHash, targetEl, () => {
						const route = router.currentRoute.value
						if (route.hash !== to.hash) {
							return
						}

						void router.replace({
							path: route.path,
							query: route.query,
							hash: "",
						})
					})
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
