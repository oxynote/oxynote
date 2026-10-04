// the message key for each code core refuses a container image watcher
// hook with.
export const CONTAINER_IMAGE_WATCHER_ERROR_KEYS: Record<string, string> = {
	"document_hook.unauthorized": "editor.hooks.errors.codes.unauthorized",
	"document_hook.image_not_found": "editor.hooks.errors.codes.image-not-found",
	"document_hook.invalid_image": "editor.hooks.errors.codes.invalid-image",
}
