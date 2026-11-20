import { enableAutoUnmount, type DOMWrapper } from "@vue/test-utils"
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
	menuItem,
	clearTeleportedOverlays,
	mountWithFrozenClock,
	raisedToasts,
	refuseForRole,
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
	credentials: {
		kind: "basic",
		configured: { username: true, password: true },
	},
}

const EXISTING_WITH_CA: DataSource = {
	...EXISTING,
	credentials: { kind: "basic", configured: { caCertificate: true } },
}

const EXISTING_WITH_TOKEN: DataSource = {
	...EXISTING,
	credentials: { kind: "token", configured: { token: true } },
}

const EXISTING_WITH_LOGIN_AND_CA: DataSource = {
	...EXISTING,
	credentials: {
		kind: "basic",
		configured: { username: true, password: true, caCertificate: true },
	},
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

// name, url and token, in template order, once the token kind is picked
function tokenFields(wrapper: Awaited<ReturnType<typeof mountAction>>) {
	const inputs = wrapper.findAll("input")

	return {
		name: at(inputs, 0),
		url: at(inputs, 1),
		token: at(inputs, 2),
		caCertificate: wrapper.get("textarea"),
	}
}

function kindTrigger(wrapper: Awaited<ReturnType<typeof mountAction>>) {
	return wrapper.get("[data-slot='dropdown-menu-trigger']")
}

// reka opens a dropdown on the pointerdown and click a user's click sends
async function openKindMenu(wrapper: Awaited<ReturnType<typeof mountAction>>) {
	await kindTrigger(wrapper).trigger("pointerdown", { button: 0 })
	await kindTrigger(wrapper).trigger("click")
	await nextTick()
}

async function pickKind(
	wrapper: Awaited<ReturnType<typeof mountAction>>,
	kind: DataSourceCredentialsKind,
) {
	await openKindMenu(wrapper)
	menuItem(kindTitle(kind)).click()
	await nextTick()
}

function kindMenuItems(): HTMLElement[] {
	return Array.from(
		document.body.querySelectorAll<HTMLElement>("[role^='menuitem']"),
	)
}

function kindTitle(kind: DataSourceCredentialsKind) {
	return t(`${UPSERT}.auth-kind-field.${kind}.title`)
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
	enableAutoUnmount(afterEach)

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

		it("describes a loki data source being connected", async ({ expect }) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Loki,
			})

			expect(wrapper.text()).toContain(t(`${UPSERT}.description.creation.loki`))
			expect(fields(wrapper).url.attributes("placeholder")).toBe(
				t(`${UPSERT}.url-field.loki.placeholder`),
			)
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
				credentials: { kind: "basic", username: "reader", password: "secret" },
			})
		})

		it("sends a loki data source with its credentials and CA certificate", async ({
			expect,
		}) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Loki,
			})
			await fields(wrapper).name.setValue("Prod logs")
			await fields(wrapper).url.setValue("https://loki.test")
			await fields(wrapper).username.setValue("reader")
			await fields(wrapper).password.setValue("secret")
			await fields(wrapper).caCertificate.setValue(CA_CERTIFICATE)

			await submit(wrapper)

			expect(calls).toHaveLength(1)
			expect(calls[0]?.body).toEqual({
				type: DataSourceType.Loki,
				name: "Prod logs",
				url: "https://loki.test",
				credentials: {
					kind: "basic",
					username: "reader",
					password: "secret",
					caCertificate: CA_CERTIFICATE,
				},
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

		it("shows only the access denied toast for a refused creation", async ({
			expect,
		}) => {
			mockEndpoint("POST", "/api/data-sources", (_call, event) =>
				refuseForRole(event),
			)
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).name.setValue("Prod metrics")
			await fields(wrapper).url.setValue("http://prometheus.test")

			await submit(wrapper)

			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("general.errors.role-not-allowed") },
			])
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

		it("describes a loki data source being updated", async ({ expect }) => {
			const wrapper = await mountAction({
				updateTarget: { ...EXISTING, type: DataSourceType.Loki },
			})

			expect(wrapper.text()).toContain(t(`${UPSERT}.description.update.loki`))
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
			expect(calls[0]?.body).toEqual({
				credentials: { kind: "basic", password: "" },
			})
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

			expect(calls[0]?.body).toEqual({
				credentials: { kind: "basic", username: "reader" },
			})
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
				credentials: { kind: "basic", password: "new-secret" },
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
				credentials: { kind: "basic", caCertificate: CA_CERTIFICATE },
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
				credentials: { kind: "basic", caCertificate: "" },
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
				credentials: { kind: "basic", caCertificate: CA_CERTIFICATE },
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
				credentials: { kind: "basic", password: "new-secret" },
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

		it("shows only the access denied toast for a refused update", async ({
			expect,
		}) => {
			mockEndpoint("PUT", `/api/data-sources/${EXISTING.id}`, (_call, event) =>
				refuseForRole(event),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING })
			await fields(wrapper).name.setValue("Staging metrics")

			await submit(wrapper)

			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("general.errors.role-not-allowed") },
			])
			expect(wrapper.emitted("close")).toBeUndefined()
		})
	})

	describe("authentication kind", { concurrent: false }, () => {
		beforeEach(clearTeleportedOverlays)

		it.for([DataSourceType.Prometheus, DataSourceType.Loki])(
			"offers a %s data source a token, starting on username and password",
			async (type, { expect }) => {
				const wrapper = await mountAction({ creationType: type })

				expect(wrapper.text()).toContain(t(`${UPSERT}.auth-kind-field.label`))
				expect(kindTrigger(wrapper).text()).toBe(kindTitle("basic"))
				expect(
					kindTrigger(wrapper)
						.attributes("aria-labelledby")
						?.split(" ")
						.map((id) => wrapper.get(`[id="${id}"]`).text()),
				).toEqual([t(`${UPSERT}.auth-kind-field.label`), kindTitle("basic")])
				expect(fields(wrapper).username.exists()).toBe(true)
			},
		)

		it("offers both kinds with their descriptions, the current one checked", async ({
			expect,
		}) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})

			await openKindMenu(wrapper)

			const items = kindMenuItems()

			expect(
				items.map((item) =>
					Array.from(item.querySelectorAll(":scope > div > div"), (line) =>
						line.textContent.trim(),
					),
				),
			).toEqual([
				[kindTitle("basic"), t(`${UPSERT}.auth-kind-field.basic.description`)],
				[kindTitle("token"), t(`${UPSERT}.auth-kind-field.token.description`)],
			])
			expect(
				items.map(
					(item) => item.querySelector('[class~="i-lucide:check"]') !== null,
				),
			).toEqual([true, false])
		})

		it("keeps typed credentials when the kind is switched and back", async ({
			expect,
		}) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).username.setValue("reader")
			await pickKind(wrapper, "token")
			await tokenFields(wrapper).token.setValue("secret")
			await pickKind(wrapper, "basic")

			expect((fields(wrapper).username.element as HTMLInputElement).value).toBe(
				"reader",
			)

			await pickKind(wrapper, "token")

			expect(
				(tokenFields(wrapper).token.element as HTMLInputElement).value,
			).toBe("secret")
		})

		it("saves with a token when the hidden username is too long", async ({
			expect,
		}) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).username.setValue("u".repeat(129))
			await pickKind(wrapper, "token")
			await tokenFields(wrapper).name.setValue("Prod metrics")
			await tokenFields(wrapper).url.setValue("http://prometheus.test")
			await tokenFields(wrapper).token.setValue("secret")

			await submit(wrapper)

			expect(calls[0]?.body).toMatchObject({
				credentials: { kind: "token", token: "secret" },
			})
		})

		it.for([
			DataSourceType.PostgreSQL,
			DataSourceType.Supabase,
			DataSourceType.MySQL,
			DataSourceType.MariaDB,
		])("offers a %s data source no token", async (type, { expect }) => {
			const wrapper = await mountAction({ creationType: type })

			expect(wrapper.text()).not.toContain(t(`${UPSERT}.auth-kind-field.label`))
			expect(wrapper.find("[data-slot='dropdown-menu-trigger']").exists()).toBe(
				false,
			)
		})

		it("sends basic credentials for a sql data source", async ({ expect }) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.PostgreSQL,
			})
			await fields(wrapper).name.setValue("Prod db")
			await fields(wrapper).url.setValue("postgresql://db.test:5432/main")
			await fields(wrapper).username.setValue("reader")

			await submit(wrapper)

			expect(calls[0]?.body).toMatchObject({
				credentials: { kind: "basic", username: "reader", password: "" },
			})
		})

		it("sends a token in place of a username and password", async ({
			expect,
		}) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Loki,
			})
			await pickKind(wrapper, "token")
			await tokenFields(wrapper).name.setValue("Prod logs")
			await tokenFields(wrapper).url.setValue("https://loki.test")
			await tokenFields(wrapper).token.setValue("secret")
			await tokenFields(wrapper).caCertificate.setValue(CA_CERTIFICATE)

			await submit(wrapper)

			expect(kindTrigger(wrapper).text()).toBe(kindTitle("token"))
			expect(tokenFields(wrapper).token.attributes("type")).toBe("password")
			expect(calls[0]?.body).toEqual({
				type: DataSourceType.Loki,
				name: "Prod logs",
				url: "https://loki.test",
				credentials: {
					kind: "token",
					token: "secret",
					caCertificate: CA_CERTIFICATE,
				},
			})
		})

		it("keeps the authentication kind while the submit is in flight", async ({
			expect,
		}) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await fields(wrapper).name.setValue("Prod metrics")
			await fields(wrapper).url.setValue("http://prometheus.test")
			await fields(wrapper).username.setValue("reader")

			await wrapper.get("form").trigger("submit")
			// validation has run and the submit waits out its spinner delay
			await vi.advanceTimersByTimeAsync(100)

			expect(kindTrigger(wrapper).attributes("disabled")).toBeDefined()

			await openKindMenu(wrapper)

			expect(kindMenuItems()).toHaveLength(0)

			await settleActionSubmit()

			expect(calls[0]?.body).toMatchObject({
				credentials: { kind: "basic", username: "reader" },
			})
		})

		it("rejects an empty token", async ({ expect }) => {
			const calls = mockEndpoint("POST", "/api/data-sources", () => ({
				id: EXISTING.id,
			}))
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await pickKind(wrapper, "token")
			await tokenFields(wrapper).name.setValue("Prod metrics")
			await tokenFields(wrapper).url.setValue("http://prometheus.test")

			await submit(wrapper)

			expect(calls).toHaveLength(0)
			expect(wrapper.text()).toContain(
				t(`${UPSERT}.creds-token-field.required`),
			)
		})

		it("explains how the token is sent", async ({ expect }) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await pickKind(wrapper, "token")

			await openPopover(at(helpButtons(wrapper), 0))

			expect(document.body.textContent).toContain(
				t(`${UPSERT}.creds-token-field.help`),
			)
		})

		it("opens on a stored token, locked until it is reset", async ({
			expect,
		}) => {
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_TOKEN })

			expect(kindTrigger(wrapper).text()).toBe(kindTitle("token"))
			expect(tokenFields(wrapper).token.attributes("disabled")).toBeDefined()
			expect(tokenFields(wrapper).token.attributes("placeholder")).toBe(
				t(`${UPSERT}.hidden-placeholder`),
			)
			expect(resetButtons(wrapper)).toHaveLength(1)
		})

		it("keeps a stored token when only the name changes", async ({
			expect,
		}) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_TOKEN })
			await tokenFields(wrapper).name.setValue("Staging metrics")

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({ name: "Staging metrics" })
		})

		it("replaces a stored token after a reset", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_TOKEN })
			await at(resetButtons(wrapper), 0).trigger("click")
			await tokenFields(wrapper).token.setValue("rotated")

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({
				credentials: { kind: "token", token: "rotated" },
			})
		})

		it("switches a stored login to a token, keeping the CA certificate", async ({
			expect,
		}) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({
				updateTarget: EXISTING_WITH_LOGIN_AND_CA,
			})
			await pickKind(wrapper, "token")

			expect(tokenFields(wrapper).token.attributes("disabled")).toBeUndefined()
			expect(
				tokenFields(wrapper).caCertificate.attributes("disabled"),
			).toBeDefined()

			await tokenFields(wrapper).token.setValue("secret")

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({
				credentials: { kind: "token", token: "secret" },
			})
		})

		it("switches a stored token to username and password", async ({
			expect,
		}) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_TOKEN })
			await pickKind(wrapper, "basic")
			await fields(wrapper).username.setValue("reader")

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({
				credentials: { kind: "basic", username: "reader" },
			})
		})

		it("sends the token kind with a new url", async ({ expect }) => {
			const calls = mockEndpoint(
				"PUT",
				`/api/data-sources/${EXISTING.id}`,
				() => ({ id: EXISTING.id }),
			)
			const wrapper = await mountAction({ updateTarget: EXISTING_WITH_TOKEN })
			await tokenFields(wrapper).url.setValue("http://other.test")
			await tokenFields(wrapper).token.setValue("secret")

			await submit(wrapper)

			expect(calls[0]?.body).toEqual({
				url: "http://other.test",
				credentials: { kind: "token", token: "secret" },
			})
		})
	})

	describe("labels", { concurrent: false }, () => {
		it.for(Object.values(DataSourceType))(
			"labels the %s fields plainly",
			async (type, { expect }) => {
				const wrapper = await mountAction({ creationType: type })
				const labels = wrapper.findAll("label").map((l) => l.text())

				expect(labels).toEqual([
					"Name",
					"URL",
					expect.stringMatching(/^Username/),
					expect.stringMatching(/^Password/),
					expect.stringMatching(/^Trusted Certificate/),
				])
			},
		)

		it("labels the token field plainly", async ({ expect }) => {
			const wrapper = await mountAction({
				creationType: DataSourceType.Prometheus,
			})
			await pickKind(wrapper, "token")

			expect(wrapper.findAll("label").map((l) => l.text())).toEqual([
				"Name",
				"URL",
				"Token",
				expect.stringMatching(/^Trusted Certificate/),
			])
		})
	})

	describe("read-only warning", { concurrent: false }, () => {
		it.for([DataSourceType.Prometheus, DataSourceType.Loki])(
			"leaves %s without a read-only warning",
			async (type, { expect }) => {
				const wrapper = await mountAction({ creationType: type })

				expect(wrapper.text()).not.toContain("read-only")
			},
		)

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
