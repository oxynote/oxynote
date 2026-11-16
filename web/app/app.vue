<script setup lang="ts">
import { Toaster } from "#layers/ui/app/components/shadcn/ui/sonner"

const { color } = useAppearance() // used for side effects

onMounted(() => {
	document.documentElement.classList.toggle(
		"dark",
		color.value.theme === "dark",
	)
})
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
