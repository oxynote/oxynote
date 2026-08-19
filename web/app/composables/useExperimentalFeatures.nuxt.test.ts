import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import useExperimentalFeatures, {
	ExperimentalFeature,
} from "./useExperimentalFeatures"

const { useAuthSessionMock } = vi.hoisted(() => {
	// the default implementation keeps the app bootstrap alive: the
	// websocket plugin refreshes the session query during nuxt init
	return {
		useAuthSessionMock: vi.fn((): any => ({
			fetchAuthSession: {
				refresh: () => Promise.resolve({ data: undefined }),
			},
			fetchOrganization: {
				refresh: () => Promise.resolve({ data: undefined }),
				data: { value: undefined },
			},
		})),
	}
})

mockNuxtImport("useAuthSession", () => useAuthSessionMock)

function arrange(opts: { orgId?: string; features: string }) {
	useAuthSessionMock.mockReturnValue({
		fetchOrganization: {
			data: { value: opts.orgId ? { data: { id: opts.orgId } } : undefined },
		},
	})

	// the runtime config object is mutable at runtime; each test sets the
	// value it asserts against
	useRuntimeConfig().public.experimentalFeatures = opts.features
}

// the tests arrange a shared module-level mock (mockNuxtImport singleton)
// and the shared runtime config, so they cannot interleave
describe("useExperimentalFeatures", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mock explicitly
	beforeEach(() => {
		useAuthSessionMock.mockReset()
	})

	it("enables a feature for a listed organization", ({ expect }) => {
		arrange({ orgId: "org2", features: "ai-assistant:org1, org2" })
		const { isExperimentalFeatureEnabled } = useExperimentalFeatures()

		expect(
			isExperimentalFeatureEnabled(ExperimentalFeature.AIAssistant).value,
		).toBe(true)
	})

	it("disables a feature for an unlisted organization", ({ expect }) => {
		arrange({ orgId: "org3", features: "ai-assistant:org1,org2" })
		const { isExperimentalFeatureEnabled } = useExperimentalFeatures()

		expect(
			isExperimentalFeatureEnabled(ExperimentalFeature.AIAssistant).value,
		).toBe(false)
	})

	it("disables features without an active organization", ({ expect }) => {
		arrange({ features: "ai-assistant:org1" })
		const { isExperimentalFeatureEnabled } = useExperimentalFeatures()

		expect(
			isExperimentalFeatureEnabled(ExperimentalFeature.AIAssistant).value,
		).toBe(false)
	})

	it("disables a feature that is not configured", ({ expect }) => {
		arrange({ orgId: "org1", features: "other-feature:org1" })
		const { isExperimentalFeatureEnabled } = useExperimentalFeatures()

		expect(
			isExperimentalFeatureEnabled(ExperimentalFeature.AIAssistant).value,
		).toBe(false)
	})

	it("disables a feature configured without organizations", ({ expect }) => {
		arrange({ orgId: "org1", features: "ai-assistant" })
		const { isExperimentalFeatureEnabled } = useExperimentalFeatures()

		expect(
			isExperimentalFeatureEnabled(ExperimentalFeature.AIAssistant).value,
		).toBe(false)
	})
})
