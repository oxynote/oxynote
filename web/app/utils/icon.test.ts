import type { IconifyJSON } from "@iconify/types"
import { icons as cib } from "@iconify-json/cib"
import { icons as deviconPlain } from "@iconify-json/devicon-plain"
import { icons as fileIcons } from "@iconify-json/file-icons"
import { icons as fluent } from "@iconify-json/fluent"
import { icons as mdi } from "@iconify-json/mdi"
import { icons as mingcute } from "@iconify-json/mingcute"
import { icons as ph } from "@iconify-json/ph"
import { icons as ri } from "@iconify-json/ri"
import { icons as simpleIcons } from "@iconify-json/simple-icons"
import { describe, it } from "vitest"
import { selectableIconList, selectableIcons } from "./icon"

const collections: Record<string, IconifyJSON | undefined> = {
	cib,
	"devicon-plain": deviconPlain,
	"file-icons": fileIcons,
	fluent,
	mdi,
	mingcute,
	ph,
	ri,
	"simple-icons": simpleIcons,
}

describe("selectableIconList", () => {
	it("returns the id of every selectable icon in order", ({ expect }) => {
		const list = selectableIconList()

		expect(list).toEqual(selectableIcons.map((v) => v.id))
		expect(list.length).toBeGreaterThan(0)
	})

	it("lists every icon once", ({ expect }) => {
		const ids = selectableIconList()

		expect(new Set(ids).size).toBe(ids.length)
	})

	it.for(selectableIconList())(
		"names %s after a glyph its installed collection ships",
		(id, { expect }) => {
			const [prefix, name] = id.split(":") as [string, string]
			const collection = collections[prefix]

			expect(collection).toBeDefined()
			expect(
				name in (collection?.icons ?? {}) ||
					name in (collection?.aliases ?? {}),
			).toBe(true)
		},
	)
})
