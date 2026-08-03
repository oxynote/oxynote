<script lang="ts" setup>
import { nodeViewProps, NodeViewWrapper, NodeViewContent } from "@tiptap/vue-3"
import { cn } from "~/lib/utils"

defineProps(nodeViewProps)
</script>

<template>
	<NodeViewWrapper
		as="div"
		data-type="metric-grid"
		:data-diff-status="node.attrs.diffStatus"
		:class="cn('metric-grid @container')"
	>
		<NodeViewContent
			:class="
				cn(
					'metric-grid-content grid [grid-auto-flow:row] gap-3',
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

/* in case we leave an empty metric grid, we don't want it to take up space */
.metric-grid:not(:has(> .metric-grid-content > *)) {
	@apply hidden;
}

/* metric block size spans - responsive to container width */
/* default (2 columns): all sizes span full width */
.metric-block-compact,
.metric-block-standard,
.metric-block-wide {
	@apply col-span-2;
}

/* @xl (4 columns): compact/standard = half, wide = full */
@container (width >= 36rem) {
	.metric-block-compact,
	.metric-block-standard {
		@apply col-span-2;
	}

	.metric-block-wide {
		@apply col-span-4;
	}
}

/* @5xl (6 columns): compact = 1/3, standard = 1/2, wide = full */
@container (width >= 64rem) {
	.metric-block-compact {
		@apply col-span-2;
	}

	.metric-block-standard {
		@apply col-span-3;
	}

	.metric-block-wide {
		@apply col-span-6;
	}
}
</style>
