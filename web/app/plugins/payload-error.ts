// registers custom devalue reducers/revivers so that Error
// instances (TypeError, etc.) in the SSR payload can be
// serialized without the "Cannot stringify arbitrary non-POJOs"
// crash. This covers errors cached by Pinia Colada or any
// other state that ends up in the Nuxt payload.
export default definePayloadPlugin(() => {
	definePayloadReducer("ErrorObject", (value: unknown) => {
		if (value instanceof Error) {
			return {
				name: value.name,
				message: value.message,
				stack: import.meta.dev ? value.stack : undefined,
				cause:
					value.cause instanceof Error
						? { name: value.cause.name, message: value.cause.message }
						: undefined,
			}
		}
	})

	definePayloadReviver("ErrorObject", (data: Record<string, unknown>) => {
		const error = new Error(data.message as string)
		error.name = data.name as string

		if (data.stack) {
			error.stack = data.stack as string
		}

		if (data.cause) {
			const causeData = data.cause as { name?: string; message?: string }
			const cause = new Error(causeData.message)

			if (causeData.name) {
				cause.name = causeData.name
			}

			error.cause = cause
		}

		return error
	})
})
