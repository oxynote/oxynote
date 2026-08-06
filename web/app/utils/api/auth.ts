export type AuthMethod = "github" | "google" | "slack"

export interface AuthMethods {
	methods: AuthMethod[]
}
