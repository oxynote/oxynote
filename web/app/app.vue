<script setup lang="ts">
import { Toaster } from "#layers/ui/app/components/shadcn/ui/sonner"

const { color } = useAppearance() // used for side effects
</script>
<template>
	<Html :class="{ dark: color.theme === 'dark' }">
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
