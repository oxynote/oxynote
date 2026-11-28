import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import SidebarSpotlight from "./SidebarSpotlight.vue"
import { t } from "./test-helpers"

const PROPS = {
	title: "Oxynote 0.12.0 is available",
	text: "You are on 0.11.0.",
	action: {
		label: "Release notes",
		link: "https://oxynote.io/blog/oxynote-v0-12",
		icon: "mingcute:external-link-line",
	},
}

describe("<SidebarSpotlight>", () => {
	it("shows its title, text and action link", async ({ expect }) => {
		const wrapper = await mountSpotlight()

		const link = wrapper.get("a")

		expect.soft(wrapper.text()).toContain(PROPS.title)
		expect.soft(wrapper.text()).toContain(PROPS.text)
		expect.soft(link.text()).toContain(PROPS.action.label)
		expect.soft(link.text()).toContain(t("sidebar.spotlight.new-tab-hint"))
		expect.soft(link.attributes("href")).toBe(PROPS.action.link)
		expect.soft(link.attributes("target")).toBe("_blank")
		expect.soft(link.attributes("rel")).toBe("noopener noreferrer")
	})

	it("asks to be dismissed from its close button", async ({ expect }) => {
		const wrapper = await mountSpotlight()

		await closeButton(wrapper).trigger("click")

		expect.soft(wrapper.emitted("dismiss")).toHaveLength(1)
		expect.soft(wrapper.emitted("action")).toBeUndefined()
	})

	it("reports that its action link was followed", async ({ expect }) => {
		const wrapper = await mountSpotlight()

		await wrapper.get("a").trigger("click")

		expect.soft(wrapper.emitted("action")).toHaveLength(1)
		expect.soft(wrapper.emitted("dismiss")).toBeUndefined()
	})
})

function mountSpotlight() {
	return mountSuspended(SidebarSpotlight, { props: PROPS })
}

function closeButton(wrapper: Awaited<ReturnType<typeof mountSpotlight>>) {
	const button = wrapper
		.findAll("button")
		.find((el) => el.text() === t("sidebar.spotlight.dismiss"))
	if (!button) {
		throw new Error("no close button")
	}

	return button
}
