import type { FetchError } from "ofetch"
import { showToastMessage } from "~/components/toast"

export const StorageErrorCode = {
	InvalidContentType: "storage.invalid_content_type",
	SizeLimitExceeded: "storage.size_limit_exceeded",
} as const

export type StorageErrorCode =
	(typeof StorageErrorCode)[keyof typeof StorageErrorCode]

interface StorageErrorResponse {
	code: string
}

function isStorageErrorResponse(data: unknown): data is StorageErrorResponse {
	return (
		typeof data === "object" &&
		data !== null &&
		"code" in data &&
		typeof (data as StorageErrorResponse).code === "string"
	)
}

function isFetchError(error: unknown): error is FetchError {
	return (
		typeof error === "object" &&
		error !== null &&
		"data" in error &&
		"statusCode" in error
	)
}

/**
 * Extracts the storage error code from a fetch error, if present.
 * Returns the error code if it's a known storage error, or null otherwise.
 */
export function getStorageErrorCode(error: unknown): StorageErrorCode | null {
	if (!isFetchError(error)) {
		return null
	}

	if (!isStorageErrorResponse(error.data)) {
		return null
	}

	const code = error.data.code

	if (
		code === StorageErrorCode.InvalidContentType ||
		code === StorageErrorCode.SizeLimitExceeded
	) {
		return code
	}

	return null
}

/**
 * Handles storage errors by displaying appropriate toast messages.
 * Returns true if the error was a storage error and was handled,
 * false otherwise (caller should handle with generic error message).
 */
export function handleStorageError(
	error: unknown,
	t: (key: string) => string,
): boolean {
	const code = getStorageErrorCode(error)

	if (!code) {
		return false
	}

	switch (code) {
		case StorageErrorCode.InvalidContentType:
			showToastMessage(
				"error",
				t("storage-errors.invalid-content-type.title"),
				t("storage-errors.invalid-content-type.description"),
			)
			return true
		case StorageErrorCode.SizeLimitExceeded:
			showToastMessage(
				"error",
				t("storage-errors.size-limit-exceeded.title"),
				t("storage-errors.size-limit-exceeded.description"),
			)
			return true
	}
}
