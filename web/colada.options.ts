import type { PiniaColadaOptions } from "@pinia/colada"
import { PiniaColadaAutoRefetch } from "@pinia/colada-plugin-auto-refetch"

export default {
	plugins: [PiniaColadaAutoRefetch({ autoRefetch: false })],
} satisfies PiniaColadaOptions
