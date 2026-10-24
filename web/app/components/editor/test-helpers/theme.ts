// helpers for the suites that mount components reading the theme's chart
// or caret colours. Test-only: the app/**/test-helpers/** coverage
// exclude keeps this out of the denominator.
import { vi } from "vitest"

// the colour helpers in ~/assets/css resolve CSS variables by painting
// them onto a canvas, and happy-dom hands out no 2d context — every
// colour lookup would throw. The stand-in reports one flat colour, which
// is enough for suites that only care about which colour slot was used.
export function stubThemeColorContext() {
	const context = {
		clearRect: () => undefined,
		fillRect: () => undefined,
		fillStyle: "",
		getImageData: () => ({ data: [0, 0, 0, 255] }),
	}

	vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
		context as unknown as CanvasRenderingContext2D,
	)
}
