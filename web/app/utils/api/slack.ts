export interface SlackConnectionStatus {
	connected: boolean
	configured: boolean
}

export interface SlackInstallResponse {
	url: string
}

export interface SlackUserLinkSettings {
	notifications: boolean
}
