// compare the icons the web picker offers with core's copy, which the
// assistant validates icons against. Print what differs and exit 1 when
// they drift apart.
//
// usage: node scripts/check-icons.mjs
import { readFileSync } from "node:fs"
import { selectableIcons } from "../web/app/utils/icon.ts"

const corePath = "server/core/internal/document/icon/icons.json"
const core = JSON.parse(
	readFileSync(new URL(`../${corePath}`, import.meta.url), "utf8"),
)

const entry = (icon) => `${icon.id} (${icon.name})`
const web = selectableIcons.map(entry)
const coreEntries = core.map(entry)

if (web.join("\n") === coreEntries.join("\n")) {
	process.exit(0)
}

const onlyWeb = web.filter((e) => !coreEntries.includes(e))
const onlyCore = coreEntries.filter((e) => !web.includes(e))

console.error(`web/app/utils/icon.ts and ${corePath} differ.`)

for (const e of onlyWeb) {
	console.error(`  only in icon.ts:    ${e}`)
}

for (const e of onlyCore) {
	console.error(`  only in icons.json: ${e}`)
}

if (onlyWeb.length === 0 && onlyCore.length === 0) {
	console.error("  same entries in a different order")
}

process.exit(1)
