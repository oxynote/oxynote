import { common } from "lowlight"
import { describe, it } from "vitest"
import {
	defaultExtendedCodeBlockLanguage,
	extendedCodeBlockLanguageOptions,
	extendedCodeBlockLanguages,
} from "./languages"

describe("extendedCodeBlockLanguages", () => {
	it("adds curl to lowlight's common grammar set", ({ expect }) => {
		expect(extendedCodeBlockLanguages.curl).toBeTypeOf("function")
		expect(extendedCodeBlockLanguages.javascript).toBeTypeOf("function")
		expect(extendedCodeBlockLanguages.yaml).toBeTypeOf("function")
	})

	it("leaves lowlight's shared common record untouched", ({ expect }) => {
		expect(common).not.toBe(extendedCodeBlockLanguages)
		expect(common.curl).toBeUndefined()
	})
})

describe("extendedCodeBlockLanguageOptions", () => {
	it("lists every registered grammar", ({ expect }) => {
		expect(Object.keys(extendedCodeBlockLanguageOptions).toSorted()).toEqual(
			Object.keys(extendedCodeBlockLanguages).toSorted(),
		)
	})

	it("orders the entries alphabetically by grammar key", ({ expect }) => {
		const keys = Object.keys(extendedCodeBlockLanguageOptions)

		expect(keys).toEqual(keys.toSorted())
	})

	it("gives every grammar a display name", ({ expect }) => {
		const unnamed = Object.entries(extendedCodeBlockLanguageOptions)
			.filter(([, name]) => name === undefined)
			.map(([key]) => key)

		expect(unnamed).toEqual([])
	})

	it.for([
		{ key: "cpp", expected: "C++" },
		{ key: "csharp", expected: "C#" },
		{ key: "curl", expected: "cURL" },
		{ key: "objectivec", expected: "Objective-C" },
		{ key: "php-template", expected: "PHP Template" },
		{ key: "vbnet", expected: "VB.Net" },
		{ key: "wasm", expected: "WebAssembly" },
		{ key: "xml", expected: "XML / HTML" },
	])("labels $key as $expected", ({ key, expected }, { expect }) => {
		expect(extendedCodeBlockLanguageOptions[key]).toBe(expected)
	})
})

describe("defaultExtendedCodeBlockLanguage", () => {
	it("names a registered grammar", ({ expect }) => {
		expect(defaultExtendedCodeBlockLanguage).toBe("plaintext")
		expect(
			extendedCodeBlockLanguages[defaultExtendedCodeBlockLanguage],
		).toBeTypeOf("function")
	})
})
