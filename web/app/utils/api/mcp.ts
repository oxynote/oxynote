// better-auth's OAuth consent record, as returned by
// /api/auth/oauth2/get-consents.
export interface OAuthConsent {
	id: string
	clientId: string
	scopes: string[]
	createdAt: string
	updatedAt?: string
}

// publicly visible OAuth client fields, as returned by
// /api/auth/oauth2/public-client (OAuth wire naming).
export interface OAuthPublicClient {
	client_id: string
	client_name?: string
	client_uri?: string
	logo_uri?: string
}

// the consent endpoint's answer: better-auth responds with a JSON
// redirect envelope ({redirect, url}) when called over fetch; the
// redirect_uri key covers the documented response shape.
export interface OAuthConsentDecision {
	redirect?: boolean
	url?: string
	redirect_uri?: string
}
