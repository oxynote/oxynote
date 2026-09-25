// the refusals core answers a hook write with, keyed by error code. A 422
// names the status the hook would start in, a 400 the setting it rejects,
// a 424 a service the hook could not reach.
const HOOK_ERROR_KEYS: Record<string, string> = {
	"document_hook.unconfigured": "editor.hooks.errors.codes.unconfigured",
	"document_hook.missing_installation":
		"editor.hooks.errors.codes.missing-installation",
	"document_hook.missing_repository":
		"editor.hooks.errors.codes.missing-repository",
	"document_hook.missing_branch": "editor.hooks.errors.codes.missing-branch",
	"document_hook.tree_truncated": "editor.hooks.errors.codes.tree-truncated",
	"document_hook.unauthorized": "editor.hooks.errors.codes.unauthorized",
	"document_hook.image_not_found": "editor.hooks.errors.codes.image-not-found",
	"document_hook.invalid_url": "editor.hooks.errors.codes.invalid-url",
	"document_hook.invalid_repository":
		"editor.hooks.errors.codes.invalid-repository",
	"document_hook.missing_paths": "editor.hooks.errors.codes.missing-paths",
	"registry.invalid_reference": "editor.hooks.errors.codes.invalid-image",
	"document_hook.upstream_unavailable":
		"editor.hooks.errors.codes.upstream-unavailable",
}

// hookErrorKey returns the i18n key describing why a hook write failed,
// or undefined when the error names no known reason.
export function hookErrorKey(err: unknown): string | undefined {
	if (!err || typeof err !== "object") {
		return undefined
	}

	const code =
		"data" in err &&
		err.data &&
		typeof err.data === "object" &&
		"code" in err.data &&
		typeof err.data.code === "string"
			? err.data.code
			: undefined

	return code ? HOOK_ERROR_KEYS[code] : undefined
}
