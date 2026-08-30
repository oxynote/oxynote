import { pino, type DestinationStream } from "pino"

// the levels are core's slog levels, spelled the same way, so one
// OXYNOTE_LOG_LEVEL means the same thing on both sides of the deployment.
export type LogLevel = "DEBUG" | "INFO" | "WARN" | "ERROR"

// what the service logs through, and all it needs: a message at one of four
// levels. It is the narrow port pino satisfies, so a test takes four
// vi.fn()s rather than standing up a logger to assert against.
export interface Logger {
	debug(message: string): void
	info(message: string): void
	warn(message: string): void
	error(message: string): void
}

// createLogger builds the pino instance every line goes through, shaped to
// match what core's slog handler emits — an ISO timestamp, the level as an
// uppercase word, the message under "msg" — so one deployment's two
// services produce one log format rather than two.
//
// base is null because pino would otherwise stamp every line with a pid and
// a hostname, and neither says anything about a container running one
// process under an id docker assigned. destination is a parameter because
// writing to stdout is a side effect, and those belong to the composition
// root; a test passes a stream it can read back.
export function createLogger(
	level: LogLevel,
	destination: DestinationStream,
): Logger {
	return pino(
		{
			level: level.toLowerCase(),
			base: null,
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
