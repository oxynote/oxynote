import stringify from "safe-stable-stringify"

// the regexp pattern of the first half of any RFC 3339 date.
const dateFormat = /^\d{4}-\d{2}-\d{2}T\d{2}/

export function jsonReviver(_: any, value: any): any {
	// the date strings need to be converted to proper date
	// objects
	if (typeof value === "string" && dateFormat.test(value)) {
		return new Date(value)
	}

	return value
}

export function jsonStableStringify(value: any): string {
	return stringify(value) ?? ""
}
