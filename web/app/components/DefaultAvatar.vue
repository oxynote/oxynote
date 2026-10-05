<script lang="ts" setup>
// always use this as <LazyDefaultAvatar>. The avatar library then loads
// only when a default avatar is shown.
const props = defineProps<{
	kind: AvatarKind
	seed?: string
	// whose picture this is, for the text a screen reader will read
	name?: string | null
	// the name is already written beside the picture, so a screen reader
	// skips the picture instead of reading the name twice
	decorative?: boolean
}>()

const { t } = useI18n({ useScope: "global" })

const label = computed(() => {
	if (props.decorative) {
		return ""
	}

	if (props.kind === "deleted-user") {
		return t("general.default-avatar.deleted-user")
	}

	if (props.kind === "invited-user") {
		return t("general.default-avatar.invited-user")
	}

	if (props.kind === "organization") {
		return props.name
			? t("general.default-avatar.organization", { name: props.name })
			: t("general.default-avatar.organization-unnamed")
	}

	return props.name
		? t("general.default-avatar.user", { name: props.name })
		: t("general.default-avatar.user-unnamed")
})
const src = computed(() => defaultAvatar(props.kind, props.seed))
</script>
<template>
	<!--
	the picture takes the corners of the tile it sits in. The tile clips
	its content, but a browser can let an animated picture escape that clip
	-->
	<img :src="src" :alt="label" class="size-full rounded-[inherit]" />
</template>
