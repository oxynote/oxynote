import type { APIRequestContext } from "@playwright/test"

// the prod stack's two instances with the rate limiter on
// (oxynote-rate-limited and oxynote-trusted-proxies in
// docker-compose.prod.yaml). Changing a port means changing that service.
export const RATE_LIMITED_URL = "http://localhost:19085"
export const TRUSTED_PROXIES_URL = "http://localhost:19086"

// failedSignIn signs in with a wrong password and returns the status. The
// X-Forwarded-For value is what a proxy in front of the image would send.
export async function failedSignIn(
	request: APIRequestContext,
	origin: string,
	forwardedFor: string,
): Promise<number> {
	const response = await request.post(
		`${origin}/auth-realtime/api/auth/sign-in/email`,
		{
			headers: { Origin: origin, "X-Forwarded-For": forwardedFor },
			data: {
				email: "nobody@example.com",
				password: "wrong-password-1",
			},
		},
	)

	return response.status()
}
