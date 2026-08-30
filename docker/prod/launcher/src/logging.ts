import { pino, type DestinationStream } from "pino"

// what the launcher logs its own events through: a message at one of three
// levels.
export interface Logger {
	info(message: string): void
	warn(message: string): void
	error(message: string): void
}

// createLogger builds the pino instance the launcher's own events go
// through, in the shape core and auth-realtime emit: an ISO timestamp, an
// uppercase level, the message under "msg", and the service field every
// record in the container stream carries. A child's output does not go
// through it — see childRecord, which adds that one field and touches
// nothing else.
//
// the level is fixed at info: the launcher's lines are the container's own
// lifecycle, a dozen per boot, and the "up at" line among them is the one an
// operator waits for — OXYNOTE_LOG_LEVEL lowers what the services
// themselves write, not whether the container narrates its start.
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

// childRecord renders one line of a child's output as one JSON record.
//
// a child that already logs JSON gets its object through as it wrote it,
// with one field added: service, naming the process the line came from. Its
// own level, time and fields are the ones that mean anything — stamping the
// launcher's on top would report when the relay ran rather than when the
// event happened, and rank a line the launcher never read. Anything else —
// a plain-text line, a JSON array, a bare number — becomes the msg of a
// record carrying only what the launcher does know: which child, and what
// it said.
//
// service is written last so it wins outright: the launcher is the one
// thing that knows for certain which process produced the line.
export function childRecord(service: string, line: string): string {
	return JSON.stringify({ ...(asObject(line) ?? { msg: line }), service })
}

// asObject answers with the line's own fields when it is a JSON object, and
// undefined for everything else. Only an object can open with a brace, so
// the guard settles both questions at once: what it lets through parses to
// an object or not at all, and the try stays off every plain-text line a
// child writes — an array or a bare number never reaches it.
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

// createLineSplitter buffers a child's raw output chunks and hands over
// complete lines, so each becomes one log record rather than whatever
// happened to arrive in one read. A trailing fragment is held until its
// newline arrives and flushed at stream end, which keeps the last line of a
// crashing child — usually the one that matters — from being lost.
//
// mute drops the lines it matches. It is for output a child gives no way to
// switch off — see the web spec in index.ts.
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
