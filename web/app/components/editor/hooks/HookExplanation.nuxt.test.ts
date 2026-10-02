import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import HookExplanation from "./HookExplanation.vue"
import { t } from "~/components/test-helpers"

describe("<HookExplanation>", () => {
	it.for([
		{
			name: "what a waiting block hook will do",
			triggered: false,
			nodeId: "block-1",
			key: "editor.hooks.url-watcher.existing-item-block-explanation",
		},
		{
			name: "what a waiting page hook will do",
			triggered: false,
			nodeId: null,
			key: "editor.hooks.url-watcher.existing-item-full-document-explanation",
		},
		{
			name: "what a triggered block hook did",
			triggered: true,
			nodeId: "block-1",
			key: "editor.hooks.url-watcher.triggered-item-block-explanation",
		},
		{
			name: "what a triggered page hook did",
			triggered: true,
			nodeId: null,
			key: "editor.hooks.url-watcher.triggered-item-full-document-explanation",
		},
	])("says $name", async ({ triggered, nodeId, key }, { expect }) => {
		const wrapper = await mountSuspended(HookExplanation, {
			props: {
				type: DocumentHookType.URLWatcher,
				triggered: triggered,
				nodeId: nodeId,
			},
		})

		expect(wrapper.text()).toBe(t(key))
	})

	it("fills the message's value from its slot", async ({ expect }) => {
		const wrapper = await mountSuspended(HookExplanation, {
			props: {
				type: DocumentHookType.ScheduledReminder,
				triggered: false,
				nodeId: "block-1",
			},
			slots: { value: () => "Oct 15, 2026" },
		})

		expect(wrapper.text()).toBe(
			t("editor.hooks.scheduled-reminder.existing-item-block-explanation", {
				value: "Oct 15, 2026",
			}),
		)
	})
})
