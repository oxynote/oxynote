import { toast } from "vue-sonner"
import ToastMessage from "./ToastMessage.vue"
import ToastProgress from "./ToastProgress.vue"

export function showToastMessage(
	type: "success" | "error" | "info" | "warning",
	title: string,
	description?: string,
	action?: { label: string; onClick: () => void },
) {
	const toastId = toast.custom(() =>
		h(markRaw(ToastMessage), {
			type: type,
			title: title,
			description: description,
			action: action,
			onClose: () => {
				toast.dismiss(toastId)
			},
		}),
	)
}

export interface ProgressToast {
	// progress is a fraction from 0 to 1, or null while the total is unknown
	update(progress: number | null): void
	succeed(title: string): void
	fail(title: string): void
	dismiss(): void
}

// how long a settled progress toast stays, matching the toaster's default
const SETTLED_TOAST_DURATION_MS = 5000

// showProgressToast opens a toast that stays until the work it tracks
// settles, then shows the outcome in place and leaves on its own. The
// render function reads reactive state, so updates re-render the toast
// without pushing a new one.
export function showProgressToast(title: string): ProgressToast {
	const state = reactive<{
		status: "progress" | "success" | "error"
		title: string
		progress: number | null
	}>({
		status: "progress",
		title: title,
		progress: 0,
	})

	const toastId = toast.custom(
		() =>
			h(markRaw(ToastProgress), {
				status: state.status,
				title: state.title,
				progress: state.progress,
				onClose: dismiss,
			}),
		{ duration: Number.POSITIVE_INFINITY },
	)

	function dismiss() {
		toast.dismiss(toastId)
	}

	function settle(status: "success" | "error", settledTitle: string) {
		state.status = status
		state.title = settledTitle
		setTimeout(dismiss, SETTLED_TOAST_DURATION_MS)
	}

	return {
		update: (progress) => {
			state.progress = progress
		},
		succeed: (settledTitle) => {
			settle("success", settledTitle)
		},
		fail: (settledTitle) => {
			settle("error", settledTitle)
		},
		dismiss: dismiss,
	}
}
