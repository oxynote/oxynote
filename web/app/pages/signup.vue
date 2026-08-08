<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "@/components/shadcn/ui/form"
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

const { signInSocial, signUpEmailPassword, setupSignInRedirect } =
	useAuthSession()
const { fetchAuthConfig } = useAuthAPI()

const { fetchOrganizationStats } = useOrganizationAPI()
const { currentUrl } = useCurrentUrl()
const route = useRoute()
const config = useRuntimeConfig()

const emailPasswordFormSchema = toTypedSchema(
	z.object({
		email: z.string().trim().email(t("onboarding.signup.errors.email-invalid")),
		password: z
			.string()
			.min(16)
			.max(128)
			.regex(/[0-9]/, {
				message: t("onboarding.signup.errors.password-number"),
			})
			.regex(/[^a-zA-Z0-9]/, {
				message: t("onboarding.signup.errors.password-symbol"),
			}),
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

const onEmailPasswordSubmit = emailPasswordForm.handleSubmit(async (values) => {
	if (!newAccountsAllowed.value && !route.query.next) {
		showToastMessage(
			"info",
			t("onboarding.signup.errors.organization-limit-reached.title"),
			t("onboarding.signup.errors.organization-limit-reached.description"),
		)
		return
	}

	loading.value = "email-password"

	// better-auth requires a display name; the email local part is a
	// sensible initial value until the user sets a real one.
	const res = await signUpEmailPassword({
		email: values.email,
		password: values.password,
		name: values.email.split("@")[0] || values.email,
	})
	// no duplicate-email branch on purpose: better-auth answers duplicate
	// signups with a synthetic success so the browser can't probe which
	// emails have accounts. The existing owner is notified through their
	// inbox instead (onExistingUserSignUp in auth-realtime).
	if (res.error) {
		loading.value = null
		showToastMessage("error", t("onboarding.signup.errors.signup-failed"))

		return
	}

	navigateTo({
		path: "/verify-email",
		query: { new: values.email, sent: "true" },
	})
})

function backToSignup() {
	showEmailPasswordForm.value = false
	emailPasswordForm.resetForm()
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
			<div v-if="!showEmailPasswordForm" class="flex w-full flex-col gap-3">
				<ShadcnUiButton
					v-if="enabledMethods.includes('google')"
					size="lg"
					:class="cn('h-10 w-full', loading !== null && 'pointer-events-none')"
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
					:class="cn('h-10 w-full', loading !== null && 'pointer-events-none')"
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
					:class="cn('h-10 w-full', loading !== null && 'pointer-events-none')"
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
				<ShadcnUiButton
					v-if="enabledMethods.includes('email-password')"
					size="lg"
					:class="cn('h-10 w-full', loading !== null && 'pointer-events-none')"
					variant="outline"
					@click="showEmailPasswordForm = true"
				>
					<Icon name="lucide:mail" class="size-4" />
					{{ $t("onboarding.signup.signup-email-password") }}
				</ShadcnUiButton>
				<div
					v-if="noMethodsConfigured"
					class="text-center text-xs text-accent-foreground"
				>
					{{ $t("onboarding.signup.no-methods") }}
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
									$t('onboarding.signup.email-password-form.email-placeholder')
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
								autocomplete="new-password"
								:placeholder="
									$t(
										'onboarding.signup.email-password-form.password-placeholder',
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
					{{ $t("onboarding.signup.email-password-form.continue") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					type="button"
					size="lg"
					variant="ghost"
					class="h-10 w-full text-muted-foreground"
					:disabled="loading === 'email-password'"
					@click="backToSignup"
				>
					{{ $t("onboarding.signup.email-password-form.back") }}
				</ShadcnUiButton>
			</form>
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
