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
	requestPasswordReset,
	setupSignInRedirect,
} = useAuthSession()
const { fetchAuthConfig, isEmailEnabled, isSingleOrganization, defaultAdmin } =
	useAuthAPI()

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

const loading = ref<
	"github" | "google" | "slack" | "email-password" | "password-reset" | null
>(null)
const view = ref<"methods" | "email-password" | "reset-request" | "reset-sent">(
	"methods",
)
const resetEmail = ref("")
const enabledMethods = computed(
	() => fetchAuthConfig.state.value.data?.methods ?? [],
)
// shown only while the default password still works.
const defaultAdminLogin = computed(() => {
	const admin = defaultAdmin.value
	if (!admin?.password) {
		return null
	}

	return { email: admin.email, password: admin.password }
})
const noMethodsConfigured = computed(
	() =>
		fetchAuthConfig.state.value.status === "success" &&
		enabledMethods.value.length === 0,
)
// with three or more visible methods, the first button carries the
// primary variant to anchor the list; with fewer, every button stays
// outline. The find list must match the template's button order.
const primaryMethod = computed(() => {
	if (enabledMethods.value.length < 3) {
		return null
	}

	return (
		(["google", "github", "slack", "email-password"] as const).find((m) =>
			enabledMethods.value.includes(m),
		) ?? null
	)
})

// an OAuth authorization request interrupted by login lands here with
// the provider's signed query (client_id + redirect_uri + sig). After
// sign-in the browser has to return to the authorize endpoint with that
// query intact so the flow can continue to the consent step.
const oauthContinueUrl = computed(() => {
	if (!route.query.client_id || !route.query.redirect_uri) {
		return null
	}

	const params = new URLSearchParams()

	for (const [key, value] of Object.entries(route.query)) {
		if (typeof value === "string") params.set(key, value)
	}

	return `${config.public.authRealtimeAPIBaseHttpURL}/api/auth/oauth2/authorize?${params.toString()}`
})

let redirectTimeout: ReturnType<typeof setTimeout> | undefined

onMounted(() => {
	if (route.query.verified === "true") {
		showToastMessage("success", t("onboarding.login.email-verified"))
	}

	if (route.query.reset === "true") {
		showToastMessage("success", t("onboarding.login.password-reset-success"))
	}

	redirectTimeout = setupSignInRedirect()
})

onUnmounted(() => {
	if (redirectTimeout !== undefined) clearTimeout(redirectTimeout)
})

async function logInWithProvider(provider: "github" | "google" | "slack") {
	loading.value = provider

	const res = (await signInSocial({
		provider,
		callbackURL:
			oauthContinueUrl.value ??
			postAuthDocumentUrl(
				config.public.appBaseURL,
				route.query.next as string | undefined,
			),
		newUserCallbackURL: postAuthDocumentUrl(
			config.public.appBaseURL,
			route.query.next as string | undefined,
			"/welcome",
		),
		fetchOptions: { query: route.query },
	})) as AuthResponse

	if (res.error) {
		showToastMessage("error", t("onboarding.login.errors.login-failed"))
		// reset only on error so that the loading spinner shows while
		// redirecting
		loading.value = null
	}
}

const onEmailPasswordSubmit = emailPasswordForm.handleSubmit(async (values) => {
	loading.value = "email-password"

	// the callbackURL does two jobs: the verification link an unverified
	// sign-in attempt re-sends drops the user there with the confirmation
	// flag, and a successful sign-in sends the browser there, where the
	// middleware forwards the signed-in visitor to next. Absolute against
	// the app origin because a relative path would resolve against the auth
	// server's origin, which does not serve the frontend in host-dev mode.
	const nextUrl = route.query.next as string | undefined
	const callbackQuery = new URLSearchParams({ verified: "true" })
	if (nextUrl) {
		callbackQuery.set("next", nextUrl)
	}

	const res = (await signInEmailPassword({
		email: values.email,
		password: values.password,
		callbackURL: `${config.public.appBaseURL}/login?${callbackQuery.toString()}`,
	})) as AuthResponse

	if (res.error) {
		// the sign-in attempt re-sent the verification link
		// (sendOnSignIn), so the check-your-inbox page is accurate.
		// Loading stays set so the spinner shows while redirecting.
		if (res.error.code === "EMAIL_NOT_VERIFIED") {
			void navigateTo({
				path: "/verify-email",
				query: { new: values.email, sent: "true" },
			})

			return
		}

		loading.value = null

		if (res.error.code === "INVALID_EMAIL_OR_PASSWORD") {
			showToastMessage(
				"error",
				t("onboarding.login.errors.invalid-credentials"),
			)

			return
		}

		showToastMessage("error", t("onboarding.login.errors.login-failed"))

		return
	}

	// a full navigation, not a router push: the authorize endpoint
	// answers with OAuth redirects the SPA router cannot follow. It
	// replaces the redirect below.
	if (oauthContinueUrl.value) {
		window.location.href = oauthContinueUrl.value

		return
	}

	// on the web a successful sign-in answers with redirect: true, and
	// better-auth's fetch plugin sets window.location to the callbackURL. A
	// router navigation here as well races that full-page load and cancels
	// it, which leaves the page stuck on /login.
	if (!__DESKTOP_BUILD__) {
		return
	}

	// the desktop bridge signs in from Electron's main process, where no
	// window follows the redirect, so the renderer navigates itself. The
	// session query still caches the signed-out null within its staleTime —
	// refetch before navigating or the middleware bounces straight back to
	// /login
	await fetchAuthSession.refetch()

	void navigateTo(nextUrl ? decodeURIComponent(nextUrl) : "/", {
		replace: true,
	})
})

