<script lang="ts" setup>
import { toTypedSchema } from "@vee-validate/zod"
import { useForm } from "vee-validate"
import * as z from "zod"
// for some reason this isn't auto imported
import { FormField as ShadcnUiFormField } from "@/components/shadcn/ui/form"
import { cn } from "~/lib/utils"
import { showToastMessage } from "~/components/toast"

definePageMeta({
	name: "onboarding",
	path: "/welcome",
	alias: ["/onboarding", "/onboard"],
})

const { t } = useI18n({ useScope: "global" })
useHead({
	title: () => t("general.onboarding-page-title"),
})

const formSchema = toTypedSchema(
	z.object({
		workspaceName: z
			.string()
			.trim()
			.min(2)
			.max(50)
			.regex(/^[a-zA-Z0-9-_]+$/, {
				message: t("onboarding.welcome.errors.workspace-name-regex"),
			}),
		workspaceSlug: z
			.string()
			.trim()
			.min(2)
			.max(50)
			.regex(/^[a-zA-Z0-9-_]+$/, {
				message: t("onboarding.welcome.errors.workspace-url-regex"),
			}),
	}),
)
const form = useForm({
	validationSchema: formSchema,
})
const loading = ref<"logout" | "submit" | null>(null)
const {
	fetchAuthSession,
	fetchOrganization,
	safeSignOut,
	checkOrganizationSlug,
	createOrganization,
	setActiveOrganization,
} = useAuthSession()
const nuxtApp = useNuxtApp()
const userEmail = computed(() => {
	return fetchAuthSession.state.value.data?.data?.user.email || ""
})

const onSubmit = form.handleSubmit(async (values) => {
	loading.value = "submit"
	await delay(300) // show loading spinner for at least a moment

	if (values.workspaceSlug) {
		const res = (await checkOrganizationSlug({
			slug: values.workspaceSlug,
		})) as AuthResponse
		if (res.error) {
			loading.value = null

			if (res.error.status === 400) {
				form.setFieldError(
					"workspaceSlug",
					t("onboarding.welcome.errors.workspace-url-taken"),
				)
				return
			}

			form.setFieldError(
				"workspaceSlug",
				t("onboarding.welcome.errors.workspace-url-invalid"),
			)

			return
		}
	}

	// better-auth fills data whenever error is null, so the organization is
	// typed as always present rather than re-checked after the error branch
	const { data: orgData, error: orgError } = (await createOrganization({
		name: values.workspaceName,
		slug: values.workspaceSlug,
		logo: "https://example.com/logo.png", // TODO allow uploading logo later
	})) as AuthResponse & { data: { id: string; slug: string } }
	if (orgError) {
		// reset only on error so that the loading spinner shows while
		// redirecting
		loading.value = null

		// https://github.com/better-auth/better-auth/blob/main/packages/better-auth/src/plugins/organization/error-codes.ts#L4
		if (orgError.code === "YOU_ARE_NOT_ALLOWED_TO_CREATE_A_NEW_ORGANIZATION") {
			showToastMessage(
				"info",
				t("onboarding.signup.errors.organization-limit-reached.title"),
				t("onboarding.signup.errors.organization-limit-reached.description"),
			)
			return
		}

		showToastMessage("error", t("onboarding.welcome.errors.create-failed"))
		return
	}

	const res = (await setActiveOrganization({
		organizationId: orgData.id,
		organizationSlug: orgData.slug,
	})) as AuthResponse
	if (res.error) {
		showToastMessage("error", t("onboarding.welcome.errors.activation-failed"))
		// reset only on error so that the loading spinner shows while
		// redirecting
		loading.value = null
		return
	}

	await Promise.all([fetchAuthSession.refetch(), fetchOrganization.refetch()])

	// created here, not at the top of the setup: a freshly instantiated
	// tree query fetches immediately, and before the workspace exists the
	// session has no active organization, which core rejects with a 401 —
	// one the API client turns into a redirect to /login, bouncing a
	// signed-in visitor straight back here in an endless loop. By this
	// point the organization exists and the refreshed session carries it.
	const { fetchDocumentTree } = await nuxtApp.runWithContext(() =>
		useDocumentAPI(),
	)

	let firstDoc: DocumentTreeElement | null = null

	// since the organization and its first document creation is atomic, we
	// may need to retry fetching the document tree a few times
	for (let retry = 0; retry < 50; retry++) {
		const docTree = await fetchDocumentTree.refetch()

		const doc = docTree.data?.[0]
		if (doc) {
			firstDoc = doc
			break
		}

		// wait a bit before (re)trying
		await delay(500)
	}

	if (!firstDoc) {
		// unlikely to happen, but just in case
		showToastMessage(
			"error",
			t("onboarding.welcome.errors.first-document-load-failed"),
		)
		return
	}

	void navigateTo(
		`/${createNameSlug(values.workspaceName)}/${createNameSlugWithId(firstDoc.documentName, firstDoc.id)}`,
		{
			replace: true,
		},
	)
})

