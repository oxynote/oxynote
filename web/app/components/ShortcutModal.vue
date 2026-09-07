<script lang="ts" setup>
import type { ShortcutEntry } from "~/utils/shortcuts"
import { allItems } from "./editor/slash/items"
import { SLASH_COMMAND_TRIGGER_CHAR } from "./editor/slash/extension"

const open = defineModel<boolean>({ default: false })

const { osType } = useDetectHost()
const { t } = useI18n({ useScope: "global" })

interface Row {
	id: string
	label: string
	description: string
	tokens: ReturnType<typeof extractShortcutKeys>
	// markdown and slash text is shown as typed, in a monospace face
	mono: boolean
}

// markdown that inserts a block is typed as written; a trailing space in
// the pattern is the space the rule waits for, shown as its own key
function typedTokens(pattern: string): Row["tokens"] {
	const keys = pattern.endsWith(" ")
		? [pattern.trimEnd(), t("shortcuts.modal.space-key")]
		: [pattern]

	return keys.map((key) => ({ key }))
}

function entryRow(entry: ShortcutEntry): Row {
	const base = {
		id: entry.i18nKey,
		label: t(entry.i18nKey),
		description: t(entry.descriptionI18nKey),
	}

	switch (entry.kind) {
		case "action":
			return {
				...base,
				tokens: extractShortcutKeys(
					shortcutByOS(entry.action.keyboardKey, osType.value),
				),
				mono: false,
			}
		case "keys":
			return {
				...base,
				tokens: extractShortcutKeys(
					shortcutByOS(entry.keyboardKey, osType.value),
				),
				mono: false,
			}
		case "typed":
			return { ...base, tokens: typedTokens(entry.pattern), mono: true }
	}
}

// the slash menu's items are the one place a block's name, description and
// markdown live, so both sections below read them from there
const markdownBlockRows = allItems
	.filter((item) => item.shortcut)
	.map((item): Row => ({
		id: `markdown-${item.titleI18nKey}`,
		label: t(item.titleI18nKey),
		description: `${t(item.descriptionI18nKey)} ${t("shortcuts.modal.markdown-block-hint")}`,
		tokens: typedTokens(item.shortcut ?? ""),
		mono: true,
	}))

const slashRows = allItems.map((item): Row => ({
	id: `slash-${item.titleI18nKey}`,
	label: t(item.titleI18nKey),
	description: `${t(item.descriptionI18nKey)} ${t("shortcuts.modal.slash-hint")}`,
	tokens: [
		{
			key: `${SLASH_COMMAND_TRIGGER_CHAR}${t(item.titleI18nKey).toLowerCase()}`,
		},
	],
	mono: true,
}))

const sections = computed(() => [
	...SHORTCUT_GROUPS.map((group) => ({
		id: group.id,
		title: t(group.i18nKey),
		rows: [
			...group.shortcuts.map(entryRow),
			...(group.id === "markdown" ? markdownBlockRows : []),
		],
	})),
	{
		id: "slash-commands",
		title: t("shortcuts.groups.slash-commands"),
		rows: slashRows,
	},
])
</script>
<template>
	<ShadcnUiDialog :open="open" @update:open="(val) => (open = val)">
		<ShadcnUiDialogContent
			class="max-h-[85dvh] w-[90dvw] overflow-y-auto p-0 text-foreground outline-none md:max-h-[70dvh] lg:w-200"
			@open-auto-focus.prevent
			@close-auto-focus.prevent
		>
			<div class="flex flex-col gap-8 p-6">
				<ShadcnUiDialogHeader>
					<ShadcnUiDialogTitle class="text-2xl">
						{{ $t("shortcuts.modal.title") }}
					</ShadcnUiDialogTitle>
					<ShadcnUiDialogDescription class="sr-only">
						{{ $t("shortcuts.modal.description") }}
					</ShadcnUiDialogDescription>
					<ShadcnUiButton
						variant="ghost-plain"
						class="absolute top-1/2 right-0 -translate-y-1/2 p-0"
						@click="open = false"
					>
						<Icon name="lucide:x" size="1.3rem" />
						<span class="sr-only">
							{{ $t("general.modal-close-screen-reader-hint") }}
						</span>
					</ShadcnUiButton>
				</ShadcnUiDialogHeader>
				<div
					v-for="section in sections"
					:key="section.id"
					:data-shortcut-section="section.id"
					class="flex flex-col gap-4"
				>
					<h3 class="text-2base">{{ section.title }}</h3>
					<div
						class="flex flex-col divide-y divide-border/70 rounded-md border bg-muted/50 px-4 py-1 dark:divide-border/50"
					>
						<div
							v-for="row in section.rows"
							:key="row.id"
							:data-shortcut-row="row.id"
							class="flex items-center justify-between gap-4 py-2.5"
						>
							<div class="flex min-w-0 flex-col gap-0.5">
								<div class="text-2base">{{ row.label }}</div>
								<div class="text-xs text-muted-foreground">
									{{ row.description }}
								</div>
							</div>
							<ShadcnUiKbdGroup class="shrink-0">
								<template v-for="(val, index) in row.tokens" :key="index">
									<ShadcnUiKbd
										v-if="!val.connector"
										:class="row.mono ? 'font-mono' : undefined"
									>
										{{ val.key }}
									</ShadcnUiKbd>
									<span
										v-else-if="val.connector === 'sequence'"
										class="text-xs text-muted-foreground"
									>
										{{ $t("shortcuts.connector") }}
									</span>
								</template>
							</ShadcnUiKbdGroup>
						</div>
					</div>
				</div>
			</div>
		</ShadcnUiDialogContent>
	</ShadcnUiDialog>
</template>
