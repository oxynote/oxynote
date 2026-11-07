<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper, NodeViewContent } from "@tiptap/vue-3"
import { cn } from "#layers/ui/app/lib/utils"

defineProps(nodeViewProps)
</script>

<template>
	<NodeViewWrapper
		as="div"
		data-type="block-grid"
		:data-diff-status="node.attrs.diffStatus"
		:class="cn('block-grid @container')"
	>
		<NodeViewContent
			:class="
				cn(
					'block-grid-content grid [grid-auto-flow:row] gap-3',
					'grid-cols-[repeat(2,minmax(0,1fr))]',
					'@xl:grid-cols-[repeat(4,minmax(0,1fr))]',
					'@5xl:grid-cols-[repeat(6,minmax(0,1fr))]',
				)
			"
		/>
	</NodeViewWrapper>
</template>

<style global lang="css">
@reference "@/assets/css/main.css";

/* in case we leave an empty grid, we don't want it to take up space */
.block-grid:not(:has(> .block-grid-content > *)) {
	@apply hidden;
}

/* grid item size spans - responsive to container width */
/* default (2 columns): all sizes span full width */
.grid-item-compact,
.grid-item-standard,
.grid-item-wide {
	@apply col-span-2;
}

/* @xl (4 columns): compact/standard = half, wide = full */
@container (width >= 36rem) {
	.grid-item-compact,
	.grid-item-standard {
		@apply col-span-2;
	}

	.grid-item-wide {
		@apply col-span-4;
	}
}

/* @5xl (6 columns): compact = 1/3, standard = 1/2, wide = full */
@container (width >= 64rem) {
	.grid-item-compact {
		@apply col-span-2;
	}

	.grid-item-standard {
		@apply col-span-3;
	}

	.grid-item-wide {
		@apply col-span-6;
	}
}
</style>
