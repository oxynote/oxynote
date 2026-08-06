import type { $Fetch } from "ofetch"
import type { AuthClient } from "~/plugins/02.auth"

declare module "#app" {
	interface NuxtApp {
		$authClient: AuthClient
		$coreAPIClient: $Fetch
		$authRealtimeAPIClient: $Fetch
	}
}

declare module "vue" {
	interface ComponentCustomProperties {
		$authClient: AuthClient
		$coreAPIClient: $Fetch
		$authRealtimeAPIClient: $Fetch
	}
}

declare module "nuxt/schema" {
	interface PageMeta {
		skipAuth?: boolean
	}
}

export {}
