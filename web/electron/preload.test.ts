import { beforeEach, describe, expect, it, vi } from "vitest"

const {
	exposeInMainWorldMock,
	invokeMock,
	onMock,
	removeListenerMock,
	setupRendererMock,
} = vi.hoisted(() => ({
	exposeInMainWorldMock: vi.fn(),
	invokeMock: vi.fn(),
	onMock: vi.fn(),
	removeListenerMock: vi.fn(),
	setupRendererMock: vi.fn(),
}))

vi.mock("electron", () => ({
	contextBridge: { exposeInMainWorld: exposeInMainWorldMock },
	ipcRenderer: {
		invoke: invokeMock,
		on: onMock,
		removeListener: removeListenerMock,
	},
}))
vi.mock("@better-auth/electron/preload", () => ({
	setupRenderer: setupRendererMock,
}))

// the renderer-facing auth bridge: every key delegates to the ipc channel
// of the same name registered in auth-ipc.ts
const authKeys = [
	"getSession",
	"signInEmailPassword",
	"signUpEmailPassword",
	"requestPasswordReset",
	"changePassword",
	"listAccounts",
	"updateUser",
	"changeEmail",
	"deleteUser",
	"getFullOrganization",
	"checkOrganizationSlug",
	"createOrganization",
	"setActiveOrganization",
	"acceptOrganizationInvitation",
	"updateOrganization",
	"inviteOrganizationMember",
	"cancelOrganizationInvitation",
	"removeOrganizationMember",
	"updateOrganizationMemberRole",
]

interface Host {
	osType: string
	openExternal: (url: string) => Promise<unknown>
	files: {
		download: (
			url: string,
			name: string,
			onProgress: (received: number, total: number) => void,
		) => Promise<unknown>
	}
	auth: Record<string, (args?: unknown) => Promise<unknown>>
}

type ProgressListener = (
	event: unknown,
	id: number,
	received: number,
	total: number,
) => void

function progressListener(): ProgressListener {
	const call = onMock.mock.calls[0] as [string, ProgressListener] | undefined

	if (!call) {
		throw new Error("no progress listener was registered")
	}

	return call[1]
}

// the unit's act is its module evaluation, so each test re-imports it
// against freshly reset module-level mocks
async function importPreload(): Promise<void> {
	vi.resetModules()
	await import("./preload")
}

// module evaluation reads process.platform once, so the platform cases
// stub it for the duration of the import
async function importPreloadOnPlatform(platform: string): Promise<void> {
	const original = Object.getOwnPropertyDescriptor(process, "platform")

	if (!original) {
		throw new Error("process.platform descriptor is missing")
	}

	Object.defineProperty(process, "platform", {
		value: platform,
		configurable: true,
	})

	try {
		await importPreload()
	} finally {
		Object.defineProperty(process, "platform", original)
	}
}

function exposedHost(): Host {
	const call = exposeInMainWorldMock.mock.calls.find(
		([key]) => key === "__host",
	)

	if (!call) {
		throw new Error("__host was not exposed")
	}

	return call[1] as Host
}

