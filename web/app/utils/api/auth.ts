export type AuthMethod = "github" | "google" | "slack"

export interface AuthConfig {
	methods: AuthMethod[]
}
