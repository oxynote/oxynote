type WebPersistentStorage = "cookie" | "local-storage"

type DesktopPersistentStorage = "local-storage"

interface PersistentStateSerializer<T> {
	read: (raw: string) => T
	write: (value: T) => string
}

interface PersistentCookieOptions {
	path?: string
	maxAge?: number
	expires?: Date
	domain?: string
	sameSite?: "lax" | "strict" | "none"
	secure?: boolean
}

export interface PersistentStateOptions<T> {
	key: string
	defaultValue: T | (() => T)
	storage?: {
		web?: WebPersistentStorage
		desktop?: DesktopPersistentStorage
	}
	watch?: boolean | "shallow"
	serializer?: PersistentStateSerializer<T>
	cookie?: PersistentCookieOptions
}

export default function <T>(opts: PersistentStateOptions<T>) {
	const { isWeb } = useDetectHost()

	const serializer = opts.serializer

	const defaultValue =
		typeof opts.defaultValue === "function"
			? (opts.defaultValue as () => T)()
			: opts.defaultValue

	const storage = isWeb.value
		? (opts.storage?.web ?? "cookie")
		: (opts.storage?.desktop ?? "local-storage")

	if (storage === "cookie") {
		return useCookie<T>(opts.key, {
			default: () => defaultValue,
			// only pass watch when explicitly specified; otherwise let useCookie
			// use its default (true), which is required for setting .value to
			// actually persist to document.cookie on the client
			...(opts.watch !== undefined ? { watch: opts.watch } : {}),
			path: opts.cookie?.path ?? "/",
			maxAge: opts.cookie?.maxAge,
			expires: opts.cookie?.expires,
			domain: opts.cookie?.domain,
			sameSite: opts.cookie?.sameSite,
			secure: opts.cookie?.secure,
			...(serializer
				? {
						encode: (value) => serializer.write(value),
						decode: (raw) => {
							if (raw == null) return defaultValue
							try {
								return serializer.read(raw)
							} catch {
								return defaultValue
							}
						},
					}
				: {}),
		})
	}

	return useLocalStorage<T>(opts.key, defaultValue, {
		serializer: serializer
			? {
					read: (raw) => {
						try {
							return serializer.read(raw)
						} catch {
							return defaultValue
						}
					},
					write: (value) => serializer.write(value),
				}
			: undefined,
		shallow: opts.watch === "shallow",
	})
}
