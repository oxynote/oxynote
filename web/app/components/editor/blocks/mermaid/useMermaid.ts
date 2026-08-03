import { shallowRef, ref, readonly, type Ref } from "vue"
import type { MermaidConfig } from "mermaid"
import { mermaidThemeColors } from "~/assets/css"

type RenderMermaid = (
	id: string,
	source: string,
) => Promise<{ svg: string } | { error: string }>

const mermaidModule = shallowRef<typeof import("mermaid") | null>(null)
const isLoading = ref(false)
const loadError = ref<string | null>(null)
let loadPromise: Promise<void> | null = null
let lastInitializedDark: boolean | null = null

function buildMermaidConfig(dark: boolean): MermaidConfig {
	const c = mermaidThemeColors()

	return {
		startOnLoad: false,
		securityLevel: "strict",
		suppressErrorRendering: true,
		darkMode: dark,
		theme: "null",
		fontFamily: c.fontFamily,
		themeVariables: {
			background: c.background,
			fontFamily: c.fontFamily,
			primaryColor: c.card,
			primaryTextColor: c.foreground,
			secondaryColor: c.muted,
			secondaryTextColor: c.foreground,
			tertiaryColor: c.accent,
			tertiaryTextColor: c.foreground,
			primaryBorderColor: c.border,
			secondaryBorderColor: c.border,
			tertiaryBorderColor: c.border,
			noteBkgColor: c.accent,
			noteTextColor: c.foreground,
			noteBorderColor: c.border,
			lineColor: c.mutedForeground,
			arrowheadColor: c.mutedForeground,
			textColor: c.foreground,
			border2: c.border,
			nodeBkg: c.card,
			mainBkg: c.card,
			nodeBorder: c.border,
			clusterBkg: c.muted,
			clusterBorder: c.border,
			defaultLinkColor: c.mutedForeground,
			titleColor: c.foreground,
			edgeLabelBackground: c.background,
			nodeTextColor: c.foreground,
			// Sequence diagram
			actorBorder: c.border,
			actorBkg: c.card,
			actorTextColor: c.foreground,
			actorLineColor: c.mutedForeground,
			labelBoxBkgColor: c.card,
			signalColor: c.foreground,
			signalTextColor: c.foreground,
			labelBoxBorderColor: c.border,
			labelTextColor: c.foreground,
			loopTextColor: c.foreground,
			activationBorderColor: c.border,
			activationBkgColor: c.muted,
			sequenceNumberColor: c.primaryForeground,
			// Gantt
			sectionBkgColor: c.muted,
			altSectionBkgColor: c.background,
			sectionBkgColor2: c.card,
			excludeBkgColor: c.accent,
			taskBorderColor: c.border,
			taskBkgColor: c.card,
			activeTaskBorderColor: c.primary,
			activeTaskBkgColor: c.primary,
			gridColor: c.border,
			doneTaskBkgColor: c.muted,
			doneTaskBorderColor: c.border,
			critBorderColor: c.destructive,
			critBkgColor: c.destructive,
			todayLineColor: c.primary,
			vertLineColor: c.border,
			taskTextColor: c.foreground,
			taskTextOutsideColor: c.foreground,
			taskTextLightColor: c.primaryForeground,
			taskTextDarkColor: c.foreground,
			taskTextClickableColor: c.primary,
			// Person / C4
			personBorder: c.border,
			personBkg: c.card,
			// Tables
			rowOdd: c.card,
			rowEven: c.muted,
			// State diagram
			transitionColor: c.mutedForeground,
			transitionLabelColor: c.foreground,
			stateLabelColor: c.foreground,
			stateBkg: c.card,
			labelBackgroundColor: c.background,
			compositeBackground: c.background,
			altBackground: c.muted,
			compositeTitleBackground: c.card,
			compositeBorder: c.border,
			innerEndBackground: c.border,
			// Error
			errorBkgColor: c.destructive,
			errorTextColor: c.destructiveForeground,
			specialStateColor: c.primary,
			classText: c.foreground,
			// Pie chart
			pie1: c.chart[0],
			pie2: c.chart[1],
			pie3: c.chart[2],
			pie4: c.chart[3],
			pie5: c.chart[4],
			pie6: c.chart[5],
			pie7: c.chart[6],
			pie8: c.chart[7],
			pie9: c.chart[8],
			pie10: c.chart[9],
			pie11: c.chart[10],
			pie12: c.chart[11],
			pieTitleTextColor: c.foreground,
			pieSectionTextColor: c.foreground,
			pieLegendTextColor: c.foreground,
			pieStrokeColor: c.border,
			pieOuterStrokeColor: c.border,
			// Venn
			venn1: c.chart[0],
			venn2: c.chart[1],
			venn3: c.chart[2],
			venn4: c.chart[3],
			venn5: c.chart[4],
			venn6: c.chart[5],
			venn7: c.chart[6],
			venn8: c.chart[7],
			vennTitleTextColor: c.foreground,
			vennSetTextColor: c.foreground,
			// Color scales
			cScale0: c.chart[0],
			cScale1: c.chart[1],
			cScale2: c.chart[2],
			cScale3: c.chart[3],
			cScale4: c.chart[4],
			cScale5: c.chart[5],
			cScale6: c.chart[6],
			cScale7: c.chart[7],
			cScale8: c.chart[8],
			cScale9: c.chart[9],
			cScale10: c.chart[10],
			cScale11: c.chart[11],
			// Requirement diagram
			requirementBackground: c.card,
			requirementBorderColor: c.border,
			requirementTextColor: c.foreground,
			relationColor: c.mutedForeground,
			relationLabelBackground: c.background,
			relationLabelColor: c.foreground,
			// Git graph
			git0: c.chart[0],
			git1: c.chart[1],
			git2: c.chart[2],
			git3: c.chart[3],
			git4: c.chart[4],
			git5: c.chart[5],
			git6: c.chart[6],
			git7: c.chart[7],
			gitBranchLabel0: c.foreground,
			gitBranchLabel1: c.foreground,
			gitBranchLabel2: c.foreground,
			gitBranchLabel3: c.foreground,
			gitBranchLabel4: c.foreground,
			gitBranchLabel5: c.foreground,
			gitBranchLabel6: c.foreground,
			gitBranchLabel7: c.foreground,
			branchLabelColor: c.foreground,
			tagLabelColor: c.foreground,
			tagLabelBackground: c.muted,
			tagLabelBorder: c.border,
			commitLabelColor: c.foreground,
			commitLabelBackground: c.background,
			// Entity relationship
			attributeBackgroundColorOdd: c.card,
			attributeBackgroundColorEven: c.muted,
			// XY chart
			xyChart: {
				backgroundColor: c.background,
				titleColor: c.foreground,
				xAxisTitleColor: c.foreground,
				xAxisLabelColor: c.foreground,
				xAxisTickColor: c.foreground,
				xAxisLineColor: c.border,
				yAxisTitleColor: c.foreground,
				yAxisLabelColor: c.foreground,
				yAxisTickColor: c.foreground,
				yAxisLineColor: c.border,
				plotColorPalette: c.chart.join(","),
			},
			// Quadrant chart
			quadrant1Fill: c.card,
			quadrant2Fill: c.muted,
			quadrant3Fill: c.accent,
			quadrant4Fill: c.card,
			quadrant1TextFill: c.foreground,
			quadrant2TextFill: c.foreground,
			quadrant3TextFill: c.foreground,
			quadrant4TextFill: c.foreground,
			quadrantPointFill: c.primary,
			quadrantPointTextFill: c.primaryForeground,
			quadrantXAxisTextFill: c.foreground,
			quadrantYAxisTextFill: c.foreground,
			quadrantInternalBorderStrokeFill: c.border,
			quadrantExternalBorderStrokeFill: c.border,
			quadrantTitleFill: c.foreground,
			// Architecture
			archEdgeColor: c.mutedForeground,
			archEdgeArrowColor: c.mutedForeground,
			archGroupBorderColor: c.border,
			// Radar
			radar: {
				axisColor: c.mutedForeground,
				graticuleColor: c.border,
			},
			// User journey
			fillType0: c.chart[0],
			fillType1: c.chart[1],
			fillType2: c.chart[2],
			fillType3: c.chart[3],
			fillType4: c.chart[4],
			fillType5: c.chart[5],
			fillType6: c.chart[6],
			fillType7: c.chart[7],
		},
	} satisfies MermaidConfig
}

