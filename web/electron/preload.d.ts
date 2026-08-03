import type { authClient } from "./auth-client"

// Better Auth's setupRenderer() puts these on window in the renderer. Typing
// them in electron-scope keeps the preload entry honest.
declare global {
	type Bridges = typeof authClient.$Infer.Bridges
	// eslint-disable-next-line @typescript-eslint/no-empty-object-type
	interface Window extends Bridges {}
}

export {}
