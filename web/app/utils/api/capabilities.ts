// AssistantStatus specifies the assistant's availability, decided by
// core at boot from the strength of the configured model.
export enum AssistantStatus {
	Active = "active",
	ActiveButWeak = "active-but-weak",
	InactiveTooWeak = "inactive-too-weak",
	Inactive = "inactive",
}

export interface AssistantCapability {
	status: AssistantStatus
	model?: string
}

export interface Capabilities {
	github: boolean
	slack: boolean
	aiAssistant: AssistantCapability
	changeDetection: boolean
	search: boolean
}
