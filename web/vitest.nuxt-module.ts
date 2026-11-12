import { defineNuxtModule } from "nuxt/kit"

// the test app is a client build, and nuxt strips server-only composables
// from it. The payload reducer would then never register, so its tests
// could not reach it. Config can only add names to that list, so this
// module takes one out.
export default defineNuxtModule({
	meta: { name: "vitest-keep-payload-reducer" },
	setup(_options, nuxt) {
		const composables = nuxt.options.optimization.treeShake.composables.client

		composables["#app"] = composables["#app"]?.filter(
			(name) => name !== "definePayloadReducer",
		)
	},
})
