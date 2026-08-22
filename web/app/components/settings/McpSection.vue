<script lang="ts" setup>
import { cn } from "~/lib/utils"
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
	switch (scope) {
		case "documents:read":
			return t("settings.mcp.scopes.documents-read")
		case "documents:write":
			return t("settings.mcp.scopes.documents-write")
	}

	// a scope this build has no name for is shown verbatim rather than
	// dropped: the client was granted it either way.
	return scope
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
	<div class="flex flex-col">
		<div
			class="flex w-full flex-col justify-between gap-2 sm:flex-row sm:items-center"
		>
			<div class="flex flex-col gap-0.5">
				<div class="text-2base">
					{{ $t("settings.mcp.endpoint-label") }}
				</div>
				<div class="text-xs text-muted-foreground">
					{{ $t("settings.mcp.endpoint-description") }}
				</div>
			</div>
			<div
				class="flex w-full items-center justify-between gap-1.5 sm:max-w-44 sm:min-w-44 sm:justify-end md:max-w-52 md:min-w-0 lg:max-w-70"
			>
				<code class="min-w-0 truncate font-mono text-2sm">
					{{ endpointURL }}
				</code>
				<ShadcnUiButton
					type="button"
					class="size-6 shrink-0 bg-transparent p-0"
					variant="outline"
					@click="copyEndpoint"
				>
					<Icon name="lucide:copy" size="0.75rem" />
					<span class="sr-only">
						{{ $t("settings.mcp.copy-button-screen-reader-hint") }}
					</span>
				</ShadcnUiButton>
			</div>
		</div>
		<div class="my-3.5 h-px w-full bg-border" />
		<div class="flex w-full flex-col gap-0.75">
			<i18n-t keypath="settings.mcp.clients-label" tag="div" class="text-2base">
				<template #count>{{ consents.length }}</template>
			</i18n-t>
			<div v-if="consents.length === 0" class="text-2sm text-muted-foreground">
				{{ $t("settings.mcp.no-clients") }}
			</div>
			<table v-else class="w-full table-fixed">
				<colgroup>
					<col class="w-auto md:w-25 lg:w-30" />
					<col class="hidden md:table-column md:w-9 lg:w-14" />
					<col class="w-12 lg:w-16" />
				</colgroup>
				<tbody class="divide-y divide-border/70 dark:divide-border/50">
					<tr v-for="consent in consents" :key="consent.id">
						<td class="py-2">
							<div class="flex min-w-0 flex-col">
								<div class="truncate text-sm">
									{{ clientLabel(consent.clientId) }}
								</div>
								<div class="truncate text-2sm text-muted-foreground">
									{{ consent.scopes.map(scopeLabel).join(", ") }}
								</div>
							</div>
						</td>
						<td class="hidden md:table-cell">
							<div class="flex min-w-0 flex-col">
								<div class="text-2sm text-muted-foreground">
									{{ $t("settings.mcp.authorized-label") }}
								</div>
								<div class="text-2sm text-muted-foreground">
									{{ $d(new Date(consent.createdAt), "short") }}
								</div>
							</div>
						</td>
						<td>
							<div
								class="flex h-full items-center justify-end whitespace-nowrap"
							>
								<ShadcnUiButton
									type="button"
									variant="ghost-plain-destructive"
									:class="cn('p-0 text-sm')"
									@click="revoke(consent.id)"
								>
									{{ $t("settings.mcp.revoke") }}
								</ShadcnUiButton>
							</div>
						</td>
					</tr>
				</tbody>
			</table>
		</div>
	</div>
</template>
