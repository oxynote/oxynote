import { setResponseStatus } from "h3"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import {
	clearQueryCache,
	disposeMockEndpoints,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import DataSourceUpsertAction from "./DataSourceUpsertAction.vue"
import {
	at,
	findButtonByText,
	mountWithFrozenClock,
	settleActionSubmit,
} from "../test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const EXISTING: DataSource = {
	id: "ds1".padEnd(20, "0"),
	name: "Prod metrics",
	type: DataSourceType.Prometheus,
	url: "http://prometheus.test",
	status: DataSourceStatus.Success,
	createdAt: "2026-01-01T00:00:00Z",
	updatedAt: null,
}

function mountAction(props: Record<string, unknown>) {
	return mountWithFrozenClock(DataSourceUpsertAction, { props: props })
}

// name, url, username, password — in template order
function fields(wrapper: Awaited<ReturnType<typeof mountAction>>) {
	const inputs = wrapper.findAll("input")

	return {
		name: at(inputs, 0),
		url: at(inputs, 1),
		username: at(inputs, 2),
		password: at(inputs, 3),
	}
}

async function submit(wrapper: Awaited<ReturnType<typeof mountAction>>) {
	await wrapper.get("form").trigger("submit")
	await settleActionSubmit()
}

// showToastMessage hands vue-sonner a render function, so the toast's own
// props only exist once that is called
function renderToastProps(render: unknown) {
	if (typeof render !== "function") {
		throw new Error("toast.custom was not called with a render function")
	}

	return (render as () => { props: { description?: string } })().props
}

function statusError(status: DataSourceStatus) {
	return (_call: unknown, event: Parameters<typeof setResponseStatus>[0]) => {
		setResponseStatus(event, 400)

		return { code: `data_source.${status}` }
	}
}

