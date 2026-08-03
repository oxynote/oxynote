import type { ComputedRef, Ref } from "vue"
import { createContext } from "reka-ui"

export {
	SIDEBAR_COOKIE_MAX_AGE,
	SIDEBAR_COOKIE_NAME,
	SIDEBAR_DEFAULT_WIDTH,
	SIDEBAR_MAX_WIDTH,
	SIDEBAR_MIN_WIDTH,
	SIDEBAR_MOBILE_BREAKPOINT,
	SIDEBAR_WIDTH_MOBILE,
} from "./constants"

export const [useSidebar, provideSidebarContext] = createContext<{
	state: ComputedRef<"expanded" | "collapsed">
	open: Ref<boolean>
	setOpen: (value: boolean) => void
	isMobile: Ref<boolean>
	openMobile: Ref<boolean>
	setOpenMobile: (value: boolean) => void
	toggleSidebar: () => void
}>("Sidebar")

export const [useSidebarWidth, provideSidebarWidthContext] = createContext<{
	width: Ref<number>
	setWidth: (value: number) => void
}>("SidebarWidth")
