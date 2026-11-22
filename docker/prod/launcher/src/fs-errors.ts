export function isNoEntry(err: unknown): boolean {
	return err instanceof Error && "code" in err && err.code === "ENOENT"
}

export function isExists(err: unknown): boolean {
	return err instanceof Error && "code" in err && err.code === "EEXIST"
}
