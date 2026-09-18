import { beforeEach, describe, it } from "vitest"
import { isReadonly } from "vue"
import useEditorMeta from "./useEditorMeta"

// the composable is backed by a shared cookie state and the app-wide
// editor store — the tests cannot interleave, and each arranges the state
// it asserts
describe("useEditorMeta", { concurrent: false }, () => {
	beforeEach(() => {
		useEditorStore().setActiveBranchProtected(false)
		useEditorStore().updateMainAreaWidth(1400)
	})

	it("toggles the editable flag", ({ expect }) => {
		const meta = useEditorMeta()
		meta.setEditable(true)

		meta.toggleIsEditable()

		expect(meta.isEditable.value).toBe(false)
	})

	it("sets the editable flag", ({ expect }) => {
		const meta = useEditorMeta()

		meta.setEditable(false)

		expect(meta.isEditable.value).toBe(false)
	})

	it("reads a protected branch as not editable", ({ expect }) => {
		const meta = useEditorMeta()
		meta.setEditable(true)

		useEditorStore().setActiveBranchProtected(true)

		expect(meta.isEditable.value).toBe(false)
		expect(meta.isBranchProtected.value).toBe(true)
	})

	it("restores the reader's choice once the branch is unprotected", ({
		expect,
	}) => {
		const meta = useEditorMeta()
		meta.setEditable(true)
		useEditorStore().setActiveBranchProtected(true)

		useEditorStore().setActiveBranchProtected(false)

		expect(meta.isEditable.value).toBe(true)
		expect(meta.isBranchProtected.value).toBe(false)
	})

	it("locks the editor through the store", ({ expect }) => {
		const meta = useEditorMeta()

		meta.updateLock(true)

		expect(meta.isLocked.value).toBe(true)
	})

	it("reports editable and unlocked only when both hold", ({ expect }) => {
		const meta = useEditorMeta()
		meta.setEditable(true)

		meta.updateLock(false)
		expect(meta.isEditableAndUnlocked.value).toBe(true)

		meta.updateLock(true)
		expect(meta.isEditableAndUnlocked.value).toBe(false)
	})

	it("exposes a readonly editable flag", ({ expect }) => {
		const meta = useEditorMeta()

		expect(isReadonly(meta.isEditable)).toBe(true)
	})

	it("starts in compact view", ({ expect }) => {
		const meta = useEditorMeta()

		expect(meta.isCompactView.value).toBe(true)
	})

	it("reads full view while the main area is narrower than the cap", ({
		expect,
	}) => {
		const meta = useEditorMeta()

		useEditorStore().updateMainAreaWidth(1000)

		expect(meta.isCompactView.value).toBe(false)
	})

	it("returns to compact view once the main area is wide again", ({
		expect,
	}) => {
		const meta = useEditorMeta()
		useEditorStore().updateMainAreaWidth(1000)

		useEditorStore().updateMainAreaWidth(1400)

		expect(meta.isCompactView.value).toBe(true)
	})

	it("switches to full view and back", ({ expect }) => {
		const meta = useEditorMeta()

		meta.toggleCompactView()
		expect(meta.isCompactView.value).toBe(false)

		meta.toggleCompactView()
		expect(meta.isCompactView.value).toBe(true)
	})
})
