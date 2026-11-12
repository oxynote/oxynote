import { afterEach, beforeEach, describe, it, vi } from "vitest"
import { toast } from "vue-sonner"
import ToastMessage from "./ToastMessage.vue"
import ToastProgress from "./ToastProgress.vue"
import { showProgressToast, showToastMessage } from "./toast"

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

	it("passes an optional action to the rendered message", ({ expect }) => {
		const action = { label: "Reload", onClick: vi.fn() }

		showToastMessage("warning", "Read-only", undefined, action)

		expect(renderToastBody().props.action).toBe(action)
		expect(action.onClick).not.toHaveBeenCalled()
	})

	it("keeps an actionable toast visible until it is dismissed", ({
		expect,
	}) => {
		showToastMessage("warning", "Read-only", undefined, {
			label: "Reload",
			onClick: vi.fn(),
		})

		expect(toast.custom).toHaveBeenCalledExactlyOnceWith(expect.any(Function), {
			duration: Number.POSITIVE_INFINITY,
		})
		expect(toast.dismiss).not.toHaveBeenCalled()
	})

	it("keeps the toaster's default duration for a message without an action", ({
		expect,
	}) => {
		showToastMessage("success", "Saved")

		expect(toast.custom).toHaveBeenCalledExactlyOnceWith(
			expect.any(Function),
			undefined,
		)
		expect(toast.dismiss).not.toHaveBeenCalled()
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

// vue-sonner is a module mock and settling is timed, so the calls and the
// fake clock are shared by the whole describe
describe("showProgressToast", { concurrent: false }, () => {
	beforeEach(() => {
		vi.mocked(toast.custom).mockReset()
		vi.mocked(toast.dismiss).mockReset()
		vi.mocked(toast.custom).mockReturnValue("toast-1")
		vi.useFakeTimers()
	})

	afterEach(() => {
		vi.useRealTimers()
	})

	it("pushes one toast that stays until the work settles", ({ expect }) => {
		showProgressToast("Downloading notes.zip")

		expect(toast.custom).toHaveBeenCalledExactlyOnceWith(expect.any(Function), {
			duration: Number.POSITIVE_INFINITY,
		})
		expect(toast.dismiss).toHaveBeenCalledTimes(0)
	})

	it("renders a ToastProgress starting at no progress", ({ expect }) => {
		showProgressToast("Downloading notes.zip")

		const body = renderProgressBody()

		expect(body.type).toBe(ToastProgress)
		expect(body.props).toMatchObject({
			status: "progress",
			title: "Downloading notes.zip",
			progress: 0,
		})
	})

	it.for([
		{ name: "renders the latest progress", input: 0.42 },
		{ name: "renders an unknown total", input: null },
	])("$name", ({ input }, { expect }) => {
		const handle = showProgressToast("Downloading notes.zip")

		handle.update(input)

		expect(renderProgressBody().props.progress).toBe(input)
		expect(toast.custom).toHaveBeenCalledTimes(1)
	})

	it.for([
		{ method: "succeed", expected: "success" },
		{ method: "fail", expected: "error" },
	] as const)(
		"shows the outcome in place when told to $method",
		({ method, expected }, { expect }) => {
			const handle = showProgressToast("Downloading notes.zip")

			handle[method]("Done")

			expect(renderProgressBody().props).toMatchObject({
				status: expected,
				title: "Done",
			})
			expect(toast.custom).toHaveBeenCalledTimes(1)
			expect(toast.dismiss).toHaveBeenCalledTimes(0)
		},
	)

	it("leaves on its own once settled", ({ expect }) => {
		const handle = showProgressToast("Downloading notes.zip")
		handle.succeed("Downloaded notes.zip")

		vi.advanceTimersByTime(4999)
		expect(toast.dismiss).toHaveBeenCalledTimes(0)

		vi.advanceTimersByTime(1)

		expect(toast.dismiss).toHaveBeenCalledExactlyOnceWith("toast-1")
	})

	it("stays while the work is still running", ({ expect }) => {
		const handle = showProgressToast("Downloading notes.zip")
		handle.update(0.5)

		vi.advanceTimersByTime(60_000)

		expect(toast.dismiss).toHaveBeenCalledTimes(0)
	})

	it("dismisses its toast when asked", ({ expect }) => {
		const handle = showProgressToast("Downloading notes.zip")

		handle.dismiss()

		expect(toast.dismiss).toHaveBeenCalledExactlyOnceWith("toast-1")
	})

	it("dismisses its toast when the rendered toast asks to close", ({
		expect,
	}) => {
		showProgressToast("Downloading notes.zip")

		renderProgressBody().props.onClose()

		expect(toast.dismiss).toHaveBeenCalledExactlyOnceWith("toast-1")
	})
})

// renders the progress toast's body as vue-sonner would on its next
// render: the render function reads the toast's current state
function renderProgressBody() {
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
