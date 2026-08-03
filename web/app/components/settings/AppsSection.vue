<script lang="ts" setup>
import { showToastMessage } from "../toast"

const { fetchGitHubConnectionStatus, fetchGitHubInstallURL, disconnectGitHub } =
	useGitHubAPI()
const { fetchSlackConnectionStatus, fetchSlackInstallURL, disconnectSlack } =
	useSlackAPI()
const { t } = useI18n({ useScope: "global" })
const { openExternalLink } = useExternalLinks()

async function installGitHub() {
	try {
		const result = await fetchGitHubInstallURL()
		openExternalLink(result.url)
	} catch {
		showToastMessage(
			"error",
			t("settings.apps.github.errors.install.title"),
			t("settings.apps.github.errors.install.description"),
		)
	}
}

async function installSlack() {
	try {
		const result = await fetchSlackInstallURL()
		openExternalLink(result.url)
	} catch {
		showToastMessage(
			"error",
			t("settings.apps.slack.errors.install.title"),
			t("settings.apps.slack.errors.install.description"),
		)
	}
}

async function disconnectGitHubApp() {
	try {
		await disconnectGitHub.mutateAsync()
	} catch {
		showToastMessage(
			"error",
			t("settings.apps.github.errors.disconnect.title"),
			t("settings.apps.github.errors.disconnect.description"),
		)
		return
	}
}

async function disconnectSlackApp() {
	try {
		await disconnectSlack.mutateAsync()
	} catch {
		showToastMessage(
			"error",
			t("settings.apps.slack.errors.disconnect.title"),
			t("settings.apps.slack.errors.disconnect.description"),
		)
		return
	}
}
</script>
<template>
	<div class="flex flex-col">
		<div class="flex w-full items-center justify-between gap-4">
			<div class="flex items-center gap-2">
				<Icon name="simple-icons:github" class="size-7 shrink-0" />
				<div class="flex flex-col">
					<div class="text-sm">
						{{ $t("settings.apps.github.title") }}
					</div>
					<div
						class="hidden text-2base text-muted-foreground md:block md:text-2sm"
					>
						{{ $t("settings.apps.github.description") }}
					</div>
				</div>
			</div>
			<div class="flex flex-col items-center justify-center self-stretch">
				<div
					v-if="fetchGitHubConnectionStatus.data.value?.connected"
					class="flex flex-col items-end justify-center self-stretch"
				>
					<div class="p-0 text-sm font-medium">
						{{ $t("settings.apps.github.connected") }}
					</div>
					<ShadcnUiButton
						type="button"
						variant="ghost-plain"
						size="custom"
						class="h-fit p-0 text-xs text-muted-foreground hover:text-muted-foreground/70! focus:text-muted-foreground/90!"
						@click="disconnectGitHubApp"
					>
						{{ $t("settings.apps.github.disconnect") }}
					</ShadcnUiButton>
				</div>
				<ShadcnUiButton
					v-else
					type="button"
					variant="ghost-plain"
					class="gap-1.5 p-0 text-sm"
					@click="installGitHub"
				>
					<Icon name="lucide:external-link" />
					{{ $t("settings.apps.github.connect") }}
				</ShadcnUiButton>
			</div>
		</div>
		<div class="my-3.5 h-px w-full bg-border" />
		<div class="flex w-full items-center justify-between gap-2">
			<div class="flex items-center gap-2">
				<Icon name="devicon:slack" class="size-6.5 shrink-0" />
				<div class="flex flex-col">
					<div class="text-sm">
						{{ $t("settings.apps.slack.title") }}
					</div>
					<div
						class="hidden text-2base text-muted-foreground md:block md:text-2sm"
					>
						{{ $t("settings.apps.slack.description") }}
					</div>
				</div>
			</div>
			<div>
				<div
					v-if="fetchSlackConnectionStatus.data.value?.connected"
					class="flex flex-col items-end justify-center self-stretch"
				>
					<div class="p-0 text-sm font-medium">
						{{ $t("settings.apps.slack.connected") }}
					</div>
					<ShadcnUiButton
						type="button"
						variant="ghost-plain"
						size="custom"
						class="h-fit p-0 text-xs text-muted-foreground hover:text-muted-foreground/70! focus:text-muted-foreground/90!"
						@click="disconnectSlackApp"
					>
						{{ $t("settings.apps.slack.disconnect") }}
					</ShadcnUiButton>
				</div>
				<ShadcnUiButton
					v-else
					type="button"
					variant="ghost-plain"
					class="gap-1.5 p-0 text-sm"
					@click="installSlack"
				>
					<Icon name="lucide:external-link" />
					{{ $t("settings.apps.slack.connect") }}
				</ShadcnUiButton>
			</div>
		</div>
	</div>
</template>
