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
		<div class="flex w-95 flex-col items-center gap-5 px-2">
			<div class="flex flex-col items-center gap-6">
				<Icon name="custom-icons:main-logo" class="size-12" />
				<i18n-t
					:keypath="
						pageRoute.query.sent
							? 'onboarding.verify-email.sent-title'
							: 'onboarding.verify-email.title'
					"
					tag="div"
					class="text-center text-lg font-semibold"
				>
					<template #email>{{ pageRoute.query.new }}</template>
				</i18n-t>
			</div>
			<NuxtLink
				v-if="!pageRoute.query.sent"
				to="/"
				class="text-base font-semibold text-muted-foreground hover:opacity-70 active:opacity-50"
			>
				{{ $t("onboarding.verify-email.button") }}
			</NuxtLink>
			<NuxtLink
				v-if="pageRoute.query.sent"
				:to="{ name: 'signup' }"
				class="text-xs font-semibold text-accent-foreground hover:opacity-70 active:opacity-50"
			>
				{{ $t("onboarding.verify-email.back-to-signup") }}
			</NuxtLink>
		</div>
	</main>
</template>
