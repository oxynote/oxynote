import { setResponseStatus, type H3Event } from "h3"
import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import { useHookActions } from "./hook-actions"
import { makeHook } from "./test-helpers"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
	type RecordedCall,
} from "~/composables/api/test-helpers"
import { raisedToasts, t, WAIT_FOR_OPTIONS } from "~/components/test-helpers"

vi.mock("vue-sonner", () => ({
	toast: { custom: vi.fn(), dismiss: vi.fn() },
}))

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const HOOK_ID = makeXid("hook")
const HOOK_PATH = `/api/documents/${DOCUMENT_ID}/hooks/${HOOK_ID}`
const SETTINGS = { url: "https://oxynote.test" }

// the editor store, the query cache and the mocked toast module are all
// shared, so these tests cannot interleave
describe("useHookActions", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		vi.mocked(toast.custom).mockReset()
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
	})

	afterEach(disposeMockEndpoints)

	it("creates a hook on the open block after closing the menu", async ({
		expect,
	}) => {
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const { actions, close } = await mountActions(null)

		const done = await actions.create(SETTINGS)

		expect(done).toBe(true)
		expect(close).toHaveBeenCalledTimes(1)
		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toEqual({
			type: DocumentHookType.URLWatcher,
			branchId: BRANCH_ID,
			blockId: "block-1",
			settings: SETTINGS,
		})
		expect(raisedToasts()).toEqual([])
	})

	it("creates nothing while no page is open", async ({ expect }) => {
		useEditorStore().updateActiveDocumentId(null)
		const calls = mockEndpoint(
			"POST",
			`/api/documents/${DOCUMENT_ID}/hooks`,
			() => ({ id: HOOK_ID }),
		)
		const { actions, close } = await mountActions(null)

		const done = await actions.create(SETTINGS)

		expect(done).toBe(false)
		expect(close).toHaveBeenCalledTimes(0)
		expect(calls).toHaveLength(0)
	})

	it("says so when the hook cannot be created", async ({ expect }) => {
		mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, failing)
		const { actions } = await mountActions(null)

		const done = await actions.create(SETTINGS)

		expect(done).toBe(false)
		expect(raisedToasts()).toMatchObject([
			{ type: "error", title: t("editor.hooks.errors.create-failed") },
		])
	})

	it.for([
		{ name: "creating", update: false },
		{ name: "updating", update: true },
	])(
		"names the reason core refused $name the hook for",
		async ({ update }, { expect }) => {
			const refused = (_call: RecordedCall, event: H3Event) => {
				setResponseStatus(event, 422)

				return {
					code: "document_hook.unconfigured",
					message: "the hook cannot check its target: unconfigured",
				}
			}
			mockEndpoint("POST", `/api/documents/${DOCUMENT_ID}/hooks`, refused)
			mockEndpoint("PUT", HOOK_PATH, refused)
			const { actions } = await mountActions(update ? undefined : null)

			const done = update
				? await actions.update(SETTINGS)
				: await actions.create(SETTINGS)

			expect(done).toBe(false)
			expect(raisedToasts()).toMatchObject([
				{ type: "error", title: t("editor.hooks.errors.codes.unconfigured") },
			])
		},
	)

	it("updates the hook's settings after closing the menu", async ({
		expect,
	}) => {
		const calls = mockEndpoint("PUT", HOOK_PATH, () => ({ id: HOOK_ID }))
		const { actions, close } = await mountActions()

		const done = await actions.update(SETTINGS)

		expect(done).toBe(true)
		expect(close).toHaveBeenCalledTimes(1)
		expect(calls).toHaveLength(1)
		expect(calls[0]?.body).toEqual({ settings: SETTINGS })
	})

	it.for([
		{
			name: "an update",
			input: false,
			expected: "editor.hooks.errors.update-failed",
		},
		{
			name: "a renewal",
			input: true,
			expected: "editor.hooks.errors.renew-failed",
		},
	])("says so when $name fails", async ({ input, expected }, { expect }) => {
		mockEndpoint("PUT", HOOK_PATH, failing)
		const { actions } = await mountActions()

		const done = await actions.update(SETTINGS, input)

		expect(done).toBe(false)
		expect(raisedToasts()).toMatchObject([
			{ type: "error", title: t(expected) },
		])
	})

	it("updates nothing without a hook", async ({ expect }) => {
		const calls = mockEndpoint("PUT", HOOK_PATH, () => ({ id: HOOK_ID }))
		const { actions, close } = await mountActions(null)

		const done = await actions.update(SETTINGS)

		expect(done).toBe(false)
		expect(close).toHaveBeenCalledTimes(0)
		expect(calls).toHaveLength(0)
	})

	it("deletes the hook after closing the menu", async ({ expect }) => {
		const calls = mockEndpoint("DELETE", HOOK_PATH, () => null)
		const { actions, close } = await mountActions()

		const done = await actions.remove()

		expect(done).toBe(true)
		expect(close).toHaveBeenCalledTimes(1)
		expect(calls).toHaveLength(1)
	})

	it("says so when the hook cannot be deleted", async ({ expect }) => {
		mockEndpoint("DELETE", HOOK_PATH, failing)
		const { actions } = await mountActions()

		const done = await actions.remove()

		expect(done).toBe(false)
		expect(raisedToasts()).toMatchObject([
			{ type: "error", title: t("editor.hooks.errors.delete-failed") },
		])
	})

	it("deletes nothing without a hook", async ({ expect }) => {
		const calls = mockEndpoint("DELETE", HOOK_PATH, () => null)
		const { actions, close } = await mountActions(null)

		const done = await actions.remove()

		expect(done).toBe(false)
		expect(close).toHaveBeenCalledTimes(0)
		expect(calls).toHaveLength(0)
	})

	it("resets the hook after closing the menu", async ({ expect }) => {
		const calls = mockEndpoint("PUT", `${HOOK_PATH}/reset`, () => ({
			id: HOOK_ID,
		}))
		const { actions, close } = await mountActions()

		const done = await actions.reset()

		expect(done).toBe(true)
		expect(close).toHaveBeenCalledTimes(1)
		await vi.waitFor(() => {
			expect(calls).toHaveLength(1)
		}, WAIT_FOR_OPTIONS)
	})

	it("says so when the hook cannot be reset", async ({ expect }) => {
		mockEndpoint("PUT", `${HOOK_PATH}/reset`, failing)
		const { actions } = await mountActions()

		const done = await actions.reset()

		expect(done).toBe(false)
		expect(raisedToasts()).toMatchObject([
			{ type: "error", title: t("editor.hooks.errors.reset-failed") },
		])
	})

	it("resets nothing without a hook", async ({ expect }) => {
		const calls = mockEndpoint("PUT", `${HOOK_PATH}/reset`, () => ({
			id: HOOK_ID,
		}))
		const { actions, close } = await mountActions(null)

		const done = await actions.reset()

		expect(done).toBe(false)
		expect(close).toHaveBeenCalledTimes(0)
		expect(calls).toHaveLength(0)
	})
})

// the composable reads i18n, the editor store and the api through a
// component instance, so each test mounts a small host
async function mountActions(
	hook: DocumentHook | null = makeHook({
		id: HOOK_ID,
		documentId: DOCUMENT_ID,
		branchId: BRANCH_ID,
	}),
) {
	const close = vi.fn()
	const host: { actions?: ReturnType<typeof useHookActions> } = {}
	await mountSuspended(
		defineComponent({
			setup() {
				host.actions = useHookActions({
					type: DocumentHookType.URLWatcher,
					nodeId: () => "block-1",
					hook: () => hook,
					close: close,
				})

				return () => h("div")
			},
		}),
	)
	if (!host.actions) {
		throw new Error("the host did not set up the actions")
	}

	return { actions: host.actions, close: close }
}

function failing(_call: RecordedCall, event: H3Event) {
	setResponseStatus(event, 500)

	return { message: "boom" }
}
