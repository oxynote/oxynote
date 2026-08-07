<script setup lang="ts">
import { postAuthDocumentUrl } from "#imports"
import { showToastMessage } from "~/components/toast"
import { cn } from "~/lib/utils"

definePageMeta({
	name: "login",
	skipAuth: true,
})

const { t } = useI18n({ useScope: "global" })
useHead({
	title: () => t("general.login-page-title"),
})

const { signInSocial, setupSignInRedirect } = useAuthSession()
const { fetchAuthConfig } = useAuthAPI()

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

let redirectTimeout: ReturnType<typeof setTimeout> | undefined

onMounted(() => {
	const error = route.query.error
	if (error === "auth.login_not_allowed") {
		showToastMessage("error", t("onboarding.login.errors.login-not-allowed"))
	} else if (error === "auth.registration_not_allowed") {
		showToastMessage(
			"error",
			t("onboarding.login.errors.registration-not-allowed"),
		)
	}

	redirectTimeout = setupSignInRedirect()
})

onUnmounted(() => {
	if (redirectTimeout !== undefined) clearTimeout(redirectTimeout)
})

async function logInWithProvider(provider: "github" | "google" | "slack") {
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
		showToastMessage("error", t("onboarding.login.errors.login-failed"))
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
		<div class="flex w-[16.75rem] flex-col items-center gap-5">
			<div class="flex flex-col items-center gap-6">
				<Icon name="custom-icons:main-logo" class="size-12" />
				<div class="text-lg font-semibold">
					{{ $t("onboarding.login.title") }}
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
					@click="logInWithProvider('google')"
				>
					<Icon
						:name="
							loading === 'google'
								? 'svg-spinners:blocks-shuffle-3'
								: 'simple-icons:google'
						"
						class="size-4"
					/>
					{{ $t("onboarding.login.login-google") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					v-if="enabledMethods.includes('github')"
					size="lg"
					:class="
						cn('h-[2.5rem] w-full', loading !== null && 'pointer-events-none')
					"
					variant="outline"
					:disabled="loading === 'github'"
					@click="logInWithProvider('github')"
				>
					<Icon
						:name="
							loading === 'github'
								? 'svg-spinners:blocks-shuffle-3'
								: 'simple-icons:github'
						"
						class="size-4"
					/>
					{{ $t("onboarding.login.login-github") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					v-if="enabledMethods.includes('slack')"
					size="lg"
					:class="
						cn('h-[2.5rem] w-full', loading !== null && 'pointer-events-none')
					"
					variant="outline"
					:disabled="loading === 'slack'"
					@click="logInWithProvider('slack')"
				>
					<Icon
						:name="
							loading === 'slack'
								? 'svg-spinners:blocks-shuffle-3'
								: 'simple-icons:slack'
						"
						class="size-4"
					/>
					{{ $t("onboarding.login.login-slack") }}
				</ShadcnUiButton>
				<div
					v-if="noMethodsConfigured"
					class="text-center text-xs text-accent-foreground"
				>
					{{ $t("onboarding.login.no-methods") }}
				</div>
			</div>
			<i18n-t
				keypath="onboarding.login.no-account.main"
				tag="div"
				class="text-xs text-accent-foreground"
			>
				<template #sign-up>
					<NuxtLink
						:to="{ name: 'signup', query: route.query }"
						class="font-semibold hover:opacity-70 active:opacity-50"
					>
						{{ $t("onboarding.login.no-account.placeholders.sign-up") }}
					</NuxtLink>
				</template>
				<template #learn-more>
					<a
						:href="config.public.linkToMoreInfoAboutProduct"
						class="font-semibold hover:opacity-70 active:opacity-50"
					>
						{{ $t("onboarding.login.no-account.placeholders.learn-more") }}
					</a>
				</template>
			</i18n-t>
		</div>
	</main>
</template>
