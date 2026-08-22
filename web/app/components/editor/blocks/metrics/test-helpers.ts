// shared helpers for the metric block component suites. Test-only: the
// app/**/test-helpers.ts coverage exclude keeps this out of the
// denominator, and nothing here is imported by app code.
export { stubThemeColorContext as stubChartColorContext } from "../../test-helpers/theme"

// the threshold palette is read straight off the root element's CSS
// variables, which no stylesheet defines under happy-dom. Distinct values
// let a suite tell one swatch from another.
export function stubThresholdPalette(): string[] {
	const colors = Array.from(
		{ length: 16 },
		(_value, index) => `#${(index + 1).toString(16).padStart(2, "0")}0000`,
	)

	colors.forEach((color, index) => {
		document.documentElement.style.setProperty(
			`--chart-threshold-${index + 1}`,
			color,
		)
	})

	return colors
}
