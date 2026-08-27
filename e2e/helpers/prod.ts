import { spawn } from "node:child_process"
import { request as httpRequest } from "node:http"
import { BASE_URL } from "./config"
import { composeArgs, composeCwd } from "./stack"

// the production image's internal port layout, mirroring
// docker/prod/launcher/src/mapping.ts. Caddy (8080) is the only
// wildcard listener; the three services behind it bind 127.0.0.1, and
// caddy's own admin API is switched off rather than bound anywhere.
//
// Each entry carries a path that answers 200 when the service is reached,
// so a probe that gets through is unambiguous rather than a bare 404.
export const INTERNAL_SERVICES = [
	{ name: "core", port: "8180", path: "/api/x/version" },
	{ name: "auth-realtime", port: "8181", path: "/api/auth-config" },
	{ name: "web", port: "3000", path: "/login" },
	{ name: "caddy admin", port: "2019", path: "/config/" },
] as const

// the front door of the container, reached the same way as the ports above.
// It is the positive control for every probe: the probe container can
// resolve and reach the image, so a refusal on 8180 is a bind and not a
// broken network.
export const FRONT_DOOR = { port: "8080", path: "/login" }

// the compose service the probe runs in, and the one it dials.
const PROBE_SERVICE = "probe"
const TARGET_SERVICE = "oxynote"

export interface ProbeResult {
	// whether the target answered at the HTTP level at all. A refused
	// connection and an HTTP error are both failures for wget, and only
	// the second one means the surface was reachable.
	reached: boolean
	detail: string
}

// probeFromNetwork asks a sibling container on the private network to fetch
// a URL from the image, which is the question the trust boundary actually
// answers: not "is this port published" — none of them are — but "can a
// container that is not caddy reach an unauthenticated internal surface".
//
// It is the same check the launcher runs at boot from inside the container,
// asked from the outside, so a regression that removed the boot gate as
// well as the loopback bind still fails here.
export async function probeFromNetwork(
	port: string,
	path: string,
): Promise<ProbeResult> {
	const url = `http://${TARGET_SERVICE}:${port}${path}`
	const { code, output } = await runCompose([
		"exec",
		"-T",
		PROBE_SERVICE,
		"wget",
		"-q",
		"-O",
		"-",
		"-T",
		"3",
		url,
	])

	// busybox wget exits nonzero for a refused connection and for an HTTP
	// error alike, and only its message separates them. "server returned
	// error" means the TCP connection was accepted and a response came
	// back — the surface was reachable, whatever it answered.
	const reached = code === 0 || /server returned error/i.test(output)

	return {
		reached,
		detail: `${url} -> ${output.trim() || `exit ${String(code)}`}`,
	}
}

export interface RawResponse {
	status: number
	headers: Record<string, string | string[] | undefined>
	body: string
}

// rawRequest sends a path to the front door exactly as written, without a
// URL parser in the way.
//
// This is what the bypass cases need and what fetch() cannot do: the WHATWG
// parser resolves "..", collapses duplicate slashes and re-encodes as it
// builds the request, so a test written against fetch would send caddy a
// path that had already been normalised on the client and would prove
// nothing about caddy's own normalisation. Redirects are not followed, for
// the same reason — the status is the answer.
export function rawRequest(path: string, method = "GET"): Promise<RawResponse> {
	const { hostname, port } = new URL(BASE_URL)

	return new Promise((resolve, reject) => {
		const req = httpRequest(
			{ hostname, port, path, method, timeout: 10_000 },
			(res) => {
				let body = ""

				res.setEncoding("utf8")
				res.on("data", (chunk: string) => {
					body += chunk
				})
				res.on("end", () => {
					resolve({
						status: res.statusCode ?? 0,
						headers: res.headers,
						body,
					})
				})
			},
		)

		req.on("timeout", () => {
			req.destroy(new Error(`no answer for ${method} ${path}`))
		})
		req.on("error", reject)
		req.end()
	})
}

interface CommandResult {
	code: number
	output: string
}

// runCompose drives the production stack's compose project. It never
// rejects on a nonzero exit: the probe reads the failure, and a rejection
// would turn the expected outcome into an error.
function runCompose(args: string[]): Promise<CommandResult> {
	const child = spawn("docker", [...composeArgs(), ...args], {
		cwd: composeCwd,
		stdio: ["ignore", "pipe", "pipe"],
	})

	let output = ""

	child.stdout.setEncoding("utf8")
	child.stderr.setEncoding("utf8")
	child.stdout.on("data", (chunk: string) => {
		output += chunk
	})
	child.stderr.on("data", (chunk: string) => {
		output += chunk
	})

	return new Promise((resolve, reject) => {
		child.on("error", reject)
		child.on("close", (code) => {
			resolve({ code: code ?? -1, output })
		})
	})
}
