import { AxiosHeaders } from "axios"

type HeaderEntry = [string, string]

interface ForEachHeaders {
	forEach(callback: (value: unknown, key: string) => void): void
}

// a header value is a string, or a list of them. Anything else is not
// something a transport produced, and stringifying it would put
// "[object Object]" on the wire — so it is dropped instead.
function normalizeHeaderValue(value: unknown): string | null {
	if (value == null) {
		return null
	}

	if (Array.isArray(value)) {
		const parts = (value as unknown[])
			.map(normalizeHeaderValue)
			.filter((part): part is string => part !== null)

		return parts.length > 0 ? parts.join(", ") : null
	}

	if (typeof value === "string") {
		return value
	}

	if (
		typeof value === "number" ||
		typeof value === "boolean" ||
		typeof value === "bigint"
	) {
		return String(value)
	}

	return null
}

function hasForEach(value: object): value is ForEachHeaders {
	return typeof (value as Partial<ForEachHeaders>).forEach === "function"
}

function isEntryIterable(value: object): value is Iterable<[string, unknown]> {
	return (
		typeof (value as Partial<Iterable<unknown>>)[
			Symbol.iterator
		] === "function"
	)
}

// hocuspocus hands the request headers through in whatever shape the
// underlying transport produced, so every accepted form is normalized to a
// list of pairs before anything reads them. Returns null when the input is
// not header-shaped at all, which is what makes the raw-request fallback in
// toHeaders reachable.
export function collectHeaderEntries(
	requestHeaders: unknown,
): HeaderEntry[] | null {
	if (!requestHeaders || typeof requestHeaders !== "object") {
		return null
	}

	const entries: HeaderEntry[] = []

	// WHATWG Headers or compatible
	if (hasForEach(requestHeaders)) {
		requestHeaders.forEach((v, k) => {
			const value = normalizeHeaderValue(v)
			if (value != null) {
				entries.push([k, value])
			}
		})

		return entries
	}

	// iterable header entries
	if (isEntryIterable(requestHeaders)) {
		for (const [k, v] of requestHeaders) {
			const value = normalizeHeaderValue(v)
			if (value != null) {
				entries.push([k, value])
			}
		}

		return entries
	}

	// plain object map. Only enumerable string keys: a symbol key
	// stringifies to "Symbol(x)", which Headers.append rejects as an
	// invalid header name — turning a tolerant reader into a throw.
	const map = requestHeaders as Record<string, unknown>
	for (const k of Object.keys(map)) {
		const value = normalizeHeaderValue(map[k])
		if (value != null) {
			entries.push([k, value])
		}
	}

	return entries
}

function property(source: unknown, key: string): unknown {
	if (!source || typeof source !== "object") {
		return undefined
	}

	return (source as Record<string, unknown>)[key]
}

export function toHeaders(req: unknown, requestHeaders: unknown): Headers {
	const res = new Headers()
	const entries = collectHeaderEntries(requestHeaders)

	if (entries) {
		for (const [k, v] of entries) {
			res.append(k, v)
		}

		return res
	}

	// fallback: Node IncomingMessage.rawHeaders (always present on Node),
	// a flat list alternating key and value
	const rawHeaders = property(req, "rawHeaders")
	if (Array.isArray(rawHeaders)) {
		const raw = rawHeaders as unknown[]
		for (let i = 0; i < raw.length; i += 2) {
			const key = normalizeHeaderValue(raw[i])
			const value = normalizeHeaderValue(raw[i + 1])

			if (key != null && value != null) {
				res.append(key, value)
			}
		}

		return res
	}

	// last resort: regular req.headers
	for (const [k, v] of collectHeaderEntries(property(req, "headers")) ??
		[]) {
		res.append(k, v)
	}

	return res
}

export function toAxiosHeaders(requestHeaders: unknown): AxiosHeaders {
	const res = new AxiosHeaders()
	const entries = collectHeaderEntries(requestHeaders)

	if (entries) {
		for (const [k, v] of entries) {
			res.set(k, v)
		}

		return res
	}

	return res
}
