import slugify from "slugify"

const MAX_SLUG_LENGTH = 20

export function isXid(id: string): boolean {
	return id.length === 20
}

export function isOptimisticInsertId(id: string): boolean {
	return id.startsWith("optimistic-")
}

export function createNameSlug(name: string): string {
	if (!name.length) {
		return ""
	}

	return slugify(name, {
		replacement: "-",
		lower: false,
		strict: true,
	})
		.slice(0, MAX_SLUG_LENGTH)
		.replace(/-+$/, "")
}

export function createNameSlugWithId(name: string, id: string): string {
	// default length of https://github.com/rs/xid
	if (!isXid(id)) {
		throw new Error(
			"the ID must have exactly 20 characters to be included in a slug",
		)
	}

	if (!name.length) {
		return id
	}

	const slugName = slugify(name, {
		replacement: "-",
		lower: false,
		strict: true,
	})
		.slice(0, MAX_SLUG_LENGTH)
		.replace(/-+$/, "")

	if (!slugName.length) {
		return id
	}

	return `${slugName}-${id}`
}

export function equalNameSlugs(nameA: string, nameB: string): boolean {
	if (nameA === nameB) {
		// short-circuit for already slugged or full names
		return true
	}

	const slugA = slugify(nameA, {
		replacement: "-",
		lower: false,
		strict: true,
	})
		.slice(0, MAX_SLUG_LENGTH)
		.replace(/-+$/, "")
	const slugB = slugify(nameB, {
		replacement: "-",
		lower: false,
		strict: true,
	})
		.slice(0, MAX_SLUG_LENGTH)
		.replace(/-+$/, "")

	return slugA === slugB
}

export function extractDocInfoFromSlug(
	slug: string,
): { id: string; name?: string } | undefined {
	const parts = slug.split("-")
	if (parts.length < 2) {
		const [onlyPart] = parts
		if (onlyPart !== undefined && isXid(onlyPart)) {
			return { id: onlyPart }
		}

		return undefined
	}

	const name = parts.slice(0, -1).join("-")
	const id = parts[parts.length - 1] ?? ""

	// default length of https://github.com/rs/xid
	if (isXid(id)) {
		return { id, name }
	}

	return undefined
}

export function ensureHttps(url: string): string {
	if (!/^https?:\/\//i.test(url)) {
		return "https://" + url
	}

	return url
}

export function extractDomain(url: string): string {
	try {
		const normalized = ensureHttps(url) // in case protocol is missing
		const hostname = new URL(normalized).hostname
		const parts = hostname.split(".")

		// we need only domain + TLD
		if (parts.length >= 2) {
			return parts.slice(-2).join(".")
		}

		return hostname // fallback for localhost or similar
	} catch {
		return url
	}
}

export function addDeletionSuccessStatusToUrl(url: string): string {
	const urlObj = new URL(url)
	urlObj.searchParams.set("deletion", "success")

	return urlObj.toString()
}

export function isDeletionSuccessInUrl(url: string): boolean {
	try {
		const urlObj = new URL(url)
		return urlObj.searchParams.get("deletion") === "success"
	} catch {
		return false
	}
}