async function loadMermaid(dark: boolean) {
	if (mermaidModule.value) return
	if (loadPromise) {
		await loadPromise
		return
	}

	isLoading.value = true
	loadError.value = null

	loadPromise = import("mermaid")
		.then((mod) => {
			mermaidModule.value = mod
			mod.default.initialize(buildMermaidConfig(dark))
			lastInitializedDark = dark
		})
		.catch((err: unknown) => {
			loadError.value = err instanceof Error ? err.message : null
			loadPromise = null
		})
		.finally(() => {
			isLoading.value = false
		})

	await loadPromise
}

export function useMermaid(dark: Ref<boolean>) {
	const { t } = useI18n({ useScope: "global" })

	void loadMermaid(dark.value)

	const render: RenderMermaid = async (id, source) => {
		await loadMermaid(dark.value)

		if (!mermaidModule.value) {
			return {
				error: loadError.value ?? t("editor.mermaid.errors.load-failed"),
			}
		}

		try {
			if (lastInitializedDark !== dark.value) {
				mermaidModule.value.default.initialize(buildMermaidConfig(dark.value))
				lastInitializedDark = dark.value
			}

			const { svg } = await mermaidModule.value.default.render(id, source)

			return { svg }
		} catch (err: unknown) {
			return {
				error:
					err instanceof Error
						? err.message
						: t("editor.mermaid.errors.render-failed"),
			}
		}
	}

	return {
		render,
		isLoading: readonly(isLoading),
		loadError: readonly(loadError),
	}
}
