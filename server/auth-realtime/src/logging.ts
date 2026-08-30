import { pino, type DestinationStream } from "pino"

// the levels are core's slog levels, spelled the same way, so one
// OXYNOTE_LOG_LEVEL means the same thing on both sides of the deployment.
export type LogLevel = "DEBUG" | "INFO" | "WARN" | "ERROR"

export interface Logger {
	debug(message: string): void
	info(message: string): void
	warn(message: string): void
	error(message: string): void
}

// the record shape matches core's slog output, so both services in a
// deployment log alike. A null base drops pino's pid and hostname, which
// name nothing inside a single-process container.
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
