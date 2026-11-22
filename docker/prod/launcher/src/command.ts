export type Command =
	| { name: "serve" }
	| { name: "backup" }
	| { name: "restore"; replace: boolean }

export const usage =
	"usage: backup > oxynote-backup.tar, or restore [--replace] < oxynote-backup.tar. Without an argument the image starts Oxynote."

// parseCommand reads the arguments after the launcher's own path, and throws
// the usage line for anything else.
export function parseCommand(args: string[]): Command {
	const line = args.join(" ")

	if (line === "") {
		return { name: "serve" }
	}

	if (line === "backup") {
		return { name: "backup" }
	}

	if (line === "restore" || line === "restore --replace") {
		return { name: "restore", replace: line !== "restore" }
	}

	throw new Error(usage)
}