// the query cache and the vue-sonner module mock are app-wide singletons
// every mount in the file shares, and the submit flow is driven by the
// global fake timers
describe("<DataSourceUpsertAction>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		mockEndpoint("GET", "/api/data-sources", () => [])
	})

	afterEach(disposeMockEndpoints)

	describe("connecting a new data source", { concurrent: false }, () => {
		it("describes the data source being connected", async ({ expect }) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})

			expect(wrapper.text()).toContain("Connect a new Prometheus server")
		})

		it("starts with empty fields", async ({ expect }) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})

			expect(fields(wrapper).name.element.value).toBe("")
			expect(fields(wrapper).url.element.value).toBe("")
		})

		it("sends the filled-in data source to the server", async ({ expect }) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).name.setValue("Prod metrics")
			await fields(wrapper).url.setValue("http://prometheus.test")
			await fields(wrapper).username.setValue("reader")
			await fields(wrapper).password.setValue("secret")

			await submit(wrapper)

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({
				type: DataSourceType.Prometheus,
				name: "Prod metrics",
				url: "http://prometheus.test",
				credentials: { username: "reader", password: "secret" },
			})
		})

		it("sends empty credentials when none were given", async ({ expect }) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).name.setValue("Prod metrics")
			await fields(wrapper).url.setValue("http://prometheus.test")

			await submit(wrapper)

			expect(calls[0]?.body).toMatchObject({
				credentials: { username: "", password: "" },
			})
		})

		it("confirms and closes once the data source is connected", async ({
			expect,
		}) => {
			mockEndpoint("POST", "/api/data-sources", () => ({ id: EXISTING.id }))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).name.setValue("Prod metrics")
			await fields(wrapper).url.setValue("http://prometheus.test")

			await submit(wrapper)

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(wrapper.emitted("close")).toHaveLength(1)
		})

		it("rejects a url that is not a url", async ({ expect }) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).name.setValue("Prod metrics")
			await fields(wrapper).url.setValue("not-a-url")

			await submit(wrapper)

			expect(calls).toHaveLength(0)
			expect(wrapper.emitted("close")).toBeUndefined()
		})

		it("rejects an empty nickname", async ({ expect }) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).url.setValue("http://prometheus.test")

			await submit(wrapper)

			expect(calls).toHaveLength(0)
		})

		it.for([
			{
				status: DataSourceStatus.Unauthorized,
				expected: "Authentication credentials are incorrect",
			},
			{
				status: DataSourceStatus.Unreachable,
				expected: "server is unreachable",
			},
			{
				status: DataSourceStatus.VersionNotSupported,
				expected: "is not supported",
			},
			{
				status: DataSourceStatus.NotReadOnly,
				expected: "only needs read permissions",
			},
		])(
			"explains a $status rejection from the server",
			async ({ status, expected }, { expect }) => {
				mockEndpoint("POST", "/api/data-sources", statusError(status))
				const wrapper = await mountAction({
					creationType: DataSourceType.Prometheus,
				})
				await fields(wrapper).name.setValue("Prod metrics")
				await fields(wrapper).url.setValue("http://prometheus.test")

				await submit(wrapper)

				const rendered = vi
					.mocked(toast.custom)
					.mock.calls.map((call) => renderToastProps(call[0]))

				expect(rendered[0]?.description).toContain(expected)
				expect(wrapper.emitted("close")).toBeUndefined()
			},
		)

		it("falls back to a generic warning for an unrecognised failure", async ({
			expect,
		}) => {
			mockEndpoint("POST", "/api/data-sources", () => {
				throw new Error("boom")
			})
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).name.setValue("Prod metrics")
			await fields(wrapper).url.setValue("http://prometheus.test")

			await submit(wrapper)

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(wrapper.emitted("close")).toBeUndefined()
		})

		it("closes without connecting anything when cancelled", async ({
			expect,
		}) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})

			await findButtonByText(wrapper, "Cancel").trigger("click")

			expect(calls).toHaveLength(0)
			expect(wrapper.emitted("close")).toHaveLength(1)
		})
	})

	describe("updating a data source", { concurrent: false }, () => {
		it("describes the data source being updated", async ({ expect }) => {
			const wrapper = await mountAction({ updateTarget: EXISTING })

			expect(wrapper.text()).toContain("Update the settings of your Prometheus")
		})

		it("starts from the data source's current values", async ({ expect }) => {
			const wrapper = await mountAction({ updateTarget: EXISTING })

			expect(fields(wrapper).name.element.value).toBe("Prod metrics")
			expect(fields(wrapper).url.element.value).toBe("http://prometheus.test")
		})

		it("hides the stored credentials behind a placeholder", async ({
			expect,
		}) => {
			const wrapper = await mountAction({ updateTarget: EXISTING })

			expect(fields(wrapper).username.attributes("placeholder")).toBe(
				"[protected]",
			)
		})

		it("sends only the fields that actually changed", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING })
			await fields(wrapper).name.setValue("Staging metrics")

			await submit(wrapper)

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({ name: "Staging metrics" })
		})

		it("sends new credentials on their own", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING })
			await fields(wrapper).password.setValue("new-secret")

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({
				credentials: { password: "new-secret" },
			})
		})

		it("closes without a request when nothing changed", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING })

			await submit(wrapper)

			expect(calls).toHaveLength(0)
			expect(wrapper.emitted("close")).toHaveLength(1)
		})

		it("confirms and closes once the data source is updated", async ({
			expect,
		}) => {
			mockEndpoint("PUT", `/api/data-sources/${EXISTING.id}`, () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({ updateTarget: EXISTING })
			await fields(wrapper).name.setValue("Staging metrics")

			await submit(wrapper)

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(wrapper.emitted("close")).toHaveLength(1)
		})

		it("explains a rejection from the server", async ({ expect }) => {
			mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				statusError(DataSourceStatus.Unreachable),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING })
			await fields(wrapper).name.setValue("Staging metrics")

			await submit(wrapper)

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(wrapper.emitted("close")).toBeUndefined()
		})

		it("falls back to a generic warning for an unrecognised failure", async ({
			expect,
		}) => {
			mockEndpoint("PUT", `/api/data-sources/${EXISTING.id}`, () => {
				throw new Error("boom")
			})
			const wrapper = await mountAction({ updateTarget: EXISTING })
			await fields(wrapper).name.setValue("Staging metrics")

			await submit(wrapper)

			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(wrapper.emitted("close")).toBeUndefined()
		})
	})

	describe("read-only warning", { concurrent: false }, () => {
		it("leaves prometheus without a read-only warning", async ({ expect }) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})

			expect(wrapper.text()).not.toContain("read-only")
		})

		it.for([
			DataSourceType.PostgreSQL,
			DataSourceType.MySQL,
			DataSourceType.MariaDB,
		])("warns that %s needs a read-only user", async (type, { expect }) => {
			const wrapper = await mountAction({ creationType: type })

			expect(wrapper.text()).toContain("read-only")
		})
	})
})
