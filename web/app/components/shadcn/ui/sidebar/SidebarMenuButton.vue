<script setup lang="ts">
import type { Component } from "vue"
import type { SidebarMenuButtonProps } from "./SidebarMenuButtonChild.vue"
import { reactiveOmit } from "@vueuse/core"
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from "@/components/shadcn/ui/tooltip"
import SidebarMenuButtonChild from "./SidebarMenuButtonChild.vue"
import { useSidebar } from "./utils"

defineOptions({
	inheritAttrs: false,
})

const props = withDefaults(
	// eslint-disable-next-line @typescript-eslint/no-unsafe-argument -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
	defineProps<
		// eslint-disable-next-line @typescript-eslint/no-redundant-type-constituents -- eslint's ts program resolves .vue imports as error typed, vue-tsc accepts this
		SidebarMenuButtonProps & {
			tooltip?: string | Component
		}
	>(),
	{
		as: "button",
		variant: "default",
		size: "default",
	},
)

const { isMobile, state } = useSidebar()

const delegatedProps = reactiveOmit(props, "tooltip")
</script>

<template>
	<SidebarMenuButtonChild
		v-if="!tooltip"
		v-bind="{ ...delegatedProps, ...$attrs }"
	>
		<slot />
	</SidebarMenuButtonChild>

	<Tooltip v-else>
		<TooltipTrigger as-child>
			<SidebarMenuButtonChild v-bind="{ ...delegatedProps, ...$attrs }">
				<slot />
			</SidebarMenuButtonChild>
		</TooltipTrigger>
		<TooltipContent
			side="right"
			align="center"
			:hidden="state !== 'collapsed' || isMobile"
		>
			<template v-if="typeof tooltip === 'string'">
				{{ tooltip }}
			</template>
			<component :is="tooltip" v-else />
		</TooltipContent>
	</Tooltip>
</template>
