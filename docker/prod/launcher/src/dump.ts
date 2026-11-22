import { createReadStream } from "node:fs"
import { Readable } from "node:stream"
import { createGunzip } from "node:zlib"

export interface Script {
	before?: string
	after?: string
	dropTransactionTimeout?: boolean
}

// dumpScript streams a gzipped dump, unzipped, between before and after.
export function dumpScript(path: string, script: Script = {}): Readable {
	const sql = createReadStream(path).pipe(createGunzip())

	return Readable.from(
		(async function* () {
			yield script.before ?? ""
			yield* script.dropTransactionTimeout
				? withoutTransactionTimeout(sql)
				: sql
			yield script.after ?? ""
		})(),
	)
}

// pg_dump 17 and later set transaction_timeout, which older servers reject,
// in the preamble they write first. It is far shorter than preambleBytes.
const transactionTimeout = Buffer.from("SET transaction_timeout = 0;\n")
const preambleBytes = 65536

async function* withoutTransactionTimeout(
	sql: AsyncIterable<unknown>,
): AsyncGenerator<Buffer> {
	const head: Buffer[] = []
	let size = 0
	let past = false

	// the dump yields buffers.
	for await (const chunk of sql as AsyncIterable<Buffer>) {
		if (past) {
			yield chunk

			continue
		}

		head.push(chunk)
		size += chunk.length

		if (size >= preambleBytes) {
			past = true
			yield withoutLine(Buffer.concat(head))
		}
	}

	if (!past) {
		yield withoutLine(Buffer.concat(head))
	}
}

function withoutLine(head: Buffer): Buffer {
	const at = head.indexOf(transactionTimeout)

	return at === -1
		? head
		: Buffer.concat([
				head.subarray(0, at),
				head.subarray(at + transactionTimeout.length),
			])
}
