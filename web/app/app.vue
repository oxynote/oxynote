<script setup lang="ts">
import { Toaster } from "@/components/shadcn/ui/sonner"

const { color } = useAppearance() // used for side effects
</script>
<template>
	<Html :class="color.theme === 'dark' ? 'dark' : undefined">
		<Body>
			<Toaster
				class="pointer-events-auto"
				:toast-options="{
					duration: 5000,
				}"
				position="bottom-right"
			/>
			<NuxtPage />
		</Body>
	</Html>
</template>
