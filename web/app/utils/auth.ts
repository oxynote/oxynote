export interface AuthError {
	message?: string | undefined
	status: number
	statusText: string
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

export function postEmailVerificationUrl(
	baseUrl: string | undefined,
	email: string,
): string {
	return `${baseUrl || ""}/verify-email?new=${encodeURIComponent(email)}`
}
