import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import DataSourceRemovalAction from "./DataSourceRemovalAction.vue"
import { findButtonByText, settleMutations } from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DATA_SOURCE: DataSource = {
	id: "ds1".padEnd(20, "0"),
	name: "Prod metrics",
	type: DataSourceType.Prometheus,
	url: "http://prometheus.test",
	status: DataSourceStatus.Success,
	createdAt: "2026-01-01T00:00:00Z",
	updatedAt: null,
}

function mountAction() {
	return mountSuspended(DataSourceRemovalAction, {
		props: { data: DATA_SOURCE },
	})
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares, and the delete flow is driven by the
// global fake timers
describe("<DataSourceRemovalAction>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		vi.useFakeTimers()
		mockEndpoint("GET", "/api/data-sources", () => [])
	})

	afterEach(disposeMockEndpoints)

	it("names the data source it is about to remove", async ({ expect }) => {
		const wrapper = await mountAction()

		expect(wrapper.text()).toContain("Prod metrics")
	})

	it("removes the data source when the submit button is pressed", async ({
		expect,
	}) => {
		const calls = mockEndpoint(
			"DELETE",
			`/api/data-sources/${DATA_SOURCE.id}`,
			() => ({}),
		)
		const wrapper = await mountAction()

		await findButtonByText(wrapper, "Remove Data Source").trigger("click")
		await vi.advanceTimersByTimeAsync(300)
		await settleMutations()

		expect(calls).toHaveLength(1)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})

	it("confirms the removal", async ({ expect }) => {
		mockEndpoint("DELETE", `/api/data-sources/${DATA_SOURCE.id}`, () => ({}))
		const wrapper = await mountAction()

		await findButtonByText(wrapper, "Remove Data Source").trigger("click")
		await vi.advanceTimersByTimeAsync(300)
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it("shows a spinner and disables both buttons while removing", async ({
		expect,
	}) => {
		mockEndpoint("DELETE", `/api/data-sources/${DATA_SOURCE.id}`, () => ({}))
		const wrapper = await mountAction()

		await findButtonByText(wrapper, "Remove Data Source").trigger("click")
		await nextTick()

		expect(wrapper.find(".iconify").exists()).toBe(true)
		expect(
			findButtonByText(wrapper, "Remove Data Source").attributes("disabled"),
		).toBeDefined()
		expect(
			findButtonByText(wrapper, "Cancel").attributes("disabled"),
		).toBeDefined()
	})

	it("warns and stays open when the removal fails", async ({ expect }) => {
		mockEndpoint("DELETE", `/api/data-sources/${DATA_SOURCE.id}`, () => {
			throw new Error("boom")
		})
		const wrapper = await mountAction()

		await findButtonByText(wrapper, "Remove Data Source").trigger("click")
		await vi.advanceTimersByTimeAsync(300)
		await settleMutations()

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(wrapper.emitted("close")).toBeUndefined()
	})

	it("closes without removing anything when cancelled", async ({ expect }) => {
		const calls = mockEndpoint(
			"DELETE",
			`/api/data-sources/${DATA_SOURCE.id}`,
			() => ({}),
		)
		const wrapper = await mountAction()

		await findButtonByText(wrapper, "Cancel").trigger("click")

		expect(calls).toHaveLength(0)
		expect(wrapper.emitted("close")).toHaveLength(1)
	})
})
