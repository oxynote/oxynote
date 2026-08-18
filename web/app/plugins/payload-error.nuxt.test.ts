import { mockNuxtImport } from "@nuxt/test-utils/runtime"
import { beforeEach, describe, it, vi } from "vitest"
import plugin from "./payload-error"

const { definePayloadReducerMock, definePayloadReviverMock } = vi.hoisted(
	() => {
		return {
			definePayloadReducerMock:
				vi.fn<(name: string, reduce: (value: unknown) => unknown) => void>(),
			definePayloadReviverMock:
				vi.fn<
					(
						name: string,
						revive: (data: Record<string, unknown>) => Error,
					) => void
				>(),
		}
	},
)

mockNuxtImport("definePayloadReducer", () => definePayloadReducerMock)
mockNuxtImport("definePayloadReviver", () => definePayloadReviverMock)

function registerHandlers() {
	void plugin(useNuxtApp())

	const reduce = definePayloadReducerMock.mock.calls[0]?.[1]
	const revive = definePayloadReviverMock.mock.calls[0]?.[1]

	if (!reduce || !revive) {
		throw new Error("the plugin did not register both payload handlers")
	}

	return { reduce, revive }
}

// the tests capture handlers from shared module-level mocks (mockNuxtImport
// singletons), so they cannot interleave
describe("payload-error", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mocks explicitly
	beforeEach(() => {
		definePayloadReducerMock.mockReset()
		definePayloadReviverMock.mockReset()
	})

	it("registers the error reducer and reviver under one type name", ({
		expect,
	}) => {
		void plugin(useNuxtApp())

		expect(definePayloadReducerMock).toHaveBeenCalledExactlyOnceWith(
			"ErrorObject",
			expect.any(Function),
		)
		expect(definePayloadReviverMock).toHaveBeenCalledExactlyOnceWith(
			"ErrorObject",
			expect.any(Function),
		)
	})

	describe("reducer", () => {
		it("serializes an error into a plain object", ({ expect }) => {
			const { reduce } = registerHandlers()

			// the vitest nuxt environment is a non-dev build, so the stack is
			// dropped along with the production behaviour
			expect(reduce(new TypeError("boom"))).toEqual({
				name: "TypeError",
				message: "boom",
				stack: undefined,
				cause: undefined,
			})
		})

		it("keeps an error cause", ({ expect }) => {
			const { reduce } = registerHandlers()

			const error = new TypeError("boom", { cause: new RangeError("root") })

			expect(reduce(error)).toEqual({
				name: "TypeError",
				message: "boom",
				stack: undefined,
				cause: { name: "RangeError", message: "root" },
			})
		})

		it("drops a non-error cause", ({ expect }) => {
			const { reduce } = registerHandlers()

			const error = new Error("boom", { cause: "just text" })

			expect(reduce(error)).toEqual({
				name: "Error",
				message: "boom",
				stack: undefined,
				cause: undefined,
			})
		})

		it.for([
			{ name: "a plain object", value: {} },
			{ name: "a string", value: "boom" },
			{ name: "null", value: null },
		])("ignores $name", ({ value }, { expect }) => {
			const { reduce } = registerHandlers()

			expect(reduce(value)).toBeUndefined()
		})
	})

	describe("reviver", () => {
		it("revives a plain object into an error instance", ({ expect }) => {
			const { revive } = registerHandlers()

			const revived = revive({ name: "TypeError", message: "boom" })

			expect(revived).toBeInstanceOf(Error)
			expect(revived.name).toBe("TypeError")
			expect(revived.message).toBe("boom")
			expect(revived.cause).toBeUndefined()
		})

		it("restores the stack when present", ({ expect }) => {
			const { revive } = registerHandlers()

			const revived = revive({
				name: "Error",
				message: "boom",
				stack: "fake-stack",
			})

			expect(revived.stack).toBe("fake-stack")
		})

		it("restores an error cause with its name", ({ expect }) => {
			const { revive } = registerHandlers()

			const revived = revive({
				name: "Error",
				message: "boom",
				cause: { name: "RangeError", message: "root" },
			})

			const cause = revived.cause as Error
			expect(cause).toBeInstanceOf(Error)
			expect(cause.name).toBe("RangeError")
			expect(cause.message).toBe("root")
		})

		it("defaults the cause name when it is missing", ({ expect }) => {
			const { revive } = registerHandlers()

			const revived = revive({
				name: "Error",
				message: "boom",
				cause: { message: "root" },
			})

			expect((revived.cause as Error).name).toBe("Error")
		})
	})
})
