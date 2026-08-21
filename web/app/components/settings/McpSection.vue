<script lang="ts" setup>
import { showToastMessage } from "../toast"

const { fetchConsents, fetchPublicClient, revokeConsent } = useMCPAPI()
const { t } = useI18n({ useScope: "global" })
const config = useRuntimeConfig()

const endpointURL = computed(
	() => `${config.public.coreAPIBaseHttpURL}/api/mcp`,
)

const consents = computed(() => fetchConsents.data.value ?? [])

// client display names resolved lazily per consent; a failed lookup
// leaves the id-based fallback in place.
const clientNames = ref<Record<string, string>>({})

watch(
	consents,
	async (list) => {
		for (const consent of list) {
			if (clientNames.value[consent.clientId] !== undefined) {
				continue
			}

			try {
				const client = await fetchPublicClient(consent.clientId)

				if (client.client_name) {
					clientNames.value[consent.clientId] = client.client_name
				}
			} catch {
				// keep the fallback label; the list stays usable.
			}
		}
	},
	{ immediate: true },
)

function clientLabel(clientId: string) {
	return clientNames.value[clientId] ?? t("settings.mcp.unknown-client")
}

function scopeLabel(scope: string) {
	const key = `settings.mcp.scopes.${scope.replace(":", "-")}`
	const label = t(key)

	return label === key ? scope : label
}

function authorizedOn(createdAt: string) {
	return t("settings.mcp.authorized-on", {
		date: new Date(createdAt).toLocaleDateString(),
	})
}

async function copyEndpoint() {
	await navigator.clipboard.writeText(endpointURL.value)
	showToastMessage("success", t("settings.mcp.copied"))
}

async function revoke(id: string) {
	try {
		await revokeConsent.mutateAsync(id)
	} catch {
		showToastMessage(
			"error",
			t("settings.mcp.errors.revoke.title"),
			t("settings.mcp.errors.revoke.description"),
		)
	}
}
</script>
<template>
	<div class="flex flex-col gap-3.5">
		<div class="flex flex-col gap-2">
			<div class="text-2base text-muted-foreground md:text-2sm">
				{{ $t("settings.mcp.description") }}
			</div>
			<div class="flex items-center gap-2">
				<div class="text-sm">{{ $t("settings.mcp.endpoint-label") }}</div>
				<code
					class="rounded-sm bg-muted px-1.5 py-0.5 font-mono text-2sm text-muted-foreground"
				>
					{{ endpointURL }}
				</code>
				<ShadcnUiButton
					type="button"
					variant="ghost-plain"
					size="custom"
					class="h-fit gap-1 p-0 text-xs text-muted-foreground hover:text-muted-foreground/70!"
					@click="copyEndpoint"
				>
					<Icon name="lucide:copy" />
					{{ $t("settings.mcp.copy") }}
				</ShadcnUiButton>
			</div>
		</div>

		<div class="h-px w-full bg-border" />

		<div class="flex flex-col gap-2">
			<div class="text-sm">{{ $t("settings.mcp.connected-clients") }}</div>
			<div
				v-if="consents.length === 0"
				class="text-2base text-muted-foreground md:text-2sm"
			>
				{{ $t("settings.mcp.no-clients") }}
			</div>
			<div v-else class="flex flex-col">
				<template v-for="(consent, index) in consents" :key="consent.id">
					<div v-if="index > 0" class="my-2.5 h-px w-full bg-border" />
					<div class="flex w-full items-center justify-between gap-4">
						<div class="flex flex-col">
							<div class="text-sm">{{ clientLabel(consent.clientId) }}</div>
							<div class="text-2base text-muted-foreground md:text-2sm">
								{{ authorizedOn(consent.createdAt) }}
								·
								{{ consent.scopes.map(scopeLabel).join(", ") }}
							</div>
						</div>
						<ShadcnUiButton
							type="button"
							variant="ghost-plain"
							size="custom"
							class="h-fit p-0 text-xs text-muted-foreground hover:text-muted-foreground/70! focus:text-muted-foreground/90!"
							@click="revoke(consent.id)"
						>
							{{ $t("settings.mcp.revoke") }}
						</ShadcnUiButton>
					</div>
				</template>
			</div>
		</div>
	</div>
</template>
