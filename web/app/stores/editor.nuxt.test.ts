import { createPinia } from "pinia"
import { describe, it, vi } from "vitest"
import type { MetricConfig } from "~/components/editor/blocks/metrics/utils"
import {
	defaultLogConfig,
	type LogConfig,
} from "~/components/editor/blocks/logs/utils"
import { DiffStatus } from "~/components/editor/diff/position-map"
import { useEditorStore } from "./editor"

function makeStore() {
	return useEditorStore(createPinia())
}

function makeActiveStore() {
	const store = makeStore()
	store.updateActiveDocumentId("doc1")
	store.updateActiveBranchId("branch1")

	return store
}

function metricConfig(title: string): MetricConfig {
	return {
		title,
		dataSourceId: null,
		visualizationType: GenericQueryChartType.Line,
		queries: null,
		timeRange: null,
		refreshInterval: null,
		baseThresholdColor: "#ffffff",
		unit: {},
		axisBounds: {},
		simulationPreset: null,
		simulationActive: false,
	}
}

function logConfig(title: string): LogConfig {
	return { ...defaultLogConfig(), title: title }
}

// timer-driven behaviour runs under fake timers inside a synchronous body,
// so the shared timer state never leaks into concurrently queued tests
function withFakeNow(run: () => void) {
	vi.useFakeTimers({ now: new Date("2026-01-01T00:00:00Z") })

	try {
		run()
	} finally {
		vi.useRealTimers()
	}
}

