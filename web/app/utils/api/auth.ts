export type AuthMethod = "email-password" | "github" | "google" | "slack"

export interface AuthConfig {
	methods: AuthMethod[]
}
