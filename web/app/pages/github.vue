<script lang="ts" setup>
definePageMeta({
	name: "github",
})

const pageRoute = useRoute()
const { connectGitHub, fetchGitHubConnectionStatus } = useGitHubAPI()
const { t } = useI18n({ useScope: "global" })
useHead({
	title: () => t("general.github-connect-page-title"),
})

const connectionState = ref<"loading" | "success" | "error">("loading")

const queryParams = computed(() => {
	const params = new URLSearchParams()

	for (const [key, value] of Object.entries(pageRoute.query)) {
		if (typeof value === "string") {
			params.append(key, value)
			continue
		}

		if (Array.isArray(value)) {
			for (const entry of value) {
				if (typeof entry === "string") {
					params.append(key, entry)
				}
			}
		}
	}

	return params
})

const statusMessage = computed(() => {
	if (connectionState.value === "success") {
		return t("apps.github-connect.success")
	}

	if (connectionState.value === "error") {
		return t("apps.github-connect.error")
	}

	return t("apps.github-connect.loading")
})

onMounted(async () => {
	const params = queryParams.value
	const installationId = params.get("installation_id")
	const state = params.get("state")

	if (!installationId || !state) {
		connectionState.value = "error"
		return
	}

	try {
		await connectGitHub.mutateAsync(params)
		fetchGitHubConnectionStatus.refetch()
		connectionState.value = "success"
	} catch {
		connectionState.value = "error"
	}
})
</script>
<template>
	<main
		class="flex min-h-svh min-w-svw items-center justify-center bg-background text-foreground"
	>
		<div class="flex w-95 flex-col items-center gap-5 px-2">
			<div class="flex flex-col items-center gap-6">
				<Icon name="custom-icons:main-logo" class="size-12" />
				<div class="text-center text-lg font-semibold">
					{{ statusMessage }}
				</div>
			</div>
			<NuxtLink
				to="/"
				class="text-base font-semibold text-muted-foreground hover:opacity-70 active:opacity-50"
			>
				{{ $t("onboarding.verify-email.button") }}
			</NuxtLink>
		</div>
	</main>
</template>
