import { provideSSRWidth } from "@vueuse/core"
import { SIDEBAR_MOBILE_BREAKPOINT } from "~/components/shadcn/ui/sidebar/constants"

// needed by shadcn-nuxt. must exceed the sidebar's mobile breakpoint so the
// desktop sidebar shell renders during SSR instead of the mobile sheet.
export default defineNuxtPlugin((nuxtApp) => {
	provideSSRWidth(SIDEBAR_MOBILE_BREAKPOINT + 1, nuxtApp.vueApp)
})