async function logOut() {
	loading.value = "logout"

	const res = (await safeSignOut()) as AuthResponse
	if (res.error) {
		showToastMessage("error", t("onboarding.welcome.errors.signout-failed"))
		// reset only on error
		loading.value = null
		return
	}

	void navigateTo({ name: "login" })
}
</script>
<template>
	<main
		class="relative flex h-dvh min-h-180 items-center justify-center bg-background text-foreground"
	>
		<div class="fixed top-0 left-0 flex w-full items-center justify-between">
			<ShadcnUiButton
				size="2sm"
				variant="ghost"
				:disabled="loading === 'logout'"
				:class="
					cn(
						'mt-6 ml-8 text-muted-foreground',
						loading === 'submit' && 'pointer-events-none',
					)
				"
				@click="logOut"
			>
				{{ $t("onboarding.welcome.login-info.logout") }}
			</ShadcnUiButton>
			<div
				class="mt-6 mr-8 flex max-w-[40dvw] min-w-0 flex-col gap-0.5 md:max-w-76"
			>
				<div class="text-xs text-muted-foreground">
					{{ $t("onboarding.welcome.login-info.logged-in-as") }}
				</div>
				<div
					class="min-w-0 truncate text-2sm font-medium text-accent-foreground"
				>
					{{ userEmail }}
				</div>
			</div>
		</div>
		<div
			class="flex w-full flex-col items-center gap-8 px-3 min-[30.625rem]:w-114"
		>
			<div class="flex w-full flex-col items-center gap-6">
				<Icon name="custom-icons:main-logo" class="size-12" />
				<div class="text-center text-2xl font-semibold">
					{{ $t("onboarding.welcome.title") }}
				</div>
				<div class="text-center text-2base font-normal text-muted-foreground">
					{{ $t("onboarding.welcome.description") }}
				</div>
			</div>
			<form
				class="flex w-full flex-col items-center gap-6"
				autocomplete="off"
				@submit="onSubmit"
			>
				<div
					class="flex w-full flex-col gap-6 rounded-md bg-card p-6 text-card-foreground shadow-fully-clear"
				>
					<ShadcnUiFormField v-slot="{ componentField }" name="workspaceName">
						<ShadcnUiFormItem>
							<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
								{{ $t("onboarding.welcome.form.workspace-name.label") }}
							</ShadcnUiFormLabel>
							<ShadcnUiFormControl>
								<ShadcnUiInput
									type="text"
									:placeholder="
										$t('onboarding.welcome.form.workspace-name.placeholder')
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
							<ShadcnUiFormMessage class="text-2xs" />
						</ShadcnUiFormItem>
					</ShadcnUiFormField>
					<ShadcnUiFormField
						v-slot="{ componentField }"
						name="workspaceSlug"
						:validate-on-model-update="false"
						:validate-on-input="false"
						:validate-on-change="false"
						:validate-on-blur="true"
					>
						<ShadcnUiFormItem>
							<ShadcnUiFormLabel disable-destructive-effect class="text-2sm">
								{{ $t("onboarding.welcome.form.workspace-url.label") }}
							</ShadcnUiFormLabel>
							<ShadcnUiFormControl>
								<div class="relative">
									<div
										class="absolute top-1/2 left-3 -translate-y-1/2 text-2base text-muted-foreground"
									>
										{{ $t("onboarding.welcome.form.workspace-url.prefix") }}
									</div>
									<ShadcnUiInput
										type="text"
										disable-focus-effect
										disable-destructive-effect
										:class="
											cn(
												'h-10 pl-22.5 text-2base!',
												loading !== null && 'pointer-events-none',
											)
										"
										v-bind="{
											...componentField,
										}"
									/>
								</div>
							</ShadcnUiFormControl>
							<ShadcnUiFormMessage class="text-2xs" />
						</ShadcnUiFormItem>
					</ShadcnUiFormField>
				</div>
				<ShadcnUiButton
					type="submit"
					size="lg"
					:class="
						cn(
							'h-10 w-[80%] min-[30.625rem]:w-[70%]',
							loading === 'logout' && 'pointer-events-none',
						)
					"
					:disabled="loading === 'submit'"
				>
					<Icon
						v-show="loading === 'submit'"
						name="svg-spinners:blocks-shuffle-3"
						class="size-4"
					/>
					{{ $t("onboarding.welcome.create-workspace") }}
				</ShadcnUiButton>
			</form>
		</div>
	</main>
</template>