// the reset-request view reuses the login form's email field (a second
// useForm in this component would override the injected form context).
// That rules out handleSubmit — it validates the whole schema, and the
// password field is not rendered in this view — so the one relevant
// field is validated explicitly and its value read from the form state.
async function onPasswordResetSubmit() {
	const { valid } = await emailPasswordForm.validateField("email")

	const email = emailPasswordForm.values.email
	if (!valid || !email) {
		return
	}

	loading.value = "password-reset"

	const res = (await requestPasswordReset({
		email,
		redirectTo: `${config.public.appBaseURL}/reset-password`,
	})) as AuthResponse

	loading.value = null

	if (res.error) {
		showToastMessage(
			"error",
			t("onboarding.login.errors.password-reset-failed"),
		)

		return
	}

	resetEmail.value = email
	view.value = "reset-sent"
}

function backToLogin() {
	view.value = "methods"
	emailPasswordForm.resetForm()
}

function backToEmailPassword() {
	view.value = "email-password"
	emailPasswordForm.resetForm()
}

function methodVariant(method: AuthMethod) {
	return primaryMethod.value === method ? "default" : "outline"
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
					{{ $t("onboarding.login.title") }}
				</div>
			</div>
			<div v-if="view === 'methods'" class="flex w-full flex-col gap-3">
				<ShadcnUiButton
					v-if="enabledMethods.includes('google')"
					size="lg"
					:variant="methodVariant('google')"
					:class="cn('h-10 w-full', loading !== null && 'pointer-events-none')"
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
					:class="cn('h-10 w-full', loading !== null && 'pointer-events-none')"
					:variant="methodVariant('github')"
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
					:class="cn('h-10 w-full', loading !== null && 'pointer-events-none')"
					:variant="methodVariant('slack')"
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
					:variant="methodVariant('email-password')"
					@click="view = 'email-password'"
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
				v-else-if="view === 'email-password'"
				class="flex w-full flex-col gap-3"
				@submit="onEmailPasswordSubmit"
			>
				<div
					v-if="defaultAdminLogin"
					class="flex items-start gap-2 rounded-md bg-accent px-3 py-2.5 text-2sm text-accent-foreground"
				>
					<Icon name="mingcute:key-2-line" class="mt-1 size-3.5 shrink-0" />
					<div class="flex flex-col gap-1.5 leading-relaxed">
						<p>{{ $t("onboarding.login.default-admin.intro") }}</p>
						<ul class="list-disc pl-4">
							<li>
								<code class="inline-code break-all">
									{{ defaultAdminLogin.email }}
								</code>
							</li>
							<li>
								<code class="inline-code break-all">
									{{ defaultAdminLogin.password }}
								</code>
							</li>
						</ul>
						<p>{{ $t("onboarding.login.default-admin.outro") }}</p>
					</div>
				</div>
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
			<form
				v-else-if="view === 'reset-request'"
				class="flex w-full flex-col gap-3"
				@submit.prevent="onPasswordResetSubmit"
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
									$t('onboarding.login.password-reset-form.email-placeholder')
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
					:disabled="loading === 'password-reset'"
				>
					<Icon
						v-show="loading === 'password-reset'"
						name="svg-spinners:blocks-shuffle-3"
						class="size-4"
					/>
					{{ $t("onboarding.login.password-reset-form.continue") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					type="button"
					size="lg"
					variant="ghost"
					class="h-10 w-full text-muted-foreground"
					:disabled="loading === 'password-reset'"
					@click="backToEmailPassword"
				>
					{{ $t("onboarding.login.password-reset-form.back") }}
				</ShadcnUiButton>
			</form>
			<div v-else class="flex w-full flex-col gap-3">
				<i18n-t
					scope="global"
					keypath="onboarding.login.password-reset-sent"
					tag="div"
					class="text-center text-xs text-accent-foreground"
				>
					<template #email>{{ resetEmail }}</template>
				</i18n-t>
				<ShadcnUiButton
					type="button"
					size="lg"
					variant="ghost"
					class="h-10 w-full text-muted-foreground"
					@click="backToEmailPassword"
				>
					{{ $t("onboarding.login.password-reset-form.back") }}
				</ShadcnUiButton>
			</div>
			<i18n-t
				v-if="view === 'email-password' && isEmailEnabled"
				scope="global"
				keypath="onboarding.login.forgot-password.main"
				tag="div"
				class="text-xs text-accent-foreground"
			>
				<template #reset>
					<ShadcnUiButton
						type="button"
						variant="ghost-plain"
						size="custom"
						class="text-xs font-semibold text-accent-foreground transition-none hover:opacity-70 focus:text-accent-foreground active:opacity-50 [&:not(:disabled):hover:not(:active)]:text-accent-foreground"
						@click="view = 'reset-request'"
					>
						{{ $t("onboarding.login.forgot-password.placeholders.reset") }}
					</ShadcnUiButton>
				</template>
			</i18n-t>
			<div
				v-if="isSingleOrganization && !isInvitationRedirect(route.query.next)"
				class="text-center text-xs text-accent-foreground"
			>
				{{ $t("onboarding.login.no-account.invitation-only") }}
			</div>
			<i18n-t
				v-else
				scope="global"
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
