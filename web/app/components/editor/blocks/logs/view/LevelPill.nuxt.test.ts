import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import LevelPill from "./LevelPill.vue"
import { t } from "~/components/test-helpers"

const LEVELS = [
	LogLevel.Critical,
	LogLevel.Error,
	LogLevel.Warning,
	LogLevel.Info,
	LogLevel.Debug,
	LogLevel.Trace,
]

describe("<LevelPill>", () => {
	it.for(LEVELS)("names the %s level", async (level, { expect }) => {
		const wrapper = await mountSuspended(LevelPill, { props: { level } })

		expect(wrapper.text()).toBe(t(`editor.logs.levels.${level}`))
	})

	it("gives each level its own colour", async ({ expect }) => {
		const colours = await Promise.all(
			LEVELS.map(async (level) => {
				const wrapper = await mountSuspended(LevelPill, { props: { level } })

				return wrapper
					.get("span")
					.classes()
					.filter((c) => c.startsWith("text-") && c !== "text-2xs")
					.join(" ")
			}),
		)

		expect(new Set(colours).size).toBe(LEVELS.length)
	})

	it("keeps an empty column for an unknown level", async ({ expect }) => {
		const wrapper = await mountSuspended(LevelPill, {
			props: { level: LogLevel.Unknown },
		})

		expect(wrapper.text()).toBe("")
		expect(wrapper.get("span").classes()).toContain("w-12.5")
	})
})
