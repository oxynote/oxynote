import { beforeEach, describe, it, vi } from "vitest"
import {
	getStorageErrorCode,
	handleStorageError,
	StorageErrorCode,
} from "./file-utils"

const { showToastMessageMock } = vi.hoisted(() => {
	return { showToastMessageMock: vi.fn() }
})

vi.mock("~/components/toast", () => {
	return { showToastMessage: showToastMessageMock }
})

// the minimal shape getStorageErrorCode's fetch error guard looks for
function fetchError(data: unknown) {
	return { data, statusCode: 400 }
}

describe("getStorageErrorCode", () => {
	it.for([
		{ name: "returns null for a null error", error: null },
		{ name: "returns null for a string error", error: "boom" },
		{
			name: "returns null for an object without fetch error fields",
			error: { message: "boom" },
		},
		{
			name: "returns null when the response data is not an object",
			error: fetchError("boom"),
		},
		{
			name: "returns null when the response data has no code",
			error: fetchError({ message: "boom" }),
		},
		{
			name: "returns null when the code is not a string",
			error: fetchError({ code: 5 }),
		},
		{
			name: "returns null for an unknown storage code",
			error: fetchError({ code: "storage.unknown" }),
		},
	])("$name", ({ error }, { expect }) => {
		expect(getStorageErrorCode(error)).toBeNull()
	})

	it.for([
		{ code: StorageErrorCode.InvalidContentType },
		{ code: StorageErrorCode.SizeLimitExceeded },
	])("extracts the $code code from a fetch error", ({ code }, { expect }) => {
		expect(getStorageErrorCode(fetchError({ code }))).toBe(code)
	})
})

// the tests assert call accounting on the shared module-level toast mock,
// so they cannot interleave
describe("handleStorageError", { concurrent: false }, () => {
	// restoreMocks does not touch hand-made vi.fn() singletons in vitest 4
	// — reset the module-level mock explicitly
	beforeEach(() => {
		showToastMessageMock.mockReset()
	})

	const t = (key: string) => `t:${key}`

	it("returns false without a toast for a non-storage error", ({ expect }) => {
		const handled = handleStorageError(new Error("boom"), t)

		expect(handled).toBe(false)
		expect(showToastMessageMock).toHaveBeenCalledTimes(0)
	})

	it.for([
		{
			code: StorageErrorCode.InvalidContentType,
			messageKey: "storage-errors.invalid-content-type",
		},
		{
			code: StorageErrorCode.SizeLimitExceeded,
			messageKey: "storage-errors.size-limit-exceeded",
		},
	])(
		"shows the error toast for the $code code",
		({ code, messageKey }, { expect }) => {
			const handled = handleStorageError(fetchError({ code }), t)

			expect(handled).toBe(true)
			expect(showToastMessageMock).toHaveBeenCalledExactlyOnceWith(
				"error",
				`t:${messageKey}.title`,
				`t:${messageKey}.description`,
			)
		},
	)
})
