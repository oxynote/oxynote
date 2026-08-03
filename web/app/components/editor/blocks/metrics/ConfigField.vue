<script setup lang="ts"></script>
<template>
	<div class="flex flex-col">
		<div class="mb-1 text-left text-2sm font-medium text-foreground">
			<slot name="label" />
		</div>
		<div class="flex flex-col gap-1">
			<slot />
		</div>
	</div>
</template>
