import type { DOMWrapper } from "@vue/test-utils"
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
	clearTeleportedOverlays,
	mountWithFrozenClock,
	settleActionSubmit,
	t,
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

const EXISTING_WITH_LOGIN: DataSource = {
	...EXISTING,
	configured: { username: true, password: true },
}

const EXISTING_WITH_CA: DataSource = {
	...EXISTING,
	configured: { caCertificate: true },
}

const UPSERT = "settings.action-modals.data-source-upsert"

const CA_CERTIFICATE =
	"-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----"

function mountAction(props: Record<string, unknown>) {
	return mountWithFrozenClock(DataSourceUpsertAction, { props: props })
}

// name, url, username, password — in template order — and the CA
// certificate, the only textarea
function fields(wrapper: Awaited<ReturnType<typeof mountAction>>) {
	const inputs = wrapper.findAll("input")

	return {
		name: at(inputs, 0),
		url: at(inputs, 1),
		username: at(inputs, 2),
		password: at(inputs, 3),
		caCertificate: wrapper.get("textarea"),
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

		it("offers no CA certificate reset", async ({ expect }) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})

			expect(resetButtons(wrapper)).toHaveLength(0)
		})

		it("sends a CA certificate with the credentials", async ({ expect }) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).name.setValue("Prod metrics")
			await fields(wrapper).url.setValue("https://prometheus.test")
			await fields(wrapper).caCertificate.setValue(CA_CERTIFICATE)

			await submit(wrapper)

			expect(calls[0]?.body).toMatchObject({
				credentials: {
					username: "",
					password: "",
					caCertificate: CA_CERTIFICATE,
				},
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
			DataSourceStatus.Unauthorized,
			DataSourceStatus.Unreachable,
			DataSourceStatus.VersionNotSupported,
			DataSourceStatus.NotReadOnly,
			DataSourceStatus.InvalidCACertificate,
		])(
			"explains a %s rejection from the server",
			async (status, { expect }) => {
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

				expect(rendered[0]?.description).toBe(
					t(
						`settings.action-modals.data-source-upsert.error-message.status.${status}.description`,
					),
				)
				expect(wrapper.emitted("close")).toBeUndefined()
			},
		)

		it("falls back to a generic warning for an unrecognised failure", async ({
			expect,
		}) => {
			mockEndpoint("POST", "/api/data-sources", () => {
				throw createError({ statusCode: 500 })
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

			await findButtonByText(
				wrapper,
				t("settings.action-modals.data-source-upsert.cancel-button"),
			).trigger("click")

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

		it("locks the stored credentials until they are reset", async ({
			expect,
		}) => {
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_LOGIN })

			for (const field of [
				fields(wrapper).username,
				fields(wrapper).password,
			]) {
				expect(field.attributes("disabled")).toBeDefined()
				expect(field.attributes("placeholder")).toBe(
					t(`${UPSERT}.hidden-placeholder`),
				)
			}

			// no CA certificate is stored, so there is none to lock
			expect(fields(wrapper).caCertificate.attributes("placeholder")).toBe(
				t(`${UPSERT}.creds-ca-field.placeholder`),
			)
			expect(
				fields(wrapper).caCertificate.attributes("disabled"),
			).toBeUndefined()
			expect(resetButtons(wrapper)).toHaveLength(2)
			expect(wrapper.text()).not.toContain(
				t(`${UPSERT}.url-field.credentials-reset`),
			)
		})

		it("clears a stored password after a reset", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_LOGIN })
			await at(resetButtons(wrapper), 1).trigger("click")

			await submit(wrapper)

			expect(fields(wrapper).password.attributes("disabled")).toBeUndefined()
			expect(fields(wrapper).username.attributes("disabled")).toBeDefined()
			expect(calls[0]?.body).toEqual({ credentials: { password: "" } })
		})

		it("replaces a stored username after a reset", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_LOGIN })
			await at(resetButtons(wrapper), 0).trigger("click")
			await fields(wrapper).username.setValue("reader")

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({ credentials: { username: "reader" } })
		})

		it("warns that a new url clears the stored credentials", async ({
			expect,
		}) => {
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_LOGIN })

			await fields(wrapper).url.setValue("http://other.test")

			expect(wrapper.text()).toContain(
				t(`${UPSERT}.url-field.credentials-reset`),
			)
			expect(fields(wrapper).username.attributes("disabled")).toBeUndefined()
			expect(fields(wrapper).username.attributes("placeholder")).toBe(
				t(`${UPSERT}.creds-username-field.prometheus.placeholder`),
			)
			expect(resetButtons(wrapper)).toHaveLength(0)
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

		it("sends a new CA certificate on its own", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING })
			await fields(wrapper).caCertificate.setValue(CA_CERTIFICATE)

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({
				credentials: { caCertificate: CA_CERTIFICATE },
			})
		})

		it("locks a stored CA certificate until it is reset", async ({
			expect,
		}) => {
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_CA })

			expect(fields(wrapper).caCertificate.attributes("disabled")).toBeDefined()
			expect(fields(wrapper).caCertificate.attributes("placeholder")).toBe(
				t(`${UPSERT}.hidden-placeholder`),
			)
			expect(resetButtons(wrapper)).toHaveLength(1)
		})

		it("clears the stored CA certificate after a reset", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_CA })
			await at(resetButtons(wrapper), 0).trigger("click")

			await submit(wrapper)

			expect(
				fields(wrapper).caCertificate.attributes("disabled"),
			).toBeUndefined()
			expect(resetButtons(wrapper)).toHaveLength(0)
			expect(calls[0]?.body).toEqual({
				credentials: { caCertificate: "" },
			})
		})

		it("replaces the stored CA certificate after a reset", async ({
			expect,
		}) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_CA })
			await at(resetButtons(wrapper), 0).trigger("click")
			await fields(wrapper).caCertificate.setValue(CA_CERTIFICATE)

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({
				credentials: { caCertificate: CA_CERTIFICATE },
			})
		})

		it("unlocks the CA certificate when the url changes", async ({
			expect,
		}) => {
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_CA })

			await fields(wrapper).url.setValue("http://other.test")

			expect(
				fields(wrapper).caCertificate.attributes("disabled"),
			).toBeUndefined()
			expect(fields(wrapper).caCertificate.attributes("placeholder")).toBe(
				t(`${UPSERT}.creds-ca-field.placeholder`),
			)
			expect(resetButtons(wrapper)).toHaveLength(0)
		})

		it("sends the credentials typed with a new url", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING })
			await fields(wrapper).url.setValue("http://other.test")
			await fields(wrapper).password.setValue("new-secret")

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({
				url: "http://other.test",
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
				throw createError({ statusCode: 500 })
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
			DataSourceType.Supabase,
			DataSourceType.MySQL,
			DataSourceType.MariaDB,
		])("warns that %s needs a read-only user", async (type, { expect }) => {
			const wrapper = await mountAction({ creationType: type })

			expect(wrapper.text()).toContain("read-only")
		})
	})

	describe("field help", { concurrent: false }, () => {
		beforeEach(clearTeleportedOverlays)

		it("explains where supabase keeps its connection string", async ({
			expect,
		}) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Supabase,
			})
			const help = helpButtons(wrapper)

			await openPopover(at(help, 0))

			expect(help).toHaveLength(2)
			expect(document.body.textContent).toContain(
				t(`${UPSERT}.url-field.supabase-help.placeholders.credentials`),
			)
			expect(document.body.textContent).toContain(
				t(`${UPSERT}.url-field.supabase-help.placeholders.user`),
			)
		})

		it("explains where supabase keeps its CA certificate", async ({
			expect,
		}) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Supabase,
			})

			await openPopover(at(helpButtons(wrapper), 1))

			expect(document.body.textContent).toContain(
				t(`${UPSERT}.creds-ca-field.help`),
			)
			expect(document.body.textContent).toContain(
				t(`${UPSERT}.creds-ca-field.supabase-help.placeholders.file`),
			)
		})

		it("gives postgresql only the CA certificate help", async ({ expect }) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.PostgreSQL,
			})
			const help = helpButtons(wrapper)

			await openPopover(at(help, 0))

			expect(help).toHaveLength(1)
			expect(document.body.textContent).toContain(
				t(`${UPSERT}.creds-ca-field.help`),
			)
			expect(document.body.textContent).not.toContain(
				t(`${UPSERT}.creds-ca-field.supabase-help.placeholders.file`),
			)
		})
	})
})

function helpButtons(wrapper: Awaited<ReturnType<typeof mountAction>>) {
	return wrapper
		.findAll("button")
		.filter((b) => b.text() === t(`${UPSERT}.help-button`))
}

// reka opens a popover on the pointerdown and click a user's click sends
async function openPopover(trigger: DOMWrapper<HTMLButtonElement>) {
	await trigger.trigger("pointerdown", { button: 0 })
	await trigger.trigger("click")
	await nextTick()
}

function resetButtons(wrapper: Awaited<ReturnType<typeof mountAction>>) {
	return wrapper
		.findAll("button")
		.filter((b) => b.text() === t(`${UPSERT}.reset-button`))
}
