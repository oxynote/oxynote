export interface LoopbackCheckDeps {
	// the container's externally reachable addresses (non-loopback).
	addresses(): string[]
	// attempts a TCP connect; resolves true when something accepted.
	connects(host: string, port: number): Promise<boolean>
}

// checkLoopbackExposure dials the container's own external addresses on the
// internal service ports. The services bind loopback, so every dial must be
// refused; an accepted connection means an unauthenticated internal surface
// is reachable from the network, and the boot must fail rather than serve.
// Returns the exposed address:port pairs — empty when the gate holds.
export async function checkLoopbackExposure(
	deps: LoopbackCheckDeps,
	ports: number[],
): Promise<string[]> {
	const exposed: string[] = []

	for (const address of deps.addresses()) {
		for (const port of ports) {
			if (await deps.connects(address, port)) {
				exposed.push(`${address}:${port}`)
			}
		}
	}

	return exposed
}
