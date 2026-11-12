import { createAccessControl } from "better-auth/plugins/access"
import {
	adminAc,
	defaultStatements,
	memberAc,
} from "better-auth/plugins/organization/access"

// a workspace member's role. The auth service owns the list, and core
// keeps the same three roles.
export const ROLES = ["admin", "editor", "viewer"] as const

export type Role = (typeof ROLES)[number]

export function isRole(value: unknown): value is Role {
	return ROLES.includes(value as Role)
}

export const ac = createAccessControl(defaultStatements)

// the auth client needs the roles to type a role argument.
export const roles = {
	admin: ac.newRole(adminAc.statements),
	editor: ac.newRole(memberAc.statements),
	viewer: ac.newRole(memberAc.statements),
}
