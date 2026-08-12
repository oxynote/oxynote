<script setup lang="ts">
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "@/components/shadcn/ui/form"
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

const {
	fetchAuthSession,
	signInSocial,
	signInEmailPassword,
	setupSignInRedirect,
} = useAuthSession()
const { fetchAuthConfig } = useAuthAPI()

const route = useRoute()
const config = useRuntimeConfig()

const emailPasswordFormSchema = toTypedSchema(
	z.object({
		email: z.string().trim().email(t("onboarding.login.errors.email-invalid")),
		password: z.string().min(1, t("onboarding.login.errors.password-required")),
	}),
)
const emailPasswordForm = useForm({
	validationSchema: emailPasswordFormSchema,
})

const loading = ref<"github" | "google" | "slack" | "email-password" | null>(
	null,
)
const showEmailPasswordForm = ref(false)
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

const onEmailPasswordSubmit = emailPasswordForm.handleSubmit(async (values) => {
	loading.value = "email-password"

	const res = await signInEmailPassword({
		email: values.email,
		password: values.password,
	})

	if (res.error) {
		// the sign-in attempt re-sent the verification link
		// (sendOnSignIn), so the check-your-inbox page is accurate.
		// Loading stays set so the spinner shows while redirecting.
		if ((res.error as any).code === "EMAIL_NOT_VERIFIED") {
			navigateTo({
				path: "/verify-email",
				query: { new: values.email, sent: "true" },
			})

			return
		}

		loading.value = null

		if ((res.error as any).code === "INVALID_EMAIL_OR_PASSWORD") {
			showToastMessage(
				"error",
				t("onboarding.login.errors.invalid-credentials"),
			)

			return
		}

		showToastMessage("error", t("onboarding.login.errors.login-failed"))

		return
	}

	// the session query still caches the signed-out null within its
	// staleTime — refetch before navigating or the middleware bounces
	// straight back to /login
	await fetchAuthSession.refetch()

	const nextUrl = route.query.next as string | undefined
	navigateTo(nextUrl ? decodeURIComponent(nextUrl) : "/", { replace: true })
})

function backToLogin() {
	showEmailPasswordForm.value = false
	emailPasswordForm.resetForm()
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
			<div v-if="!showEmailPasswordForm" class="flex w-full flex-col gap-3">
				<ShadcnUiButton
					v-if="enabledMethods.includes('google')"
					size="lg"
					:class="
						cn('h-10 w-full', loading !== null && 'pointer-events-none')
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
						cn('h-10 w-full', loading !== null && 'pointer-events-none')
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
						cn('h-10 w-full', loading !== null && 'pointer-events-none')
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
				<ShadcnUiButton
					v-if="enabledMethods.includes('email-password')"
					size="lg"
					:class="cn('h-10 w-full', loading !== null && 'pointer-events-none')"
					variant="outline"
					@click="showEmailPasswordForm = true"
				>
					<Icon name="lucide:mail" class="size-4" />
					{{ $t("onboarding.login.login-email-password") }}
				</ShadcnUiButton>
				<div
					v-if="noMethodsConfigured"
					class="text-center text-xs text-accent-foreground"
				>
					{{ $t("onboarding.login.no-methods") }}
				</div>
			</div>
			<form
				v-else
				class="flex w-full flex-col gap-3"
				@submit="onEmailPasswordSubmit"
			>
				<ShadcnUiFormField
					v-slot="{ componentField, meta }"
					name="email"
					:validate-on-model-update="false"
					:validate-on-input="false"
					:validate-on-change="false"
					:validate-on-blur="true"
				>
					<ShadcnUiFormItem>
						<ShadcnUiFormControl>
							<ShadcnUiInput
								type="email"
								autocomplete="email"
								:placeholder="
									$t('onboarding.login.email-password-form.email-placeholder')
								"
								disable-focus-effect
								disable-destructive-effect
								:class="
									cn(
										'h-10 text-2base!',
										loading !== null && 'pointer-events-none',
									)
								"
								v-bind="{
									...componentField,
								}"
							/>
						</ShadcnUiFormControl>
						<ShadcnUiFormMessage
							v-if="meta.touched || meta.validated"
							class="text-2xs"
						/>
					</ShadcnUiFormItem>
				</ShadcnUiFormField>
				<ShadcnUiFormField
					v-slot="{ componentField, meta }"
					name="password"
					:validate-on-model-update="false"
					:validate-on-input="false"
					:validate-on-change="false"
					:validate-on-blur="true"
				>
					<ShadcnUiFormItem>
						<ShadcnUiFormControl>
							<ShadcnUiInput
								type="password"
								autocomplete="current-password"
								:placeholder="
									$t(
										'onboarding.login.email-password-form.password-placeholder',
									)
								"
								disable-focus-effect
								disable-destructive-effect
								:class="
									cn(
										'h-10 text-2base!',
										loading !== null && 'pointer-events-none',
									)
								"
								v-bind="{
									...componentField,
								}"
							/>
						</ShadcnUiFormControl>
						<ShadcnUiFormMessage
							v-if="meta.touched || meta.validated"
							class="text-2xs"
						/>
					</ShadcnUiFormItem>
				</ShadcnUiFormField>
				<ShadcnUiButton
					type="submit"
					size="lg"
					class="h-10 w-full"
					:disabled="loading === 'email-password'"
				>
					<Icon
						v-show="loading === 'email-password'"
						name="svg-spinners:blocks-shuffle-3"
						class="size-4"
					/>
					{{ $t("onboarding.login.email-password-form.continue") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					type="button"
					size="lg"
					variant="ghost"
					class="h-10 w-full text-muted-foreground"
					:disabled="loading === 'email-password'"
					@click="backToLogin"
				>
					{{ $t("onboarding.login.email-password-form.back") }}
				</ShadcnUiButton>
			</form>
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
