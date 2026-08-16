function cssVar(name: string, el: HTMLElement = document.documentElement) {
	return getComputedStyle(el).getPropertyValue(name).trim()
}

// converts a CSS variable's color value to hex.
// the canvas is never added to the DOM; it only exists in memory for color parsing.
// we paint a pixel and read it back because modern Chromium round-trips
// newer color formats like oklch through `fillStyle` unchanged, and downstream
// consumers (mermaid, echarts) only accept hex/rgb.
let colorCtx: CanvasRenderingContext2D | null = null
function cssColorHex(name: string) {
	if (!colorCtx) {
		const canvas = document.createElement("canvas")
		canvas.width = 1
		canvas.height = 1
		// eslint-disable-next-line @typescript-eslint/no-non-null-assertion -- a fresh canvas always yields a 2d context; it is only null when the type is unsupported or already bound to another one
		colorCtx = canvas.getContext("2d", { willReadFrequently: true })!
	}
	colorCtx.clearRect(0, 0, 1, 1)
	colorCtx.fillStyle = cssVar(name)
	colorCtx.fillRect(0, 0, 1, 1)
	const [r, g, b, a] = colorCtx.getImageData(0, 0, 1, 1).data as unknown as [
		number,
		number,
		number,
		number,
	]
	const hex = (n: number) => n.toString(16).padStart(2, "0")
	return a === 255
		? `#${hex(r)}${hex(g)}${hex(b)}`
		: `rgba(${r}, ${g}, ${b}, ${(a / 255).toFixed(3)})`
}

const rightArrowPath = `path://M8.59 14.59L13.17 10 8.59 5.41 10 4l6 6-6 6z`
const leftArrowPath = `path://M15.41 14.59L10.83 10l4.58-4.59L14 4l-6 6 6 6z`

export function chartStyles() {
	return {
		fontFamily: "Inter",
		dataColors: [
			cssColorHex("--chart-data-1"),
			cssColorHex("--chart-data-2"),
			cssColorHex("--chart-data-3"),
			cssColorHex("--chart-data-4"),
			cssColorHex("--chart-data-5"),
			cssColorHex("--chart-data-6"),
			cssColorHex("--chart-data-7"),
			cssColorHex("--chart-data-8"),
			cssColorHex("--chart-data-9"),
			cssColorHex("--chart-data-10"),
			cssColorHex("--chart-data-11"),
			cssColorHex("--chart-data-12"),
			cssColorHex("--chart-data-13"),
			cssColorHex("--chart-data-14"),
			cssColorHex("--chart-data-15"),
			cssColorHex("--chart-data-16"),
		],
		lineChart: {
			background: cssVar("--background"),
			text: cssVar("--foreground"),
			mutedText: cssVar("--muted-foreground-50"),
			gridLine: cssVar("--border"),
			tooltipLine: cssVar("--ring"),
			tooltipBackground: cssVar("--popover"),
			tooltipText: cssVar("--popover-foreground"),
			tooltipBorder: cssVar("--border"),
			legendScrollButton: cssVar("--foreground"),
			legendScrollButtonInactive: cssVar("--muted-foreground-50"),
			rightArrow: rightArrowPath,
			leftArrow: leftArrowPath,
		},
		gaugeChart: {
			background: cssVar("--background"),
			text: cssVar("--foreground"),
			mutedText: cssVar("--muted-foreground-50"),
			emptyGauge: cssVar("--muted-foreground-7"),
		},
		thresholdColors: {
			default: cssVar("--chart-threshold-6"),
			available: [
				cssVar("--chart-threshold-1"),
				cssVar("--chart-threshold-2"),
				cssVar("--chart-threshold-3"),
				cssVar("--chart-threshold-4"),
				cssVar("--chart-threshold-5"),
				cssVar("--chart-threshold-6"),
				cssVar("--chart-threshold-7"),
				cssVar("--chart-threshold-8"),
				cssVar("--chart-threshold-9"),
				cssVar("--chart-threshold-10"),
				cssVar("--chart-threshold-11"),
				cssVar("--chart-threshold-12"),
				cssVar("--chart-threshold-13"),
				cssVar("--chart-threshold-14"),
				cssVar("--chart-threshold-15"),
				cssVar("--chart-threshold-16"),
			],
		},
	}
}

export function mermaidThemeColors() {
	return {
		background: cssColorHex("--background"),
		foreground: cssColorHex("--foreground"),
		card: cssColorHex("--card"),
		muted: cssColorHex("--muted"),
		mutedForeground: cssColorHex("--muted-foreground"),
		accent: cssColorHex("--accent"),
		border: cssColorHex("--border"),
		primary: cssColorHex("--primary"),
		primaryForeground: cssColorHex("--primary-foreground"),
		destructive: cssColorHex("--destructive"),
		destructiveForeground: cssColorHex("--destructive-foreground"),
		chart: [
			cssColorHex("--chart-data-1"),
			cssColorHex("--chart-data-2"),
			cssColorHex("--chart-data-3"),
			cssColorHex("--chart-data-4"),
			cssColorHex("--chart-data-5"),
			cssColorHex("--chart-data-6"),
			cssColorHex("--chart-data-7"),
			cssColorHex("--chart-data-8"),
			cssColorHex("--chart-data-9"),
			cssColorHex("--chart-data-10"),
			cssColorHex("--chart-data-11"),
			cssColorHex("--chart-data-12"),
		],
		fontFamily: cssVar("--font-inter"),
	}
}

export function editorCaretColors() {
	const names = [
		"red",
		"orange",
		"amber",
		"yellow",
		"lime",
		"green",
		"emerald",
		"teal",
		"cyan",
		"sky",
		"blue",
		"indigo",
		"violet",
		"purple",
		"fuchsia",
		"pink",
		"rose",
	]
	const shades = [500, 600, 700, 800]

	return names.flatMap((name) =>
		shades.map((shade) => ({
			label: `${name}-${shade}`,
			value: cssColorHex(`--caret-${name}-${shade}`),
		})),
	)
}
