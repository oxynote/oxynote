export enum ExperimentalFeature {
	AIAssistant = "ai-assistant",
}

export default function () {
	const config = useRuntimeConfig()
	const { fetchOrganization } = useAuthSession()

	function isExperimentalFeatureEnabled(featureName: ExperimentalFeature) {
		return computed(() => {
			if (import.meta.dev) {
				// In development, enable all experimental features for easier testing
				return true
			}

			const organizationId = fetchOrganization.data.value?.data?.id
			if (!organizationId) {
				return false
			}

			const featureConfigs = config.public.experimentalFeatures.split(";")

			for (const featureConfig of featureConfigs) {
				const [name, orgs] = featureConfig.split(":")
				if (name === featureName) {
					const orgList = orgs?.split(",").map((org) => org.trim()) ?? []
					return orgList.includes(organizationId)
				}
			}

			return false
		})
	}

	return {
		isExperimentalFeatureEnabled,
	}
}
