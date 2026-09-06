// the visual family a file belongs to: what icon and accent the card
// shows, decided by extension first and content type second
export type FileKind =
	| "pdf"
	| "video"
	| "audio"
	| "image"
	| "archive"
	| "text"
	| "code"
	| "office"
	| "generic"

export interface FileKindStyle {
	icon: string
	accentClass: string
}

const EXTENSION_KINDS: Record<string, FileKind> = {
	pdf: "pdf",
	mp4: "video",
	webm: "video",
	mov: "video",
	mkv: "video",
	avi: "video",
	m4v: "video",
	mp3: "audio",
	wav: "audio",
	ogg: "audio",
	m4a: "audio",
	flac: "audio",
	aac: "audio",
	png: "image",
	jpg: "image",
	jpeg: "image",
	gif: "image",
	webp: "image",
	svg: "image",
	bmp: "image",
	tif: "image",
	tiff: "image",
	heic: "image",
	zip: "archive",
	tar: "archive",
	gz: "archive",
	tgz: "archive",
	bz2: "archive",
	xz: "archive",
	"7z": "archive",
	rar: "archive",
	txt: "text",
	md: "text",
	markdown: "text",
	rtf: "text",
	log: "text",
	js: "code",
	mjs: "code",
	cjs: "code",
	ts: "code",
	jsx: "code",
	tsx: "code",
	vue: "code",
	go: "code",
	py: "code",
	rb: "code",
	rs: "code",
	java: "code",
	kt: "code",
	swift: "code",
	c: "code",
	h: "code",
	cpp: "code",
	cs: "code",
	php: "code",
	sh: "code",
	json: "code",
	yaml: "code",
	yml: "code",
	toml: "code",
	xml: "code",
	html: "code",
	css: "code",
	sql: "code",
	csv: "code",
	doc: "office",
	docx: "office",
	xls: "office",
	xlsx: "office",
	ppt: "office",
	pptx: "office",
	odt: "office",
	ods: "office",
	odp: "office",
	key: "office",
	pages: "office",
	numbers: "office",
}

const CONTENT_TYPE_KINDS: Record<string, FileKind> = {
	"application/pdf": "pdf",
	"application/zip": "archive",
	"application/gzip": "archive",
	"application/x-gzip": "archive",
	"application/x-tar": "archive",
	"application/x-bzip2": "archive",
	"application/x-xz": "archive",
	"application/x-7z-compressed": "archive",
	"application/x-rar-compressed": "archive",
	"application/vnd.rar": "archive",
	"text/plain": "text",
	"text/markdown": "text",
	"text/rtf": "text",
	"application/rtf": "text",
	"application/json": "code",
	"application/xml": "code",
	"application/javascript": "code",
	"text/javascript": "code",
	"application/msword": "office",
	"application/vnd.ms-excel": "office",
	"application/vnd.ms-powerpoint": "office",
}

const FILE_KIND_STYLES: Record<FileKind, FileKindStyle> = {
	pdf: { icon: "lucide:file-text", accentClass: "bg-red-500/10 text-red-500" },
	video: {
		icon: "lucide:file-video-camera",
		accentClass: "bg-purple-500/10 text-purple-500",
	},
	audio: {
		icon: "lucide:file-music",
		accentClass: "bg-pink-500/10 text-pink-500",
	},
	image: {
		icon: "lucide:file-image",
		accentClass: "bg-emerald-500/10 text-emerald-500",
	},
	archive: {
		icon: "lucide:file-archive",
		accentClass: "bg-amber-500/10 text-amber-500",
	},
	text: {
		icon: "lucide:file-type",
		accentClass: "bg-slate-500/10 text-slate-500",
	},
	code: {
		icon: "lucide:file-code",
		accentClass: "bg-sky-500/10 text-sky-500",
	},
	office: {
		icon: "lucide:file-box",
		accentClass: "bg-blue-500/10 text-blue-500",
	},
	generic: {
		icon: "lucide:file",
		accentClass: "bg-muted text-muted-foreground",
	},
}

// the types a browser renders rather than runs, mirrored from the
// server's allowlist: the server serves these inline and the card opens
// them in a new tab, everything else is a download
const VIEWABLE_TYPES = new Set([
	"application/pdf",
	"image/png",
	"image/jpeg",
	"image/gif",
	"image/webp",
	"text/plain",
])

function mediaType(contentType: string | null | undefined): string {
	return (contentType ?? "").split(";")[0]?.trim().toLowerCase() ?? ""
}

function extension(name: string | null | undefined): string {
	const trimmed = (name ?? "").trim()
	const dot = trimmed.lastIndexOf(".")

	if (dot <= 0 || dot === trimmed.length - 1) {
		return ""
	}

	return trimmed.slice(dot + 1).toLowerCase()
}

export function fileKind(
	name: string | null | undefined,
	contentType: string | null | undefined,
): FileKind {
	const byExtension = EXTENSION_KINDS[extension(name)]

	if (byExtension) {
		return byExtension
	}

	const type = mediaType(contentType)
	const byType = CONTENT_TYPE_KINDS[type]

	if (byType) {
		return byType
	}

	if (type.startsWith("video/")) {
		return "video"
	}

	if (type.startsWith("audio/")) {
		return "audio"
	}

	if (type.startsWith("image/")) {
		return "image"
	}

	if (type.startsWith("text/")) {
		return "code"
	}

	if (
		type.includes("officedocument") ||
		type.includes("opendocument") ||
		type.startsWith("application/vnd.ms-")
	) {
		return "office"
	}

	return "generic"
}

export function fileKindStyle(kind: FileKind): FileKindStyle {
	return FILE_KIND_STYLES[kind]
}

export function isViewable(contentType: string | null | undefined): boolean {
	const type = mediaType(contentType)

	return (
		VIEWABLE_TYPES.has(type) ||
		type.startsWith("video/") ||
		type.startsWith("audio/")
	)
}

export function formatFileSize(bytes: number | null | undefined): string {
	if (typeof bytes !== "number" || !Number.isFinite(bytes) || bytes < 0) {
		return ""
	}

	if (bytes < 1024) {
		return `${bytes} B`
	}

	const kb = bytes / 1024

	if (kb < 1024) {
		return `${kb.toFixed(1)} KB`
	}

	return `${(kb / 1024).toFixed(1)} MB`
}
