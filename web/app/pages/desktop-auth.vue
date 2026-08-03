<script lang="ts" setup>
// Receives the OAuth handoff in the system browser and hands control back to
// the desktop app via the deep link.

definePageMeta({
	name: "desktop-auth",
	skipAuth: true,
})

const { setupSignInRedirect } = useAuthSession()
const { t } = useI18n({ useScope: "global" })

useHead({
	title: () => t("onboarding.desktop-auth.page-title"),
})

let redirectTimeout: ReturnType<typeof setTimeout> | undefined

onMounted(() => {
	redirectTimeout = setupSignInRedirect()
})

onUnmounted(() => {
	if (redirectTimeout !== undefined) clearTimeout(redirectTimeout)
})
</script>

<template>
	<main
		class="flex min-h-svh min-w-svw items-center justify-center bg-background text-foreground"
	>
		<div class="flex flex-col items-center gap-5">
			<Icon
				name="svg-spinners:blocks-shuffle-3"
				class="size-8 text-muted-foreground"
			/>
			<div class="text-lg font-semibold">
				{{ $t("onboarding.desktop-auth.message") }}
			</div>
		</div>
	</main>
</template>
