<script setup lang="ts">
import { showToastMessage } from "~/components/toast"

// this page is the OAuth consent step (better-auth's consentPage): the
// provider redirects here with a signed authorization query, the
// logged-in user decides, and the decision endpoint answers with the
// redirect that continues the flow. Logged-out users bounce through
// /login and return with the query intact via ?next=. Web-only by
// design — MCP clients open the flow in the system browser.
definePageMeta({
	name: "oauth-consent",
	middleware: (to) => {
		if (!to.query.client_id || typeof to.query.client_id !== "string") {
			return navigateTo("/")
		}
	},
})

const { t } = useI18n({ useScope: "global" })
useHead({
	title: () => t("oauth.consent.page-title"),
})

const route = useRoute()
const { fetchPublicClient, decideConsent } = useMCPAPI()

const clientId = computed(() => route.query.client_id as string)
const clientName = ref<string | null>(null)
const deciding = ref<"approve" | "deny" | null>(null)

const displayName = computed(
	() => clientName.value ?? clientId.value ?? t("oauth.consent.unknown-client"),
)

const requestedScopes = computed(() => {
	const scope = route.query.scope

	if (typeof scope !== "string") {
		return []
	}

	return scope.split(" ").filter(Boolean)
})

// scope identifiers use a colon, which i18n keypaths reserve, so the
// lookup key swaps it for a dash.
function scopeLabel(scope: string) {
	const key = `oauth.consent.scopes.${scope.replace(":", "-")}`
	const label = t(key)

	return label === key ? scope : label
}

onMounted(async () => {
	try {
		const client = await fetchPublicClient(clientId.value)
		clientName.value = client.client_name ?? null
	} catch {
		// the consent card falls back to the client id; the decision
		// endpoint is the one that actually validates the request.
	}
})

async function decide(accept: boolean) {
	deciding.value = accept ? "approve" : "deny"

	try {
		const res = await decideConsent(accept, window.location.search)

		const redirect = res.url ?? res.redirect_uri
		if (!redirect) {
			showToastMessage("error", t("oauth.consent.errors.invalid-request"))
			deciding.value = null

			return
		}

		// a full navigation: the redirect target is the client's own
		// redirect_uri (or the provider continuing the flow), not a
		// route of this app.
		window.location.href = redirect
	} catch {
		showToastMessage("error", t("oauth.consent.errors.decision-failed"))
		deciding.value = null
	}
}
</script>

<template>
	<main
		class="flex h-dvh min-h-120 min-w-dvw items-center justify-center bg-background text-foreground"
	>
		<div
			class="flex w-full max-w-md flex-col gap-5 rounded-md border bg-muted/50 p-6"
		>
			<div class="flex flex-col gap-1.5">
				<i18n-t
					keypath="oauth.consent.title"
					tag="h1"
					class="text-lg font-semibold"
				>
					<template #client>
						<span>{{ displayName }}</span>
					</template>
				</i18n-t>
				<i18n-t
					keypath="oauth.consent.description"
					tag="p"
					class="text-2sm text-muted-foreground"
				>
					<template #client>
						<span class="font-medium">{{ displayName }}</span>
					</template>
				</i18n-t>
			</div>

			<div v-if="requestedScopes.length > 0" class="flex flex-col gap-2">
				<div class="text-2sm font-medium">
					{{ $t("oauth.consent.scopes-title") }}
				</div>
				<ul class="flex flex-col gap-1.5">
					<li
						v-for="scope in requestedScopes"
						:key="scope"
						class="flex items-center gap-2 text-2sm text-muted-foreground"
					>
						<Icon name="lucide:check" class="size-4 shrink-0" />
						{{ scopeLabel(scope) }}
					</li>
				</ul>
			</div>

			<div class="flex justify-end gap-2">
				<ShadcnUiButton
					variant="outline"
					:disabled="deciding !== null"
					@click="decide(false)"
				>
					{{ $t("oauth.consent.deny") }}
				</ShadcnUiButton>
				<ShadcnUiButton :disabled="deciding !== null" @click="decide(true)">
					{{ $t("oauth.consent.approve") }}
				</ShadcnUiButton>
			</div>
		</div>
	</main>
</template>
