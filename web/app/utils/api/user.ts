// SpotlightState specifies how far a one-time spotlight has gone for the
// user. No state means it was never shown.
export enum SpotlightState {
	Shown = "shown",
	Dismissed = "dismissed",
}

export interface UserSpotlights {
	star: SpotlightState | null
}

export interface UserSpotlightsUpdateRequest {
	star: SpotlightState
}
