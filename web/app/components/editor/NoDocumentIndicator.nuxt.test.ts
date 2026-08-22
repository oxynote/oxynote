import { mountSuspended } from "@nuxt/test-utils/runtime"
import type { VueWrapper } from "@vue/test-utils"
import { describe, it } from "vitest"
import NoDocumentIndicator from "./NoDocumentIndicator.vue"
import {
	findButtonByText,
	renderedIconNames,
	t,
} from "~/components/test-helpers"

const CREATE_BUTTON = "editor.no-document-indicator.create-document-button"

function mountIndicator(props: Record<string, unknown> = {}) {
	return mountSuspended(NoDocumentIndicator, {
		props: { allSectionsLoaded: true, createLoading: false, ...props },
	})
}

// the body fades in with v-show, which leaves it in the dom with an
// inline display:none
function hidden(wrapper: VueWrapper): boolean {
	return (wrapper.get("div").element as HTMLElement).style.display === "none"
}

describe("<NoDocumentIndicator>", () => {
	it("invites the reader to create their first page", async ({ expect }) => {
		const wrapper = await mountIndicator()

		expect(wrapper.text()).toContain(t("editor.no-document-indicator.heading"))
		expect(wrapper.text()).toContain(t(CREATE_BUTTON))
	})

	it("stays hidden until the sidebar has finished loading", async ({
		expect,
	}) => {
		const wrapper = await mountIndicator({ allSectionsLoaded: false })

		expect(hidden(wrapper)).toBe(true)
	})

	it("appears once the sidebar has finished loading", async ({ expect }) => {
		const wrapper = await mountIndicator()

		expect(hidden(wrapper)).toBe(false)
	})

	it("reports that it has finished mounting", async ({ expect }) => {
		const wrapper = await mountIndicator()

		expect(wrapper.emitted("load-complete")).toHaveLength(1)
	})

	it("asks for a new page when the button is pressed", async ({ expect }) => {
		const wrapper = await mountIndicator()

		await findButtonByText(wrapper, t(CREATE_BUTTON)).trigger("click")

		expect(wrapper.emitted("create-document")).toHaveLength(1)
	})

	it("spins while the page is being created", async ({ expect }) => {
		const wrapper = await mountIndicator({ createLoading: true })

		expect(renderedIconNames(wrapper)).toEqual([
			"svg-spinners:blocks-shuffle-3",
		])
		expect(
			findButtonByText(wrapper, t(CREATE_BUTTON)).attributes("disabled"),
		).toBe("")
	})

	it("takes no second request while one is in flight", async ({ expect }) => {
		const wrapper = await mountIndicator({ createLoading: true })

		await findButtonByText(wrapper, t(CREATE_BUTTON)).trigger("click")

		expect(wrapper.emitted("create-document")).toBeUndefined()
	})
})
