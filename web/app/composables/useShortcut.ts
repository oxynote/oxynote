import type { UseMagicKeysReturn } from "@vueuse/core"

const SHORTCUT_KEY = Symbol("icon-picker")

// the shortcut/handler are optional; if not provided, the composable is used
// for side effects only (i.e., to provide the magicKeys instance)
export default function (
	shortcut?: { macOS: string; other: string },
	handler?: () => void,
) {
	const { osType } = useDetectHost()

	let magicKeys = inject<UseMagicKeysReturn<false> | null>(SHORTCUT_KEY, null)
	if (!magicKeys) {
		magicKeys = useMagicKeys()
		provide(SHORTCUT_KEY, magicKeys)
	}

	if (shortcut && handler) {
		const combo = normalizeShortcut(shortcut, osType.value)

		const stop = whenever(magicKeys[combo]!, () => {
			handler()
		})
		const stopPrevent = useEventListener(window, "keydown", (ev) => {
			if (magicKeys[combo]!.value) {
				ev.preventDefault()
			}
		})

		onScopeDispose(() => {
			stop()
			stopPrevent()
		})
	}
}
