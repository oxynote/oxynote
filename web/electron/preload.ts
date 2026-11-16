import { contextBridge, ipcRenderer } from "electron"
import type { IpcRendererEvent } from "electron"
import { setupRenderer } from "@better-auth/electron/preload"

// Exposes Better Auth's renderer bridges (window.requestAuth, window.signOut,
// window.onAuthenticated, etc.) over the IPC channel set up by setupMain().
setupRenderer()

// In hybrid dev builds (DESKTOP_BUILD=hybrid) the same Nuxt dev server is hit
// by both the electron renderer and the system browser opened for OAuth. This
// flag is the unambiguous runtime signal that lets `__DESKTOP_BUILD__`'s
// hybrid-mode probe (see nuxt.config.ts) distinguish the two — only present
// here, never set by a regular browser.
contextBridge.exposeInMainWorld("__isElectron", true)

type OsType = "macOS" | "windows" | "linux" | "other"

const platformToOsType = (p: NodeJS.Platform): OsType => {
	switch (p) {
		case "darwin":
			return "macOS"
		case "win32":
			return "windows"
		case "linux":
			return "linux"
		default:
			return "other"
	}
}

const invokeAuth = (channel: string) => (args?: unknown) =>
	ipcRenderer.invoke(channel, args)

// progress events of every download share one channel, so each request
// carries an id its listener filters on
let nextDownloadId = 0

async function downloadFile(
	url: string,
	name: string,
	onProgress: (received: number, total: number) => void,
): Promise<unknown> {
	nextDownloadId++
	const id = nextDownloadId

	const listener = (
		_event: IpcRendererEvent,
		downloadId: number,
		received: number,
		total: number,
	) => {
		if (downloadId === id) {
			onProgress(received, total)
		}
	}

	ipcRenderer.on("file:download-progress", listener)

	try {
		return await ipcRenderer.invoke("file:download", { id, url, name })
	} finally {
		ipcRenderer.removeListener("file:download-progress", listener)
	}
}

contextBridge.exposeInMainWorld("__host", {
	osType: platformToOsType(process.platform),
	openExternal: (url: string) => ipcRenderer.invoke("shell:openExternal", url),
	// main asks where to save and streams the file there, so the renderer
	// never holds its bytes. See electron/file-download.ts.
	files: {
		download: downloadFile,
	},
	// Auth ops the renderer is allowed to invoke. Every key here matches an
	// ipcMain.handle registration in electron/auth-ipc.ts. The renderer can
	// never touch the session cookie directly — it only sees results.
	auth: {
		getSession: invokeAuth("auth:getSession"),
		signInEmailPassword: invokeAuth("auth:signInEmailPassword"),
		signUpEmailPassword: invokeAuth("auth:signUpEmailPassword"),
		requestPasswordReset: invokeAuth("auth:requestPasswordReset"),
		changePassword: invokeAuth("auth:changePassword"),
		listAccounts: invokeAuth("auth:listAccounts"),
		updateUser: invokeAuth("auth:updateUser"),
		changeEmail: invokeAuth("auth:changeEmail"),
		deleteUser: invokeAuth("auth:deleteUser"),
		getFullOrganization: invokeAuth("auth:getFullOrganization"),
		checkOrganizationSlug: invokeAuth("auth:checkOrganizationSlug"),
		createOrganization: invokeAuth("auth:createOrganization"),
		setActiveOrganization: invokeAuth("auth:setActiveOrganization"),
		acceptOrganizationInvitation: invokeAuth(
			"auth:acceptOrganizationInvitation",
		),
		updateOrganization: invokeAuth("auth:updateOrganization"),
		inviteOrganizationMember: invokeAuth("auth:inviteOrganizationMember"),
		cancelOrganizationInvitation: invokeAuth(
			"auth:cancelOrganizationInvitation",
		),
		removeOrganizationMember: invokeAuth("auth:removeOrganizationMember"),
		updateOrganizationMemberRole: invokeAuth(
			"auth:updateOrganizationMemberRole",
		),
	},
})
