// the image's layered health probe: every internal service must answer and
// the front door must route, so a hung or crashed child flips the container
// unhealthy even while the launcher process itself is alive. The ports
// mirror launcher/src/mapping.ts.
const checks = [
	"http://127.0.0.1:8180/api/x/version",
	"http://127.0.0.1:8181/api/auth-config",
	"http://127.0.0.1:3000/login",
	"http://127.0.0.1:8080/auth-realtime/api/auth-config",
]

const results = await Promise.all(
	checks.map(async (url) => {
		try {
			const res = await fetch(url, {
				signal: AbortSignal.timeout(5_000),
			})

			await res.body?.cancel()

			return res.status === 200
		} catch {
			return false
		}
	}),
)

if (results.includes(false)) {
	const failed = checks.filter((_, i) => !results[i])

	console.error(`unhealthy: ${failed.join(", ")}`)
	process.exit(1)
}
