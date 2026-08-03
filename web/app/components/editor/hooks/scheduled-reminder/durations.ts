export const presetDurations: string[] = [
	...(import.meta.dev ? ["0", "1m"] : []),
	"24h",
	"72h",
	"168h",
	"336h",
	"720h",
	"2160h",
	"4320h",
	"custom",
]
