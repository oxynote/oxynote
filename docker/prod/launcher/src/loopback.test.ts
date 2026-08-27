import { describe, it, vi } from "vitest"
import { checkLoopbackExposure } from "./loopback.js"

describe("checkLoopbackExposure", () => {
	it("reports nothing when every dial is refused", async ({ expect }) => {
		const connects = vi.fn().mockResolvedValue(false)

		const exposed = await checkLoopbackExposure(
			{ addresses: () => ["172.18.0.5"], connects },
			[8180, 8181, 3000],
		)

		expect(exposed).toEqual([])
		expect(connects).toHaveBeenCalledTimes(3)
		expect(connects).toHaveBeenCalledWith("172.18.0.5", 8180)
	})

	it("names every address and port that accepted", async ({ expect }) => {
		const connects = vi.fn(
			(_host: string, port: number): Promise<boolean> =>
				Promise.resolve(port === 8180),
		)

		const exposed = await checkLoopbackExposure(
			{
				addresses: () => ["172.18.0.5", "10.0.0.2"],
				connects,
			},
			[8180, 3000],
		)

		expect(exposed).toEqual(["172.18.0.5:8180", "10.0.0.2:8180"])
	})

	it("passes with no external addresses at all", async ({ expect }) => {
		const connects = vi.fn().mockResolvedValue(true)

		const exposed = await checkLoopbackExposure(
			{ addresses: () => [], connects },
			[8180],
		)

		expect(exposed).toEqual([])
		expect(connects).toHaveBeenCalledTimes(0)
	})
})
