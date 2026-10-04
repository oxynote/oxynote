// the message key for each code core refuses a github tracking hook with.
export const GITHUB_TRACKING_ERROR_KEYS: Record<string, string> = {
	"document_hook.missing_installation":
		"editor.hooks.errors.codes.missing-installation",
	"document_hook.missing_repository":
		"editor.hooks.errors.codes.missing-repository",
	"document_hook.missing_branch": "editor.hooks.errors.codes.missing-branch",
	"document_hook.tree_truncated": "editor.hooks.errors.codes.tree-truncated",
	"document_hook.invalid_repository":
		"editor.hooks.errors.codes.invalid-repository",
	"document_hook.missing_paths": "editor.hooks.errors.codes.missing-paths",
}
