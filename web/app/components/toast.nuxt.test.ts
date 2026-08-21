import { beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ToastMessage from "./ToastMessage.vue"
import { showToastMessage } from "./toast"

vi.mock("vue-sonner", () => ({
	toast: {
		custom: vi.fn(),
		dismiss: vi.fn(),
	},
}))

// renders the toast body the way vue-sonner does: showToastMessage hands
// it a render function, so the vnode only exists once that is called
function renderToastBody() {
	const render = vi.mocked(toast.custom).mock.calls[0]?.[0] as
		| (() => {
				type: unknown
				props: Record<string, unknown> & { onClose: () => void }
		  })
		| undefined
	if (typeof render !== "function") {
		throw new Error("toast.custom was not called with a render function")
	}

	return render()
}

// vue-sonner is a module mock, so its call counts are shared by the whole
// file and cannot be isolated across interleaving tests
describe("showToastMessage", { concurrent: false }, () => {
	beforeEach(() => {
		vi.mocked(toast.custom).mockReset()
		vi.mocked(toast.dismiss).mockReset()
	})

	it("pushes exactly one custom toast", ({ expect }) => {
		showToastMessage("success", "Saved")

		expect(toast.custom).toHaveBeenCalledTimes(1)
		expect(toast.dismiss).toHaveBeenCalledTimes(0)
	})

	it("renders a ToastMessage carrying the type, title and description", ({
		expect,
	}) => {
		showToastMessage("error", "Failed", "Try again later")

		const body = renderToastBody()

		expect(body.type).toBe(ToastMessage)
		expect(body.props).toMatchObject({
			type: "error",
			title: "Failed",
			description: "Try again later",
		})
	})

	it("leaves the description undefined when none is given", ({ expect }) => {
		showToastMessage("info", "Heads up")

		expect(renderToastBody().props.description).toBeUndefined()
	})

	it("dismisses its own toast when the rendered message asks to close", ({
		expect,
	}) => {
		vi.mocked(toast.custom).mockReturnValue("toast-1")
		showToastMessage("warning", "Careful")

		renderToastBody().props.onClose()

		expect(toast.dismiss).toHaveBeenCalledExactlyOnceWith("toast-1")
	})
})
