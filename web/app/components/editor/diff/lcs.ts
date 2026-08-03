/**
 * compute the longest common subsequence of two arrays.
 * returns an array of matched index pairs [indexInA, indexInB].
 *
 * uses standard dynamic programming — O(n*m) time and space.
 * sufficient for document-level block alignment (rarely > a few hundred blocks).
 */
export function lcs<T>(
	a: T[],
	b: T[],
	eq: (x: T, y: T) => boolean = (x, y) => x === y,
): [number, number][] {
	const n = a.length
	const m = b.length

	// build DP table as a flat Uint32Array instead of a nested number[][].
	// a nested array allocates n+1 separate Array objects on the heap,
	// each holding m+1 boxed JS numbers — all of which the GC must
	// individually track and collect. a single Uint32Array is one
	// contiguous allocation with unboxed values, so the GC frees one
	// object instead of scanning n+2. this matters during frequent
	// recomputes (every 250ms–1.5s) in collaborative editing.
	const cols = m + 1
	const dp = new Uint32Array((n + 1) * cols)

	for (let i = 1; i <= n; i++) {
		for (let j = 1; j <= m; j++) {
			if (eq(a[i - 1]!, b[j - 1]!)) {
				dp[i * cols + j] = dp[(i - 1) * cols + (j - 1)]! + 1
			} else {
				dp[i * cols + j] = Math.max(
					dp[(i - 1) * cols + j]!,
					dp[i * cols + (j - 1)]!,
				)
			}
		}
	}

	// backtrack to find the matched pairs, preferring earliest positions
	// in both a and b. when a match exists but skipping it yields the
	// same LCS length, we skip to find the match at an earlier index.
	const result: [number, number][] = []
	let i = n
	let j = m
	while (i > 0 && j > 0) {
		if (
			eq(a[i - 1]!, b[j - 1]!) &&
			dp[i * cols + j] !== dp[i * cols + (j - 1)] &&
			dp[i * cols + j] !== dp[(i - 1) * cols + j]
		) {
			result.push([i - 1, j - 1])
			i--
			j--
		} else if (dp[(i - 1) * cols + j]! > dp[i * cols + (j - 1)]!) {
			i--
		} else {
			j--
		}
	}

	result.reverse()
	return result
}
