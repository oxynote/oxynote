import DOMPurify from "dompurify"

/**
 * Sanitizes HTML content, allowing only <mark> tags for highlighting search results.
 * This ensures user-generated content is safe while preserving search result highlighting.
 *
 * @param html - The HTML string to sanitize
 * @returns Sanitized HTML string with only <mark> tags allowed
 */
export function sanitizeSearchResult(html: string): string {
	if (!html) {
		return ""
	}

	// Check if we're in a browser environment
	if (typeof window === "undefined") {
		// On server side, strip all HTML tags as a fallback
		return html.replace(/<[^>]*>/g, "")
	}

	return DOMPurify.sanitize(html, {
		ALLOWED_TAGS: ["mark"],
		ALLOWED_ATTR: [],
		KEEP_CONTENT: true,
	})
}
