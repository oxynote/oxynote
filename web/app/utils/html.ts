export function waitForHtmlElementById(id: string, timeoutMs: number) {
	if (typeof document === "undefined") {
		return Promise.resolve(null)
	}

	return new Promise<HTMLElement | null>((resolve) => {
		const start = performance.now()

		const check = () => {
			const el = document.getElementById(id)
			if (el) {
				resolve(el)
				return
			}

			if (performance.now() - start >= timeoutMs) {
				resolve(null)
				return
			}

			requestAnimationFrame(check)
		}

		check()
	})
}
