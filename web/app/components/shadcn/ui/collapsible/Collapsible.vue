<script setup lang="ts">
import type { CollapsibleRootEmits, CollapsibleRootProps } from "reka-ui"
import { CollapsibleRoot, useForwardPropsEmits } from "reka-ui"

const props = defineProps<CollapsibleRootProps>()
const emits = defineEmits<CollapsibleRootEmits>()

const forwarded = useForwardPropsEmits(props, emits)
</script>

<template>
	<!-- the slot exposes reka-ui's live open state, which diverges from the
		open prop in uncontrolled usage — hence the distinct name -->
	<CollapsibleRoot
		v-slot="{ open: isOpen }"
		data-slot="collapsible"
		v-bind="forwarded"
	>
		<slot :open="isOpen" />
	</CollapsibleRoot>
</template>
