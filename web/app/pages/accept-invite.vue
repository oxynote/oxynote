<script lang="ts" setup>
import { showToastMessage } from "~/components/toast"
import { cn } from "~/lib/utils"

definePageMeta({
	middleware: async (to) => {
		if (!to.query.id || typeof to.query.id !== "string") {
			return navigateTo("/")
		}

		if (!to.query.email || typeof to.query.email !== "string") {
			return navigateTo("/")
		}

		if (!to.query.inviter || typeof to.query.inviter !== "string") {
			return navigateTo("/")
		}

		if (!to.query.orgName || typeof to.query.orgName !== "string") {
			return navigateTo("/")
		}

		if (!to.query.orgId || typeof to.query.orgId !== "string") {
			return navigateTo("/")
		}
	},
})
const { t } = useI18n({ useScope: "global" })
useHead({
	title: () => t("general.accept-invite-page-title"),
})

const {
	fetchAuthSession,
	updateSessionOnInviteAccept,
	safeSignOut,
	acceptOrganizationInvitation,
} = useAuthSession()
const pageRoute = useRoute()
const inviteInfo = computed(() => {
	return {
		id: pageRoute.query.id! as string,
		email: pageRoute.query.email! as string,
		inviter: pageRoute.query.inviter! as string,
		orgName: pageRoute.query.orgName! as string,
		orgId: pageRoute.query.orgId! as string,
	}
})
const loading = ref<"logout" | "accept" | null>(null)
const isAuthenticated = computed(() => {
	return !!fetchAuthSession.state.value.data?.data
})

function navigateToSignupWithRedirect() {
	return navigateTo({
		name: "signup",
		query: {
			next: encodeURIComponent(pageRoute.fullPath),
		},
	})
}

async function acceptInvite() {
	if (!isAuthenticated.value) {
		return
	}

	if (
		fetchAuthSession.state.value.data?.data?.user.email !==
		inviteInfo.value.email
	) {
		showToastMessage(
			"warning",
			t("onboarding.accept-invite.warn.email-mismatch.title"),
			t("onboarding.accept-invite.warn.email-mismatch.description"),
		)
		navigateTo({ name: "onboarding" })

		return
	}

	loading.value = "accept"
	await delay(300) // show loading spinner for at least a moment

	const { error } = await acceptOrganizationInvitation({
		invitationId: inviteInfo.value.id,
	})
	if (error) {
		loading.value = null

		// https://github.com/better-auth/better-auth/blob/main/packages/better-auth/src/plugins/organization/error-codes.ts#L4
		if (error.code === "ORGANIZATION_MEMBERSHIP_LIMIT_REACHED") {
			showToastMessage(
				"info",
				t("onboarding.accept-invite.errors.member-limit-reached.title"),
			)
			return
		}

		showToastMessage(
			"error",
			t("onboarding.accept-invite.errors.accept-failed"),
		)

		return
	}

	const sessionOrgUpdated = await updateSessionOnInviteAccept(
		inviteInfo.value.orgId,
	)
	if (!sessionOrgUpdated) {
		// show this as success anyway, even though we couldn't update the session
		showToastMessage(
			"success",
			t("onboarding.accept-invite.success.accepted-invite-log-out.title"),
			t("onboarding.accept-invite.success.accepted-invite-log-out.description"),
		)
		await logOut(true) // if the session couldn't be updated, log out the user (the org will be inserted on next login)
		// FIXME: edge case: what if sign-out fails?

		return
	}

	navigateTo("/")
}

async function logOut(silent = false) {
	loading.value = "logout"

	const res = await safeSignOut()
	if (res.error) {
		if (!silent) {
			showToastMessage(
				"error",
				t("onboarding.accept-invite.errors.signout-failed"),
			)
		}

		// reset only on error
		loading.value = null

		return
	}

	navigateTo({ name: "login" })
}
</script>
<template>
	<main
		class="flex h-dvh min-h-180 min-w-dvw items-center justify-center bg-background text-foreground"
	>
		<div
			v-if="isAuthenticated"
			class="fixed top-0 left-0 flex w-full items-center justify-between"
		>
			<ShadcnUiButton
				size="2sm"
				variant="ghost"
				:disabled="loading === 'logout'"
				:class="
					cn(
						'mt-6 ml-8 text-muted-foreground',
						loading === 'accept' && 'pointer-events-none',
					)
				"
				@click="logOut"
			>
				{{ $t("onboarding.accept-invite.login-info.logout") }}
			</ShadcnUiButton>
			<div
				class="mt-6 mr-8 flex max-w-[40dvw] min-w-0 flex-col gap-0.5 md:max-w-76"
			>
				<div class="text-xs text-muted-foreground">
					{{ $t("onboarding.accept-invite.login-info.logged-in-as") }}
				</div>
				<div
					class="min-w-0 truncate text-2sm font-medium text-accent-foreground"
				>
					{{ fetchAuthSession.state.value.data?.data?.user.email }}
				</div>
			</div>
		</div>
		<div
			class="mx-4 flex w-90 flex-col items-center gap-4 rounded-lg border border-border bg-card p-5"
		>
			<div class="flex flex-col items-center gap-4">
				<Icon name="custom-icons:main-logo" class="size-12" />
				<i18n-t
					keypath="onboarding.accept-invite.title"
					tag="div"
					class="text-center text-lg font-normal"
				>
					<template #name>
						<span class="font-semibold">{{ inviteInfo.inviter }}</span>
					</template>
					<template #org>
						<span class="font-semibold">{{ inviteInfo.orgName }}</span>
					</template>
				</i18n-t>
				<i18n-t
					keypath="onboarding.accept-invite.description"
					tag="div"
					class="text-center text-2sm font-normal text-muted-foreground"
				>
					<template #email>
						<span class="font-semibold">{{ inviteInfo.email }}</span>
					</template>
				</i18n-t>
			</div>
			<ShadcnUiButton
				type="button"
				size="lg"
				:class="
					cn('h-10 w-[90%]', loading === 'logout' && 'pointer-events-none')
				"
				:disabled="loading === 'accept'"
				@click="
					() => {
						isAuthenticated ? acceptInvite() : navigateToSignupWithRedirect()
					}
				"
			>
				<Icon
					v-show="loading === 'accept'"
					name="svg-spinners:blocks-shuffle-3"
					class="size-4"
				/>
				{{
					isAuthenticated
						? $t("onboarding.accept-invite.accept-button")
						: $t("onboarding.accept-invite.signup-button")
				}}
			</ShadcnUiButton>
		</div>
	</main>
</template>