// sequential by exception: these tests assert call accounting on shared
// module-level mocks (vi.mock singletons), which cannot be isolated
// across concurrently interleaving tests
describe("preload", { concurrent: false }, () => {
	// restoreMocks only covers vi.spyOn spies, so these hand-made vi.fn
	// singletons are reset here explicitly
	beforeEach(() => {
		exposeInMainWorldMock.mockClear()
		invokeMock.mockReset()
		onMock.mockReset()
		removeListenerMock.mockReset()
		setupRendererMock.mockClear()
	})

	it("registers the better-auth renderer bridges once", async () => {
		await importPreload()

		expect(setupRendererMock).toHaveBeenCalledTimes(1)
		expect(setupRendererMock).toHaveBeenCalledWith()
	})

	it("exposes only the electron flag and the host bridge", async () => {
		await importPreload()

		expect(exposeInMainWorldMock).toHaveBeenCalledTimes(2)
		expect(exposeInMainWorldMock).toHaveBeenCalledWith("__isElectron", true)
		expect(exposeInMainWorldMock).toHaveBeenCalledWith(
			"__host",
			expect.anything(),
		)
	})

	it.for([
		{ platform: "darwin", expected: "macOS" },
		{ platform: "win32", expected: "windows" },
		{ platform: "linux", expected: "linux" },
		{ platform: "freebsd", expected: "other" },
	])(
		"maps the $platform platform to the $expected os type",
		async ({ platform, expected }, { expect }) => {
			await importPreloadOnPlatform(platform)

			expect(exposedHost().osType).toBe(expected)
		},
	)

	it("delegates openExternal to the shell IPC channel", async () => {
		await importPreload()
		const result = { ok: true }
		invokeMock.mockResolvedValue(result)

		await expect(
			exposedHost().openExternal("https://example.com"),
		).resolves.toBe(result)

		expect(invokeMock).toHaveBeenCalledTimes(1)
		expect(invokeMock).toHaveBeenCalledWith(
			"shell:openExternal",
			"https://example.com",
		)
	})

	describe("files.download", () => {
		const FILE_URL = "http://test.local/core/api/documents/d1/files/f1-a.zip"

		it("asks main to download under a fresh id and resolves with its result", async () => {
			await importPreload()
			invokeMock.mockResolvedValue("completed")
			const onProgress = vi.fn()

			await expect(
				exposedHost().files.download(FILE_URL, "a.zip", onProgress),
			).resolves.toBe("completed")

			expect(invokeMock).toHaveBeenCalledExactlyOnceWith("file:download", {
				id: 1,
				url: FILE_URL,
				name: "a.zip",
			})
			expect(onMock).toHaveBeenCalledExactlyOnceWith(
				"file:download-progress",
				expect.any(Function),
			)
			expect(removeListenerMock).toHaveBeenCalledExactlyOnceWith(
				"file:download-progress",
				progressListener(),
			)
			expect(onProgress).toHaveBeenCalledTimes(0)
		})

		it("numbers each download separately", async () => {
			await importPreload()
			invokeMock.mockResolvedValue("completed")

			await exposedHost().files.download(FILE_URL, "a.zip", vi.fn())
			await exposedHost().files.download(FILE_URL, "a.zip", vi.fn())

			expect(
				invokeMock.mock.calls.map(
					([, request]) => (request as { id: number }).id,
				),
			).toEqual([1, 2])
		})

		it("passes on progress of its own download only", async () => {
			await importPreload()
			const onProgress = vi.fn()
			let finish: (value: unknown) => void = () => undefined
			invokeMock.mockReturnValue(
				new Promise((resolve) => {
					finish = resolve
				}),
			)

			const download = exposedHost().files.download(
				FILE_URL,
				"a.zip",
				onProgress,
			)
			progressListener()({}, 1, 3, 6)
			progressListener()({}, 2, 5, 9)
			finish("completed")
			await download

			expect(onProgress).toHaveBeenCalledExactlyOnceWith(3, 6)
		})

		it("stops listening when the download fails", async () => {
			await importPreload()
			const failure = new Error("boom")
			invokeMock.mockRejectedValue(failure)

			await expect(
				exposedHost().files.download(FILE_URL, "a.zip", vi.fn()),
			).rejects.toBe(failure)

			expect(removeListenerMock).toHaveBeenCalledExactlyOnceWith(
				"file:download-progress",
				progressListener(),
			)
		})
	})

	it("exposes exactly the whitelisted auth operations", async () => {
		await importPreload()

		expect(Object.keys(exposedHost().auth).sort()).toEqual([...authKeys].sort())
	})

	it.for(
		authKeys.map((key) => ({
			name: `delegates auth ${key} to its IPC channel`,
			key,
		})),
	)("$name", async ({ key }, { expect }) => {
		await importPreload()
		const result = { ok: key }
		invokeMock.mockResolvedValue(result)
		const args = { payload: key }

		const bridge = exposedHost().auth[key]

		await expect(bridge(args)).resolves.toBe(result)

		expect(invokeMock).toHaveBeenCalledTimes(1)
		expect(invokeMock).toHaveBeenCalledWith(`auth:${key}`, args)
	})
})
