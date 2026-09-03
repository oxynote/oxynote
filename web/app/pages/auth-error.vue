<script lang="ts" setup>
// better-auth redirects here (its onAPIError.errorURL) when an auth or
// OAuth request fails before it can reach the client's redirect_uri —
// most often an MCP client presenting a client_id this deployment no
// longer knows. Only the error code is shown: error_description is
// arbitrary text taken straight from the query.
definePageMeta({
	name: "auth-error",
	middleware: (to) => {
		if (!to.query.error || typeof to.query.error !== "string") {
			return navigateTo("/")
		}
	},
})

const { t } = useI18n({ useScope: "global" })
useHead({
	title: () => t("oauth.error.page-title"),
})

const route = useRoute()
const errorCode = computed(() => route.query.error as string)
</script>
<template>
	<main
		class="flex min-h-svh min-w-svw items-center justify-center bg-background text-foreground"
	>
		<div class="flex w-67 flex-col items-center gap-5">
			<div class="flex flex-col items-center gap-6">
				<Icon name="custom-icons:main-logo" class="size-12" />
				<div class="text-lg font-semibold">
					{{ $t("oauth.error.heading") }}
				</div>
			</div>
			<div class="flex w-full flex-col gap-3">
				<div class="text-center text-xs text-accent-foreground">
					{{ $t("oauth.error.description") }}
				</div>
				<div class="text-center font-mono text-xs text-muted-foreground">
					{{ $t("oauth.error.code", { code: errorCode }) }}
				</div>
				<ShadcnUiButton
					type="button"
					size="lg"
					variant="ghost"
					class="h-10 w-full text-muted-foreground"
					@click="navigateTo('/')"
				>
					{{ $t("oauth.error.button") }}
				</ShadcnUiButton>
			</div>
		</div>
	</main>
</template>
