export interface AuthError {
	message?: string | undefined
	status: number
	statusText: string
	code?: string | undefined
}

export interface AuthResponse {
	error?: AuthError | null
}

export function postAuthDocumentUrl(
	baseUrl: string | undefined,
	next: string | undefined,
	defaultPath = "/",
): string {
	return `${baseUrl || ""}${next ? decodeURIComponent(next) : defaultPath}`
}

// the accept page renders from these parameters before the invitee has a
// session. auth-realtime builds the same link for the invitation email
// (invitationLink in server/auth-realtime/src/auth.ts).
export function acceptInvitationUrl(
	baseUrl: string | undefined,
	invitation: {
		id: string
		email: string
		inviter: string
		organizationName: string
		organizationId: string
	},
): string {
	const query = new URLSearchParams({
		id: invitation.id,
		email: invitation.email,
		inviter: invitation.inviter,
		orgName: invitation.organizationName,
		orgId: invitation.organizationId,
	})

	return `${baseUrl || ""}/accept-invite?${query.toString()}`
}

// whether a login or signup page was opened on the way to accepting an
// invitation. The next path arrives URI-encoded, and a malformed one is no
// invitation.
export function isInvitationRedirect(next: unknown): boolean {
	if (typeof next !== "string") {
		return false
	}

	try {
		return decodeURIComponent(next).startsWith("/accept-invite")
	} catch {
		return false
	}
}

export function postEmailVerificationUrl(
	baseUrl: string | undefined,
	email: string,
): string {
	return `${baseUrl || ""}/verify-email?new=${encodeURIComponent(email)}`
}
