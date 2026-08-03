<script lang="ts" setup>
// eslint-disable-next-line
// @ts-nocheck // https://github.com/unovue/shadcn-vue/issues/1305#issuecomment-3030185319
import type { DrawerRootEmits, DrawerRootProps } from "vaul-vue"
import { useForwardPropsEmits } from "reka-ui"
import { DrawerRoot } from "vaul-vue"

const props = withDefaults(defineProps<DrawerRootProps>(), {
	shouldScaleBackground: true,
})

const emits = defineEmits<DrawerRootEmits>()

const forwarded = useForwardPropsEmits(props, emits)
</script>

<template>
	<DrawerRoot data-slot="drawer" v-bind="forwarded">
		<slot />
	</DrawerRoot>
</template>
