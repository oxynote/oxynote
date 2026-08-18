import { describe, it, vi } from "vitest"
import { SIDEBAR_MOBILE_BREAKPOINT } from "~/components/shadcn/ui/sidebar/constants"
import plugin from "./ssr-width.server"

describe("ssr-width.server", () => {
	it("provides an ssr width above the sidebar mobile breakpoint", ({
		expect,
	}) => {
		const provide = vi.fn()

		void plugin({ vueApp: { provide } } as unknown as Parameters<
			typeof plugin
		>[0])

		expect(provide).toHaveBeenCalledTimes(1)
		expect(provide.mock.calls[0]?.[1]).toBe(SIDEBAR_MOBILE_BREAKPOINT + 1)
	})
})
