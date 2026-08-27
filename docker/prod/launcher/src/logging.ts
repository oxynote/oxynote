export interface LinePrefixer {
	data(chunk: Buffer | string): void
	end(): void
}

// createLinePrefixer buffers a child's raw output chunks and emits complete
// lines tagged with the child's name, so the four processes' logs stay
// tellable apart in the single container stream. A trailing fragment is
// held until its newline arrives and flushed at stream end, which keeps the
// last line of a crashing child — usually the one that matters — from
// being lost.
export function createLinePrefixer(
	prefix: string,
	write: (line: string) => void,
): LinePrefixer {
	let buffer = ""

	return {
		data(chunk) {
			buffer += chunk.toString()

			for (;;) {
				const newline = buffer.indexOf("\n")

				if (newline === -1) {
					break
				}

				write(`${prefix} ${buffer.slice(0, newline)}`)
				buffer = buffer.slice(newline + 1)
			}
		},

		end() {
			if (buffer === "") {
				return
			}

			write(`${prefix} ${buffer}`)
			buffer = ""
		},
	}
}
