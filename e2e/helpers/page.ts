import type { Page } from "@playwright/test"
import { BASE_URL } from "./config"

// visit navigates to a path and waits until the app is interactive.
//
// These pages are server-rendered, so their markup — buttons included —
// is served before vue has attached anything to it, and an interaction
// that lands before hydration finishes does nothing. Vue stamps the app
// instance onto the mount root as it finishes, which is the first moment
// a click is guaranteed to register. Navigate through this rather than
// page.goto whenever a test then drives the page.
//
// The origin is resolved here rather than left to playwright's baseURL: the
// two suites drive two stacks on two ports, and only the helpers know which
// one this run picked. Callers pass a path or a whole URL — one read out of
// page.url() to revisit where a test already is — and URL resolution takes
// both, leaving an absolute one alone.
export async function visit(page: Page, path: string): Promise<void> {
	await page.goto(new URL(path, BASE_URL).href)
	await page.waitForFunction(
		() => "__vue_app__" in (document.getElementById("__nuxt") ?? {}),
	)
}
