import { flushPromises, type VueWrapper } from "@vue/test-utils"
import { mountSuspended } from "@nuxt/test-utils/runtime"
import { afterEach, beforeEach, describe, it, vi } from "vitest"
import BranchViewers from "./BranchViewers.vue"
import {
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockEndpoint,
} from "~/composables/api/test-helpers"
import { seedAuthOrganization, t, WAIT_FOR_OPTIONS } from "./test-helpers"

const DOCUMENT_ID = makeXid("doc")
const BRANCH_ID = makeXid("branch")
const VIEWS_URL = `/api/documents/${DOCUMENT_ID}/branches/${BRANCH_ID}/views`
const VIEWS_KEY = ["documents", DOCUMENT_ID, "branches", BRANCH_ID, "views"]

const ADA = makeXid("usada")
const GRACE = makeXid("usgrc")

// the editor store, the auth cache and the query cache are shared app-wide,
// so these tests cannot interleave
describe("<BranchViewers>", { concurrent: false }, () => {
	beforeEach(() => {
		clearQueryCache()
		useEditorStore().updateActiveDocumentId(DOCUMENT_ID)
		useEditorStore().updateActiveBranchId(BRANCH_ID)
	})

	afterEach(disposeMockEndpoints)

	it("lists who viewed the active branch, most recent first", async ({
		expect,
	}) => {
		const viewCalls = mockEndpoint("GET", VIEWS_URL, () => [
			{ userId: GRACE, viewedAt: "2026-10-27T08:00:00Z" },
			{ userId: ADA, viewedAt: "2026-10-25T08:00:00Z" },
		])
		seedAuthOrganization({
			members: [member(ADA, "Ada"), member(GRACE, "Grace")],
		})

		const wrapper = await mountViewers()

		expect(wrapper.text()).toContain(t("editor.navbar.timestamp.viewed-by"))
		expect(viewerRows(wrapper)).toEqual([
			["Grace", "Oct 27"],
			["Ada", "Oct 25"],
		])
		expect(viewCalls).toHaveLength(1)
	})

	it("leaves out viewers who are no longer members", async ({ expect }) => {
		mockEndpoint("GET", VIEWS_URL, () => [
			{ userId: GRACE, viewedAt: "2026-10-27T08:00:00Z" },
			{ userId: ADA, viewedAt: "2026-10-25T08:00:00Z" },
		])
		seedAuthOrganization({ members: [member(ADA, "Ada")] })

		const wrapper = await mountViewers()

		expect(viewerRows(wrapper)).toEqual([["Ada", "Oct 25"]])
	})

	it("shows nothing while nobody has viewed the branch", async ({ expect }) => {
		mockEndpoint("GET", VIEWS_URL, () => [])
		seedAuthOrganization({ members: [member(ADA, "Ada")] })

		const wrapper = await mountViewers()

		expect(wrapper.text()).toBe("")
	})
})

function member(id: string, name: string) {
	return { userId: id, user: { name: name, image: undefined } }
}

// the views arrive through a request the component gives no signal for,
// so the tests wait until its answer reaches the query cache
async function mountViewers() {
	const wrapper = await mountSuspended(BranchViewers)

	await vi.waitFor(() => {
		if (useQueryCache().getQueryData(VIEWS_KEY) === undefined) {
			throw new Error("the views have not been answered yet")
		}
	}, WAIT_FOR_OPTIONS)
	await flushPromises()

	return wrapper
}

// each row as its name and its date, the avatar left out
function viewerRows(wrapper: VueWrapper): string[][] {
	return wrapper
		.findAll("li")
		.map((row) => [row.get(".truncate").text(), row.get("time").text()])
}
