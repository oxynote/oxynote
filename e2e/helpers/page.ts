import type { Page } from "@playwright/test"

// visit navigates to a path and waits until the app is interactive.
//
// These pages are server-rendered, so their markup — buttons included —
// is served before vue has attached anything to it, and an interaction
// that lands before hydration finishes does nothing. Vue stamps the app
// instance onto the mount root as it finishes, which is the first moment
// a click is guaranteed to register. Navigate through this rather than
// page.goto whenever a test then drives the page.
export async function visit(page: Page, path: string): Promise<void> {
	await page.goto(path)
	await page.waitForFunction(
		() => "__vue_app__" in (document.getElementById("__nuxt") ?? {}),
	)
}
