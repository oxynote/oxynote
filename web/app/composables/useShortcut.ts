import type { UseMagicKeysReturn } from "@vueuse/core"

const SHORTCUT_KEY = Symbol("icon-picker")
const MODIFIERS = ["meta", "control", "alt", "shift"]

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
		const parts = combo.split("_")

		// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- useMagicKeys returns a proxy that materializes a ref for any key
		const comboKey = magicKeys[combo]!
		const current = magicKeys.current

		// a combo's ref is true whenever its keys are down, so holding an
		// extra modifier satisfies it too and ⌘⇧\ would fire ⌘\ as well.
		// The shortcut only counts while no modifier outside it is held.
		function exact() {
			return MODIFIERS.every(
				(modifier) => !current.has(modifier) || parts.includes(modifier),
			)
		}

		const stop = whenever(comboKey, () => {
			if (exact()) {
				handler()
			}
		})
		const stopPrevent = useEventListener(window, "keydown", (ev) => {
			if (comboKey.value && exact()) {
				ev.preventDefault()
			}
		})

		onScopeDispose(() => {
			stop()
			stopPrevent()
		})
	}
}
