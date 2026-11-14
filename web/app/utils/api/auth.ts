export type AuthMethod =
	"email-password" | "github" | "google" | "slack" | "oidc"

// a method that signs in through another service.
export type ProviderAuthMethod = Exclude<AuthMethod, "email-password">

export interface AuthConfig {
	methods: AuthMethod[]
	// the label of the OIDC button, null when OIDC is off.
	oidcDisplayName: string | null
	emailEnabled: boolean
	// if true, the whole instance is one workspace: it exists from the start, and
	// only invited users sign up.
	singleOrganization: boolean
	// null when a workspace takes any number of members.
	maxOrganizationMembers: number | null
	// the account that still has the default admin email. The server finds
	// it by that email. So a new email makes the whole value null. The
	// password is null once the admin changes it. Always null outside
	// single-workspace mode.
	defaultAdmin: { email: string; password: string | null } | null
}
