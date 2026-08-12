<script lang="ts" setup>
definePageMeta({
	middleware: async (to) => {
		if (!to.query.new || typeof to.query.new !== "string") {
			return navigateTo("/")
		}
	},
})

const { t } = useI18n({ useScope: "global" })
useHead({
	title: () => t("general.verify-email-page-title"),
})
const pageRoute = useRoute()
</script>
<template>
	<main
		class="flex min-h-svh min-w-svw items-center justify-center bg-background text-foreground"
	>
		<div class="flex w-67 flex-col items-center gap-5">
			<div class="flex flex-col items-center gap-6">
				<Icon name="custom-icons:main-logo" class="size-12" />
				<div class="text-lg font-semibold">
					{{
						pageRoute.query.sent
							? $t("onboarding.verify-email.sent-heading")
							: $t("onboarding.verify-email.heading")
					}}
				</div>
			</div>
			<div class="flex w-full flex-col gap-3">
				<i18n-t
					:keypath="
						pageRoute.query.sent
							? 'onboarding.verify-email.sent-title'
							: 'onboarding.verify-email.title'
					"
					tag="div"
					class="text-center text-xs text-accent-foreground"
				>
					<template #email>{{ pageRoute.query.new }}</template>
				</i18n-t>
				<ShadcnUiButton
					v-if="pageRoute.query.sent"
					type="button"
					size="lg"
					variant="ghost"
					class="h-10 w-full text-muted-foreground"
					@click="navigateTo({ name: 'login' })"
				>
					{{ $t("onboarding.verify-email.back-to-login") }}
				</ShadcnUiButton>
				<ShadcnUiButton
					v-else
					type="button"
					size="lg"
					variant="ghost"
					class="h-10 w-full text-muted-foreground"
					@click="navigateTo('/')"
				>
					{{ $t("onboarding.verify-email.button") }}
				</ShadcnUiButton>
			</div>
		</div>
	</main>
</template>
