import { stopStack } from "./helpers/stack"

// tears the stack down and drops its volumes, so the next run starts from
// an empty database rather than inheriting the accounts and workspaces
// this one created. make prints its own progress line for the step.
export default async function globalTeardown(): Promise<void> {
	await stopStack()
}
