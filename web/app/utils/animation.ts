import type { TransitionProps } from "vue"

export const defaultTransitionProps: TransitionProps = {
	enterFromClass: "opacity-0",
	enterActiveClass: "transition-opacity duration-300",
	enterToClass: "opacity-100",

	leaveFromClass: "opacity-100",
	leaveActiveClass: "transition-opacity duration-250",
	leaveToClass: "opacity-0",
}
