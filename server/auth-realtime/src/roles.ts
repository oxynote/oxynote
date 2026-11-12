import { createAccessControl } from "better-auth/plugins/access"
import {
	adminAc,
	defaultStatements,
	memberAc,
} from "better-auth/plugins/organization/access"

// a workspace member's role. The web app and core keep the same three
// roles.
const ROLES = ["admin", "editor", "viewer"] as const

export type Role = (typeof ROLES)[number]

// the role that manages the workspace. better-auth gives it to whoever
// creates the workspace and keeps at least one member holding it.
export const ADMIN_ROLE = "admin" satisfies Role

// the role that changes content but not the workspace.
const EDITOR_ROLE = "editor" satisfies Role

export const ac = createAccessControl(defaultStatements)

// editor and viewer differ only outside better-auth, in core and in the
// document connection. Inside better-auth neither may manage the
// workspace, its members or its invitations.
export const roles = {
	admin: ac.newRole(adminAc.statements),
	editor: ac.newRole(memberAc.statements),
	viewer: ac.newRole(memberAc.statements),
}

export function isRole(value: unknown): value is Role {
	return ROLES.includes(value as Role)
}

// canEdit reports whether a stored role may change content. An unknown
// role, or none, only reads.
export function canEdit(role: unknown): boolean {
	return role === ADMIN_ROLE || role === EDITOR_ROLE
}
