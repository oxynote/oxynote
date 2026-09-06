import { describe, it } from "vitest"
import {
	fileKind,
	fileKindStyle,
	formatFileSize,
	isViewable,
} from "./file-kind"

describe("fileKind", () => {
	it.for([
		{ name: "report.pdf", contentType: null, expected: "pdf" },
		{ name: "clip.mp4", contentType: null, expected: "video" },
		{ name: "clip.MOV", contentType: null, expected: "video" },
		{ name: "song.mp3", contentType: null, expected: "audio" },
		{ name: "photo.jpeg", contentType: null, expected: "image" },
		{ name: "logo.svg", contentType: null, expected: "image" },
		{ name: "notes.zip", contentType: null, expected: "archive" },
		{ name: "site.tar.gz", contentType: null, expected: "archive" },
		{ name: "backup.7z", contentType: null, expected: "archive" },
		{ name: "README.md", contentType: null, expected: "text" },
		{ name: "notes.txt", contentType: null, expected: "text" },
		{ name: "main.go", contentType: null, expected: "code" },
		{ name: "config.yaml", contentType: null, expected: "code" },
		{ name: "data.csv", contentType: null, expected: "code" },
		{ name: "deck.pptx", contentType: null, expected: "office" },
		{ name: "sheet.xlsx", contentType: null, expected: "office" },
		{ name: "letter.doc", contentType: null, expected: "office" },
		{ name: "thing.unknown", contentType: null, expected: "generic" },
		{ name: "noextension", contentType: null, expected: "generic" },
		{ name: ".gitignore", contentType: null, expected: "generic" },
		{ name: "trailingdot.", contentType: null, expected: "generic" },
	])(
		"classifies $name by its extension as $expected",
		({ name, contentType, expected }, { expect }) => {
			expect(fileKind(name, contentType)).toBe(expected)
		},
	)

	it.for([
		{ contentType: "application/pdf", expected: "pdf" },
		{ contentType: "video/webm", expected: "video" },
		{ contentType: "audio/ogg; codecs=opus", expected: "audio" },
		{ contentType: "image/heic", expected: "image" },
		{ contentType: "application/zip", expected: "archive" },
		{ contentType: "application/x-tar", expected: "archive" },
		{ contentType: "text/plain; charset=utf-8", expected: "text" },
		{ contentType: "text/markdown", expected: "text" },
		{ contentType: "application/json", expected: "code" },
		{ contentType: "text/html; charset=utf-8", expected: "code" },
		{
			contentType:
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			expected: "office",
		},
		{
			contentType: "application/vnd.oasis.opendocument.text",
			expected: "office",
		},
		{ contentType: "application/vnd.ms-excel", expected: "office" },
		{ contentType: "application/octet-stream", expected: "generic" },
		{ contentType: "", expected: "generic" },
		{ contentType: null, expected: "generic" },
	])(
		"falls back to the content type $contentType as $expected",
		({ contentType, expected }, { expect }) => {
			expect(fileKind("thing", contentType)).toBe(expected)
		},
	)

	it("lets the extension win over the content type", ({ expect }) => {
		expect(fileKind("report.pdf", "application/octet-stream")).toBe("pdf")
	})

	it("classifies a missing name by the content type", ({ expect }) => {
		expect(fileKind(null, "application/pdf")).toBe("pdf")
	})
})

describe("fileKindStyle", () => {
	it("gives every kind its own icon and accent", ({ expect }) => {
		const kinds = [
			"pdf",
			"video",
			"audio",
			"image",
			"archive",
			"text",
			"code",
			"office",
			"generic",
		] as const
		const styles = kinds.map((kind) => fileKindStyle(kind))

		expect(new Set(styles.map((style) => style.icon)).size).toBe(kinds.length)
		expect(new Set(styles.map((style) => style.accentClass)).size).toBe(
			kinds.length,
		)
		expect(styles.every((style) => style.icon.startsWith("lucide:"))).toBe(true)
	})

	it("styles the generic kind as a plain file", ({ expect }) => {
		expect(fileKindStyle("generic").icon).toBe("lucide:file")
	})
})

describe("isViewable", () => {
	it.for([
		{ contentType: "application/pdf", expected: true },
		{ contentType: "image/png", expected: true },
		{ contentType: "image/jpeg", expected: true },
		{ contentType: "image/gif", expected: true },
		{ contentType: "image/webp", expected: true },
		{ contentType: "video/mp4", expected: true },
		{ contentType: "audio/mpeg", expected: true },
		{ contentType: "text/plain; charset=utf-8", expected: true },
		{ contentType: "text/html; charset=utf-8", expected: false },
		{ contentType: "image/svg+xml", expected: false },
		{ contentType: "text/xml", expected: false },
		{ contentType: "text/javascript", expected: false },
		{ contentType: "application/zip", expected: false },
		{ contentType: "", expected: false },
		{ contentType: null, expected: false },
	])(
		"reports $contentType as viewable: $expected",
		({ contentType, expected }, { expect }) => {
			expect(isViewable(contentType)).toBe(expected)
		},
	)
})

describe("formatFileSize", () => {
	it.for([
		{ input: 0, expected: "0 B" },
		{ input: 512, expected: "512 B" },
		{ input: 1023, expected: "1023 B" },
		{ input: 1024, expected: "1.0 KB" },
		{ input: 1536, expected: "1.5 KB" },
		{ input: 2_516_582, expected: "2.4 MB" },
		{ input: 26_214_400, expected: "25.0 MB" },
	])("formats $input bytes as $expected", ({ input, expected }, { expect }) => {
		expect(formatFileSize(input)).toBe(expected)
	})

	it.for([
		{ name: "a negative size", input: -1 },
		{ name: "an infinite size", input: Number.POSITIVE_INFINITY },
		{ name: "a missing size", input: null },
		{ name: "an undefined size", input: undefined },
	])("formats $name as nothing", ({ input }, { expect }) => {
		expect(formatFileSize(input)).toBe("")
	})
})
