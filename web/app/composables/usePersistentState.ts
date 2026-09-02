import type { NuxtApp } from "#app"
import { effectScope } from "vue"

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

// one state per key, for the whole app: useCookie() and useLocalStorage()
// each hand back an independent ref, so two callers of one key drift apart
// — and on the server they race to write the same response cookie. The map
// hangs off the nuxt app instance, which is per request during SSR.
const statesByApp = new WeakMap<NuxtApp, Map<string, unknown>>()

export default function <T>(opts: PersistentStateOptions<T>) {
	const nuxtApp = useNuxtApp()
	const { isWeb } = useDetectHost()

	const states = statesByApp.get(nuxtApp) ?? new Map<string, unknown>()
	statesByApp.set(nuxtApp, states)

	const serializer = opts.serializer

	const defaultValue =
		typeof opts.defaultValue === "function"
			? (opts.defaultValue as () => T)()
			: opts.defaultValue

	const storage = isWeb.value
		? (opts.storage?.web ?? "cookie")
		: (opts.storage?.desktop ?? "local-storage")

	const createState = () => {
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

	const existing = states.get(opts.key)
	if (existing) {
		return existing as ReturnType<typeof createState>
	}

	// created detached from whichever component asked first: both refs
	// persist through a watcher bound to the active effect scope, and the
	// shared state has to keep writing after that component unmounts
	effectScope(true).run(() => {
		states.set(opts.key, createState())
	})

	return states.get(opts.key) as ReturnType<typeof createState>
}
