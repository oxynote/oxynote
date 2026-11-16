<script lang="ts" setup>
// better-auth redirects here (its onAPIError.errorURL) when an auth or
// OAuth request fails before it can reach the client's redirect_uri —
// most often an MCP client presenting a client_id this deployment no
// longer knows. Only the error code is shown: error_description is
// arbitrary text taken straight from the query.

// the codes of a sign-in the server refused for this account.
const REFUSED_SIGN_IN_CODES = [
	"SIGN_UP_DISABLED",
	"account_not_linked",
	"email_not_verified",
]

definePageMeta({
	name: "auth-error",
	middleware: (to) => {
		if (!to.query.error || typeof to.query.error !== "string") {
			return navigateTo("/")
		}
	},
})

const { t } = useI18n({ useScope: "global" })
const { oidcEndSessionUrl } = useAuthAPI()
useHead({
	title: () => t("oauth.error.page-title"),
})

const route = useRoute()
const errorCode = computed(() => route.query.error as string)
// a sign-in through a provider also ends here when it is refused. These
// codes get their own explanation, and every other code keeps the generic
// one. The two spellings differ because better-auth passes a sign-up
// hook's own error code through as it is, but names its OAuth failures in
// lowercase.
const description = computed(() => {
	switch (errorCode.value) {
		case "SIGN_UP_DISABLED":
			return t("oauth.error.sign-up-disabled")
		case "account_not_linked":
			return t("oauth.error.account-not-linked")
		case "email_not_verified":
			return t("oauth.error.email-not-verified")
		default:
			return t("oauth.error.description")
	}
})
// a refused OIDC sign-in leaves the identity provider signed in, so
// another try would come back as the same user. The button ends that
// session first.
const offersOtherAccount = computed(
	() =>
		route.query.provider === "oidc" &&
		REFUSED_SIGN_IN_CODES.includes(errorCode.value),
)

function switchAccount() {
	void navigateTo(oidcEndSessionUrl, { external: true })
}
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
					{{ description }}
				</div>
				<div class="text-center font-mono text-xs text-muted-foreground">
					{{ $t("oauth.error.code", { code: errorCode }) }}
				</div>
				<ShadcnUiButton
					v-if="offersOtherAccount"
					type="button"
					size="lg"
					variant="outline"
					class="h-10 w-full"
					@click="switchAccount"
				>
					{{ $t("oauth.error.other-account") }}
				</ShadcnUiButton>
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
