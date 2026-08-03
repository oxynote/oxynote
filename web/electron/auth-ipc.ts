import { ipcMain } from "electron"
import { authClient } from "./auth-client"

// Whitelist of auth operations exposed to the renderer. Each handler runs in
// main with the electron-store-backed session; the renderer never sees the
// session cookie, only the operation's result. Adding a new auth op here is
// the only way to expose it to the renderer.
const handlers: Record<string, (args: any) => Promise<unknown>> = {
	"auth:getSession": () => authClient.getSession(),
	"auth:updateUser": (args) => authClient.updateUser(args),
	"auth:changeEmail": (args) => authClient.changeEmail(args),
	"auth:deleteUser": (args) => authClient.deleteUser(args),
	"auth:getFullOrganization": () => authClient.organization.getFullOrganization(),
	"auth:checkOrganizationSlug": (args) => authClient.organization.checkSlug(args),
	"auth:createOrganization": (args) => authClient.organization.create(args),
	"auth:setActiveOrganization": (args) => authClient.organization.setActive(args),
	"auth:acceptOrganizationInvitation": (args) =>
		authClient.organization.acceptInvitation(args),
	"auth:updateOrganization": (args) => authClient.organization.update(args),
	"auth:inviteOrganizationMember": (args) =>
		authClient.organization.inviteMember(args),
	"auth:cancelOrganizationInvitation": (args) =>
		authClient.organization.cancelInvitation(args),
	"auth:removeOrganizationMember": (args) =>
		authClient.organization.removeMember(args),
}

export function registerAuthIpcHandlers() {
	for (const [channel, handler] of Object.entries(handlers)) {
		ipcMain.handle(channel, (_event, args) => handler(args))
	}
}
