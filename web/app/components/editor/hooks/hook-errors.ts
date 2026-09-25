// the refusals core answers a hook write with, keyed by error code.
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
	"document_hook.invalid_image": "editor.hooks.errors.codes.invalid-image",
	"document_hook.upstream_unavailable":
		"editor.hooks.errors.codes.upstream-unavailable",
}

// hookErrorMessage returns the message for the code core refused a hook
// write with, or undefined when the code names no known reason.
export function hookErrorMessage(
	err: unknown,
	i18n: Pick<ReturnType<typeof useI18n>, "t">,
): string | undefined {
	const code = (err as { data?: { code?: string } } | undefined)?.data?.code
	const key = code ? HOOK_ERROR_KEYS[code] : undefined

	return key ? i18n.t(key) : undefined
}