describe("useEditorStore", () => {
	describe("updateLock", () => {
		it("sets the lock flag", ({ expect }) => {
			const store = makeStore()

			store.updateLock(true)

			expect(store.locked).toBe(true)
		})
	})

	describe("setActiveBranchProtected", () => {
		it("sets the protected flag", ({ expect }) => {
			const store = makeStore()

			store.setActiveBranchProtected(true)

			expect(store.activeBranchProtected).toBe(true)
		})
	})

	describe("setActiveBranchReadOnly", () => {
		it("sets the connection's read-only state", ({ expect }) => {
			const store = makeActiveStore()

			store.setActiveBranchReadOnly(true)

			expect(store.activeBranchReadOnly).toBe(true)
		})

		it.for([
			{
				target: "branch",
				change: (store: ReturnType<typeof makeStore>) => {
					store.updateActiveBranchId("branch2")
				},
			},
			{
				target: "document",
				change: (store: ReturnType<typeof makeStore>) => {
					store.updateActiveDocumentId("doc2")
				},
			},
		])(
			"clears the read-only state when the $target changes",
			({ change }, { expect }) => {
				const store = makeActiveStore()
				store.setActiveBranchReadOnly(true)

				change(store)

				expect(store.activeBranchReadOnly).toBe(false)
			},
		)

		it("keeps the read-only state when the same branch and document are selected again", ({
			expect,
		}) => {
			const store = makeActiveStore()
			store.setActiveBranchReadOnly(true)

			store.updateActiveDocumentId("doc1")
			store.updateActiveBranchId("branch1")

			expect(store.activeBranchReadOnly).toBe(true)
		})
	})

	describe("updatePreloadedBranchIds", () => {
		it("replaces the preloaded branch ids", ({ expect }) => {
			const store = makeStore()

			store.updatePreloadedBranchIds(["b1", "b2"])

			expect(store.preloadedBranchIds).toEqual(["b1", "b2"])
		})
	})

	describe("updateActiveDocumentId", () => {
		it("sets the active document id", ({ expect }) => {
			const store = makeStore()

			store.updateActiveDocumentId("doc1")

			expect(store.activeDocumentId).toBe("doc1")
		})

		it("clears metric block state when the document changes", ({ expect }) => {
			const store = makeActiveStore()
			store.setMetricBlockConfig("block1", metricConfig("cfg"))
			store.activateMetricBlockConfig("block1")
			store.setMermaidBlockShowCode("block1", true)
			store.markBlockRenderSettled("block1")

			store.updateActiveDocumentId("doc2")

			expect(store.metricBlockConfigs).toEqual({})
			expect(store.activeMetricBlockConfig).toBeNull()
			expect(store.mermaidBlockShowCode).toEqual({})
			expect(store.settledBlockRenders.size).toBe(0)
		})

		it("clears log block state when the document changes", ({ expect }) => {
			const store = makeActiveStore()
			store.setLogBlockConfig("block1", logConfig("cfg"))
			store.setLogBlockDiffInfo("block1", DiffStatus.Added, null)
			store.activateLogBlockConfig("block1")

			store.updateActiveDocumentId("doc2")

			expect(store.logBlockConfigs).toEqual({})
			expect(store.logBlockDiffStatuses).toEqual({})
			expect(store.logBlockOldConfigs).toEqual({})
			expect(store.activeLogBlockConfig).toBeNull()
		})

		it("deactivates the reviewable diff when the document changes", ({
			expect,
		}) => {
			const store = makeActiveStore()
			store.setReviewableDiffActive(true)

			store.updateActiveDocumentId("doc2")

			expect(store.reviewableDiffActive).toBe(false)
		})

		it("clears the branch ids when the document changes", ({ expect }) => {
			const store = makeActiveStore()
			store.updateTargetBranchId("branch2")
			store.updateMappedDefaultBranchId("branch1")
			store.updatePreloadedBranchIds(["branch1", "branch2"])

			store.updateActiveDocumentId("doc2")

			expect(store.activeBranchId).toBeNull()
			expect(store.targetBranchId).toBeNull()
			expect(store.mappedDefaultBranchId).toBeNull()
			expect(store.preloadedBranchIds).toEqual([])
		})

		it("keeps metric block state when the id is unchanged", ({ expect }) => {
			const store = makeActiveStore()
			store.setMetricBlockConfig("block1", metricConfig("cfg"))
			store.setReviewableDiffActive(true)

			store.updateActiveDocumentId("doc1")

			expect(store.metricBlockConfigs).toEqual({
				doc1: { branch1: { block1: metricConfig("cfg") } },
			})
			expect(store.reviewableDiffActive).toBe(true)
			expect(store.activeBranchId).toBe("branch1")
		})
	})

	describe("markBlockRenderSettled", () => {
		it("remembers every settled block once", ({ expect }) => {
			const store = makeActiveStore()

			store.markBlockRenderSettled("block1")
			store.markBlockRenderSettled("block1")
			store.markBlockRenderSettled("block2")

			expect([...store.settledBlockRenders]).toEqual(["block1", "block2"])
		})
	})

	describe("updateActiveBranchId", () => {
		it("sets the active branch id", ({ expect }) => {
			const store = makeStore()

			store.updateActiveBranchId("branch1")

			expect(store.activeBranchId).toBe("branch1")
		})

		it("deactivates the reviewable diff when the branch changes", ({
			expect,
		}) => {
			const store = makeActiveStore()
			store.setReviewableDiffActive(true)

			store.updateActiveBranchId("branch2")

			expect(store.reviewableDiffActive).toBe(false)
		})

		it("keeps metric block configs when the branch changes", ({ expect }) => {
			const store = makeActiveStore()
			store.setMetricBlockConfig("block1", metricConfig("cfg"))

			store.updateActiveBranchId("branch2")

			expect(store.metricBlockConfigs).toEqual({
				doc1: { branch1: { block1: metricConfig("cfg") } },
			})
		})

		it("keeps the reviewable diff when the id is unchanged", ({ expect }) => {
			const store = makeActiveStore()
			store.setReviewableDiffActive(true)

			store.updateActiveBranchId("branch1")

			expect(store.reviewableDiffActive).toBe(true)
		})
	})

	describe("updateMappedDefaultBranchId", () => {
		it("sets the mapped default branch id", ({ expect }) => {
			const store = makeStore()

			store.updateMappedDefaultBranchId("branch1")

			expect(store.mappedDefaultBranchId).toBe("branch1")
		})
	})

	describe("updateTargetBranchId", () => {
		it("sets the target branch id", ({ expect }) => {
			const store = makeStore()

			store.updateTargetBranchId("branch1")

			expect(store.targetBranchId).toBe("branch1")
		})
	})

	describe("setBranchReviewableActionsActive", () => {
		it("sets the reviewable actions flag", ({ expect }) => {
			const store = makeStore()

			store.setBranchReviewableActionsActive(true)

			expect(store.branchReviewableActionsActive).toBe(true)
		})
	})

	describe("setReviewableDiffActive", () => {
		it("activates the reviewable diff and keeps the diff data", ({
			expect,
		}) => {
			const store = makeActiveStore()
			store.setMetricBlockDiffInfo("block1", DiffStatus.Modified, null)

			store.setReviewableDiffActive(true)

			expect(store.reviewableDiffActive).toBe(true)
			expect(store.metricBlockDiffStatuses).toEqual({
				doc1: { branch1: { block1: DiffStatus.Modified } },
			})
		})

		it("clears metric block diff data when deactivated", ({ expect }) => {
			const store = makeActiveStore()
			store.setMetricBlockDiffInfo(
				"block1",
				DiffStatus.Modified,
				metricConfig("old"),
			)

			store.setReviewableDiffActive(false)

			expect(store.metricBlockDiffStatuses).toEqual({})
			expect(store.metricBlockOldConfigs).toEqual({})
		})

		it("clears log block diff data when deactivated", ({ expect }) => {
			const store = makeActiveStore()
			store.setLogBlockConfig("block1", logConfig("cfg"))
			store.setLogBlockDiffInfo("block1", DiffStatus.Modified, logConfig("old"))

			store.setReviewableDiffActive(false)

			expect(store.logBlockDiffStatuses).toEqual({})
			expect(store.logBlockOldConfigs).toEqual({})
			expect(store.logBlockConfigs).toEqual({
				doc1: { branch1: { block1: logConfig("cfg") } },
			})
		})
	})

	describe("setMetricBlockConfig", () => {
		it("stores the config under the active document and branch", ({
			expect,
		}) => {
			const store = makeActiveStore()

			store.setMetricBlockConfig("block1", metricConfig("cfg"))

			expect(store.metricBlockConfigs).toEqual({
				doc1: { branch1: { block1: metricConfig("cfg") } },
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.setMetricBlockConfig("block1", metricConfig("cfg"))
			}).toThrow("Cannot set metric block config without an active document")
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.setMetricBlockConfig("block1", metricConfig("cfg"))
			}).toThrow("Cannot set metric block config without an active branch")
		})
	})

	describe("removeMetricBlockConfig", () => {
		it("removes the config for the active document and branch", ({
			expect,
		}) => {
			const store = makeActiveStore()
			store.setMetricBlockConfig("block1", metricConfig("cfg"))

			store.removeMetricBlockConfig("block1")

			expect(store.metricBlockConfigs).toEqual({ doc1: { branch1: {} } })
		})

		it("removes the config for an explicitly given document and branch", ({
			expect,
		}) => {
			const store = makeActiveStore()
			store.setMetricBlockConfig("block1", metricConfig("cfg"))
			store.updateActiveBranchId("branch2")

			store.removeMetricBlockConfig("block1", "doc1", "branch1")

			expect(store.metricBlockConfigs).toEqual({ doc1: { branch1: {} } })
		})

		it("ignores a block without a stored config", ({ expect }) => {
			const store = makeActiveStore()

			store.removeMetricBlockConfig("block1")

			expect(store.metricBlockConfigs).toEqual({})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.removeMetricBlockConfig("block1")
			}).toThrow("Cannot remove metric block config without an active document")
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.removeMetricBlockConfig("block1")
			}).toThrow("Cannot remove metric block config without an active branch")
		})
	})

	describe("setMetricBlockDiffInfo", () => {
		it("stores the diff status and the old config", ({ expect }) => {
			const store = makeActiveStore()

			store.setMetricBlockDiffInfo(
				"block1",
				DiffStatus.Modified,
				metricConfig("old"),
			)

			expect(store.metricBlockDiffStatuses).toEqual({
				doc1: { branch1: { block1: DiffStatus.Modified } },
			})
			expect(store.metricBlockOldConfigs).toEqual({
				doc1: { branch1: { block1: metricConfig("old") } },
			})
		})

		it("stores a null old config for an added block", ({ expect }) => {
			const store = makeActiveStore()

			store.setMetricBlockDiffInfo("block1", DiffStatus.Added, null)

			expect(store.metricBlockOldConfigs).toEqual({
				doc1: { branch1: { block1: null } },
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.setMetricBlockDiffInfo("block1", DiffStatus.Added, null)
			}).toThrow("Cannot set metric block diff info without an active document")
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.setMetricBlockDiffInfo("block1", DiffStatus.Added, null)
			}).toThrow("Cannot set metric block diff info without an active branch")
		})
	})

	describe("removeMetricBlockDiffInfo", () => {
		it("removes the diff status and the old config", ({ expect }) => {
			const store = makeActiveStore()
			store.setMetricBlockDiffInfo(
				"block1",
				DiffStatus.Modified,
				metricConfig("old"),
			)

			store.removeMetricBlockDiffInfo("block1")

			expect(store.metricBlockDiffStatuses).toEqual({ doc1: { branch1: {} } })
			expect(store.metricBlockOldConfigs).toEqual({ doc1: { branch1: {} } })
		})

		it("removes the diff info only for the explicitly given document and branch", ({
			expect,
		}) => {
			// switching the branch clears all diff data, so the info can only
			// live under the branch that is active when it is set
			const store = makeActiveStore()
			store.updateActiveBranchId("branch2")
			store.setMetricBlockDiffInfo("block1", DiffStatus.Modified, null)

			store.removeMetricBlockDiffInfo("block1", "doc1", "branch1")

			expect(store.metricBlockDiffStatuses).toEqual({
				doc1: { branch2: { block1: DiffStatus.Modified } },
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.removeMetricBlockDiffInfo("block1")
			}).toThrow(
				"Cannot remove metric block diff info without an active document",
			)
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.removeMetricBlockDiffInfo("block1")
			}).toThrow(
				"Cannot remove metric block diff info without an active branch",
			)
		})
	})

	describe("activateMetricBlockConfig", () => {
		it("sets the active config uid", ({ expect }) => {
			const store = makeStore()

			store.activateMetricBlockConfig("uid1")

			expect(store.activeMetricBlockConfig).toBe("uid1")
		})

		it("clears the active config uid", ({ expect }) => {
			const store = makeStore()
			store.activateMetricBlockConfig("uid1")

			store.activateMetricBlockConfig(null)

			expect(store.activeMetricBlockConfig).toBeNull()
		})
	})

	describe("setMetricBlockNextRefreshTimestamp", () => {
		it("schedules the next refresh after the given interval", ({ expect }) => {
			withFakeNow(() => {
				const store = makeActiveStore()

				store.setMetricBlockNextRefreshTimestamp("block1", 5000)

				expect(store.isMetricBlockDueForRefresh("block1")).toBe(false)

				vi.advanceTimersByTime(5000)

				expect(store.isMetricBlockDueForRefresh("block1")).toBe(true)
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.setMetricBlockNextRefreshTimestamp("block1", 5000)
			}).toThrow(
				"Cannot set metric block next refresh timestamp without an active document",
			)
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.setMetricBlockNextRefreshTimestamp("block1", 5000)
			}).toThrow(
				"Cannot set metric block next refresh timestamp without an active branch",
			)
		})
	})

	describe("isMetricBlockDueForRefresh", () => {
		it("reports due when no refresh is scheduled", ({ expect }) => {
			const store = makeActiveStore()

			expect(store.isMetricBlockDueForRefresh("block1")).toBe(true)
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => store.isMetricBlockDueForRefresh("block1")).toThrow(
				"Cannot check metric block refresh status without an active document",
			)
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => store.isMetricBlockDueForRefresh("block1")).toThrow(
				"Cannot check metric block refresh status without an active branch",
			)
		})
	})

	describe("clearMetricBlockNextRefreshTimestamp", () => {
		it("makes the block due again", ({ expect }) => {
			withFakeNow(() => {
				const store = makeActiveStore()
				store.setMetricBlockNextRefreshTimestamp("block1", 5000)

				store.clearMetricBlockNextRefreshTimestamp("block1")

				expect(store.isMetricBlockDueForRefresh("block1")).toBe(true)
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.clearMetricBlockNextRefreshTimestamp("block1")
			}).toThrow(
				"Cannot clear metric block refresh timestamp without an active document",
			)
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.clearMetricBlockNextRefreshTimestamp("block1")
			}).toThrow(
				"Cannot clear metric block refresh timestamp without an active branch",
			)
		})
	})

	describe("setLogBlockConfig", () => {
		it("stores the config under the active document and branch", ({
			expect,
		}) => {
			const store = makeActiveStore()

			store.setLogBlockConfig("block1", logConfig("cfg"))

			expect(store.logBlockConfigs).toEqual({
				doc1: { branch1: { block1: logConfig("cfg") } },
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.setLogBlockConfig("block1", logConfig("cfg"))
			}).toThrow("Cannot set log block config without an active document")
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.setLogBlockConfig("block1", logConfig("cfg"))
			}).toThrow("Cannot set log block config without an active branch")
		})
	})

	describe("removeLogBlockConfig", () => {
		it("removes the config for the active document and branch", ({
			expect,
		}) => {
			const store = makeActiveStore()
			store.setLogBlockConfig("block1", logConfig("cfg"))

			store.removeLogBlockConfig("block1")

			expect(store.logBlockConfigs).toEqual({ doc1: { branch1: {} } })
		})

		it("removes the config for an explicitly given document and branch", ({
			expect,
		}) => {
			const store = makeActiveStore()
			store.setLogBlockConfig("block1", logConfig("cfg"))
			store.updateActiveBranchId("branch2")

			store.removeLogBlockConfig("block1", "doc1", "branch1")

			expect(store.logBlockConfigs).toEqual({ doc1: { branch1: {} } })
		})

		it("ignores a block without a stored config", ({ expect }) => {
			const store = makeActiveStore()

			store.removeLogBlockConfig("block1")

			expect(store.logBlockConfigs).toEqual({})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.removeLogBlockConfig("block1")
			}).toThrow("Cannot remove log block config without an active document")
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.removeLogBlockConfig("block1")
			}).toThrow("Cannot remove log block config without an active branch")
		})
	})

	describe("setLogBlockDiffInfo", () => {
		it("stores the diff status and the old config", ({ expect }) => {
			const store = makeActiveStore()

			store.setLogBlockDiffInfo("block1", DiffStatus.Modified, logConfig("old"))

			expect(store.logBlockDiffStatuses).toEqual({
				doc1: { branch1: { block1: DiffStatus.Modified } },
			})
			expect(store.logBlockOldConfigs).toEqual({
				doc1: { branch1: { block1: logConfig("old") } },
			})
		})

		it("stores a null old config for an added block", ({ expect }) => {
			const store = makeActiveStore()

			store.setLogBlockDiffInfo("block1", DiffStatus.Added, null)

			expect(store.logBlockOldConfigs).toEqual({
				doc1: { branch1: { block1: null } },
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.setLogBlockDiffInfo("block1", DiffStatus.Added, null)
			}).toThrow("Cannot set log block diff info without an active document")
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.setLogBlockDiffInfo("block1", DiffStatus.Added, null)
			}).toThrow("Cannot set log block diff info without an active branch")
		})
	})

	describe("removeLogBlockDiffInfo", () => {
		it("removes the diff status and the old config", ({ expect }) => {
			const store = makeActiveStore()
			store.setLogBlockDiffInfo("block1", DiffStatus.Modified, logConfig("old"))

			store.removeLogBlockDiffInfo("block1")

			expect(store.logBlockDiffStatuses).toEqual({ doc1: { branch1: {} } })
			expect(store.logBlockOldConfigs).toEqual({ doc1: { branch1: {} } })
		})

		it("removes the diff info only for the explicitly given document and branch", ({
			expect,
		}) => {
			// switching the branch clears all diff data, so the info can only
			// live under the branch that is active when it is set
			const store = makeActiveStore()
			store.updateActiveBranchId("branch2")
			store.setLogBlockDiffInfo("block1", DiffStatus.Modified, null)

			store.removeLogBlockDiffInfo("block1", "doc1", "branch1")

			expect(store.logBlockDiffStatuses).toEqual({
				doc1: { branch2: { block1: DiffStatus.Modified } },
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.removeLogBlockDiffInfo("block1")
			}).toThrow("Cannot remove log block diff info without an active document")
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.removeLogBlockDiffInfo("block1")
			}).toThrow("Cannot remove log block diff info without an active branch")
		})
	})

	describe("activateLogBlockConfig", () => {
		it("sets the active config uid", ({ expect }) => {
			const store = makeStore()

			store.activateLogBlockConfig("uid1")

			expect(store.activeLogBlockConfig).toBe("uid1")
		})

		it("clears the active config uid", ({ expect }) => {
			const store = makeStore()
			store.activateLogBlockConfig("uid1")

			store.activateLogBlockConfig(null)

			expect(store.activeLogBlockConfig).toBeNull()
		})
	})

	describe("setLogBlockNextRefreshTimestamp", () => {
		it("schedules the next refresh after the given interval", ({ expect }) => {
			withFakeNow(() => {
				const store = makeActiveStore()

				store.setLogBlockNextRefreshTimestamp("block1", 5000)

				expect(store.isLogBlockDueForRefresh("block1")).toBe(false)

				vi.advanceTimersByTime(5000)

				expect(store.isLogBlockDueForRefresh("block1")).toBe(true)
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.setLogBlockNextRefreshTimestamp("block1", 5000)
			}).toThrow(
				"Cannot set log block next refresh timestamp without an active document",
			)
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.setLogBlockNextRefreshTimestamp("block1", 5000)
			}).toThrow(
				"Cannot set log block next refresh timestamp without an active branch",
			)
		})
	})

	describe("isLogBlockDueForRefresh", () => {
		it("reports due when no refresh is scheduled", ({ expect }) => {
			const store = makeActiveStore()

			expect(store.isLogBlockDueForRefresh("block1")).toBe(true)
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => store.isLogBlockDueForRefresh("block1")).toThrow(
				"Cannot check log block refresh status without an active document",
			)
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => store.isLogBlockDueForRefresh("block1")).toThrow(
				"Cannot check log block refresh status without an active branch",
			)
		})
	})

	describe("clearLogBlockNextRefreshTimestamp", () => {
		it("makes the block due again", ({ expect }) => {
			withFakeNow(() => {
				const store = makeActiveStore()
				store.setLogBlockNextRefreshTimestamp("block1", 5000)

				store.clearLogBlockNextRefreshTimestamp("block1")

				expect(store.isLogBlockDueForRefresh("block1")).toBe(true)
			})
		})

		it("throws without an active document", ({ expect }) => {
			const store = makeStore()

			expect(() => {
				store.clearLogBlockNextRefreshTimestamp("block1")
			}).toThrow(
				"Cannot clear log block refresh timestamp without an active document",
			)
		})

		it("throws without an active branch", ({ expect }) => {
			const store = makeStore()
			store.updateActiveDocumentId("doc1")

			expect(() => {
				store.clearLogBlockNextRefreshTimestamp("block1")
			}).toThrow(
				"Cannot clear log block refresh timestamp without an active branch",
			)
		})
	})

	describe("updateLastDragDropTimestamp", () => {
		it("marks a drag drop as just happened", ({ expect }) => {
			withFakeNow(() => {
				const store = makeStore()

				store.updateLastDragDropTimestamp()

				expect(store.isLastDragDropRecent()).toBe(true)
			})
		})
	})

	describe("isLastDragDropRecent", () => {
		it("reports false before any drag drop", ({ expect }) => {
			const store = makeStore()

			expect(store.isLastDragDropRecent()).toBe(false)
		})

		it("reports true at the threshold boundary", ({ expect }) => {
			withFakeNow(() => {
				const store = makeStore()
				store.updateLastDragDropTimestamp()

				vi.advanceTimersByTime(2000)

				expect(store.isLastDragDropRecent()).toBe(true)
			})
		})

		it("reports false past the threshold", ({ expect }) => {
			withFakeNow(() => {
				const store = makeStore()
				store.updateLastDragDropTimestamp()

				vi.advanceTimersByTime(2001)

				expect(store.isLastDragDropRecent()).toBe(false)
			})
		})

		it("honors a custom threshold", ({ expect }) => {
			withFakeNow(() => {
				const store = makeStore()
				store.updateLastDragDropTimestamp()

				vi.advanceTimersByTime(500)

				expect(store.isLastDragDropRecent(499)).toBe(false)
				expect(store.isLastDragDropRecent(500)).toBe(true)
			})
		})
	})

	describe("updateMainAreaWidth", () => {
		it("makes compact view available once the main area exceeds the cap", ({
			expect,
		}) => {
			const store = makeStore()

			store.updateMainAreaWidth(1281)

			expect(store.mainAreaWidth).toBe(1281)
			expect(store.compactViewAvailable).toBe(true)
		})

		it("keeps compact view unavailable at the cap", ({ expect }) => {
			const store = makeStore()

			store.updateMainAreaWidth(1280)

			expect(store.compactViewAvailable).toBe(false)
		})
	})

	describe("toggleAiAssistantOpen", () => {
		it("opens the assistant when closed", ({ expect }) => {
			const store = makeStore()

			store.toggleAiAssistantOpen()

			expect(store.aiAssistantOpen).toBe(true)
		})

		it("closes the assistant when open", ({ expect }) => {
			const store = makeStore()
			store.toggleAiAssistantOpen()

			store.toggleAiAssistantOpen()

			expect(store.aiAssistantOpen).toBe(false)
		})
	})

	describe("setMermaidBlockShowCode", () => {
		it("records code visibility per block", ({ expect }) => {
			const store = makeStore()

			store.setMermaidBlockShowCode("block1", true)
			store.setMermaidBlockShowCode("block2", false)

			expect(store.mermaidBlockShowCode).toEqual({
				block1: true,
				block2: false,
			})
		})
	})
})
