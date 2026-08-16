import { toast } from "vue-sonner"
import ToastMessage from "./ToastMessage.vue"

export function showToastMessage(
	type: "success" | "error" | "info" | "warning",
	title: string,
	description?: string,
) {
	const toastId = toast.custom(() =>
		h(markRaw(ToastMessage), {
			type: type,
			title: title,
			description: description,
			onClose: () => {
				toast.dismiss(toastId)
			},
		}),
	)
}
