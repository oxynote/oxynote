import { pino, type DestinationStream } from "pino"

export interface Logger {
	info(message: string): void
	warn(message: string): void
	error(message: string): void
}

// the launcher's own events; a child's line goes through childRecord
// instead. The shape matches what core and auth-realtime emit, and the
// level is fixed — OXYNOTE_LOG_LEVEL governs the services, not the
// container's account of its own boot.
export function createLogger(
	service: string,
	destination: DestinationStream,
): Logger {
	return pino(
		{
			level: "info",
			base: { service },
			timestamp: pino.stdTimeFunctions.isoTime,
			formatters: {
				level: (label) => ({
					level: label.toUpperCase(),
				}),
			},
		},
		destination,
	)
}

// childRecord renders one line of a child's output as one JSON record: the
// child's own object plus service, or {msg, service} for anything that is
// not one. Nothing else is added — the launcher's level and timestamp would
// describe the relay, not the event. service is written last, so it wins
// over a child that claims a different one.
export function childRecord(service: string, line: string): string {
	return JSON.stringify({ ...(asObject(line) ?? { msg: line }), service })
}

// only an object can open with a brace, so the guard both keeps the try off
// every plain-text line and rules out an array or a bare number.
function asObject(line: string): Record<string, unknown> | undefined {
	if (!line.startsWith("{")) {
		return undefined
	}

	try {
		return JSON.parse(line) as Record<string, unknown>
	} catch {
		return undefined
	}
}

export interface LineSplitter {
	data(chunk: Buffer | string): void
	end(): void
}

// buffers raw chunks into whole lines, so each becomes one log record. A
// trailing fragment is flushed at stream end, which keeps the last line of
// a crashing child — usually the one that matters. mute drops the lines it
// matches, for output a child gives no way to switch off.
export function createLineSplitter(
	emit: (line: string) => void,
	mute?: RegExp,
): LineSplitter {
	let buffer = ""

	function take(line: string): void {
		if (mute?.test(line)) {
			return
		}

		emit(line)
	}

	return {
		data(chunk) {
			buffer += chunk.toString()

			for (;;) {
				const newline = buffer.indexOf("\n")

				if (newline === -1) {
					break
				}

				take(buffer.slice(0, newline))
				buffer = buffer.slice(newline + 1)
			}
		},

		end() {
			if (buffer === "") {
				return
			}

			take(buffer)
			buffer = ""
		},
	}
}
