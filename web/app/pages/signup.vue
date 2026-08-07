<script lang="ts" setup>
import { showToastMessage } from "~/components/toast"
import { cn } from "~/lib/utils"

definePageMeta({
	name: "signup",
	skipAuth: true,
})

const { t } = useI18n({ useScope: "global" })
useHead({
	title: () => t("general.signup-page-title"),
})

const { signInSocial, setupSignInRedirect } = useAuthSession()
const { fetchAuthConfig } = useAuthAPI()

const { fetchOrganizationStats } = useOrganizationAPI()
const { currentUrl } = useCurrentUrl()
const route = useRoute()
const config = useRuntimeConfig()
const loading = ref<"github" | "google" | "slack" | null>(null)
const enabledMethods = computed(
	() => fetchAuthConfig.state.value.data?.methods ?? [],
)
const noMethodsConfigured = computed(
	() =>
		fetchAuthConfig.state.value.status === "success" &&
		enabledMethods.value.length === 0,
)
const newAccountsAllowed = computed(() => {
	return (
		fetchOrganizationStats.state.value.data &&
		fetchOrganizationStats.state.value.data.availableSlots > 0
	)
})

let redirectTimeout: ReturnType<typeof setTimeout> | undefined

onMounted(() => {
	if (isDeletionSuccessInUrl(currentUrl.value.toString())) {
		showToastMessage(
			"success",
			t(
				"settings.action-modals.account-deletion.success-message.account-deleted.title",
			),
			t(
				"settings.action-modals.account-deletion.success-message.account-deleted.description",
			),
		)
	}

	redirectTimeout = setupSignInRedirect()
})

onUnmounted(() => {
	if (redirectTimeout !== undefined) clearTimeout(redirectTimeout)
})

async function signUpWithProvider(provider: "github" | "google" | "slack") {
	if (!newAccountsAllowed.value && !route.query.next) {
		showToastMessage(
			"info",
			t("onboarding.signup.errors.organization-limit-reached.title"),
			t("onboarding.signup.errors.organization-limit-reached.description"),
		)
		return
	}

	loading.value = provider
	let res: AuthResponse | null = null

	switch (provider) {
		case "github":
			res = await signInSocial({
				provider: "github",
				callbackURL: postAuthDocumentUrl(
					config.public.appBaseURL,
					route.query.next as string | undefined,
				),
				newUserCallbackURL: postAuthDocumentUrl(
					config.public.appBaseURL,
					route.query.next as string | undefined,
					"/welcome",
				),
				fetchOptions: { query: route.query },
			})
			break
		case "slack":
			res = await signInSocial({
				provider: "slack",
				callbackURL: postAuthDocumentUrl(
					config.public.appBaseURL,
					route.query.next as string | undefined,
				),
				newUserCallbackURL: postAuthDocumentUrl(
					config.public.appBaseURL,
					route.query.next as string | undefined,
					"/welcome",
				),
				fetchOptions: { query: route.query },
			})
			break
		case "google":
			res = await signInSocial({
				provider: "google",
				callbackURL: postAuthDocumentUrl(
					config.public.appBaseURL,
					route.query.next as string | undefined,
				),
				newUserCallbackURL: postAuthDocumentUrl(
					config.public.appBaseURL,
					route.query.next as string | undefined,
					"/welcome",
				),
				fetchOptions: { query: route.query },
			})
	}

	if (res?.error) {
		showToastMessage("error", t("onboarding.signup.errors.signup-failed"))
		// reset only on error so that the loading spinner shows while
		// redirecting
		loading.value = null
	}
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
					{{ $t("onboarding.signup.title") }}
				</div>
			</div>
			<div class="flex w-full flex-col gap-3">
				<ShadcnUiButton
					v-if="enabledMethods.includes('google')"
					size="lg"
					:class="
						cn('h-[2.5rem] w-full', loading !== null && 'pointer-events-none')
					"
					:disabled="loading === 'google'"
					@click="signUpWithProvider('google')"
				>
					<Icon
						:name="
							loading === 'google'
								? 'svg-spinners:blocks-shuffle-3'
								: 'simple-icons:google'
						"
						class="size-4"
					/>
					{{ $t("onboarding.signup.signup-google") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					v-if="enabledMethods.includes('github')"
					size="lg"
					:class="
						cn('h-[2.5rem] w-full', loading !== null && 'pointer-events-none')
					"
					variant="outline"
					:disabled="loading === 'github'"
					@click="signUpWithProvider('github')"
				>
					<Icon
						:name="
							loading === 'github'
								? 'svg-spinners:blocks-shuffle-3'
								: 'simple-icons:github'
						"
						class="size-4"
					/>
					{{ $t("onboarding.signup.signup-github") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					v-if="enabledMethods.includes('slack')"
					size="lg"
					:class="
						cn('h-[2.5rem] w-full', loading !== null && 'pointer-events-none')
					"
					variant="outline"
					:disabled="loading === 'slack'"
					@click="signUpWithProvider('slack')"
				>
					<Icon
						:name="
							loading === 'slack'
								? 'svg-spinners:blocks-shuffle-3'
								: 'simple-icons:slack'
						"
						class="size-4"
					/>
					{{ $t("onboarding.signup.signup-slack") }}
				</ShadcnUiButton>
				<div
					v-if="noMethodsConfigured"
					class="text-center text-xs text-accent-foreground"
				>
					{{ $t("onboarding.signup.no-methods") }}
				</div>
			</div>
			<i18n-t
				keypath="onboarding.signup.conditions.main"
				tag="div"
				class="text-center text-xs text-accent-foreground"
			>
				<template #terms-of-service>
					<a
						:href="config.public.termsOfServiceURL"
						rel="noopener noreferrer"
						class="font-semibold hover:opacity-70 active:opacity-50"
					>
						{{
							$t("onboarding.signup.conditions.placeholders.terms-of-service")
						}}
					</a>
				</template>
				<template #privacy-policy>
					<a
						:href="config.public.privacyPolicyURL"
						rel="noopener noreferrer"
						class="font-semibold hover:opacity-70 active:opacity-50"
					>
						{{ $t("onboarding.signup.conditions.placeholders.privacy-policy") }}
					</a>
				</template>
			</i18n-t>
			<i18n-t
				keypath="onboarding.signup.have-account.main"
				tag="div"
				class="text-xs text-accent-foreground"
			>
				<template #log-in>
					<NuxtLink
						:to="{ name: 'login', query: route.query }"
						class="font-semibold hover:opacity-70 active:opacity-50"
					>
						{{ $t("onboarding.signup.have-account.placeholders.log-in") }}
					</NuxtLink>
				</template>
			</i18n-t>
		</div>
	</main>
</template>
