export default function () {
	const { $coreAPIClient } = useNuxtApp()

	const uploadUserImage = useMutation({
		// no optimistic updates possible here
		mutation: async (file: File) => {
			const body = new FormData()
			body.append("image", file)

			const response = await $coreAPIClient.raw(`/api/users/image`, {
				method: "PUT",
				body,
			})
			const location = response.headers.get("location")

			if (!location) {
				throw new Error("missing location header")
			}

			return location
		},
	})

	return { uploadUserImage }
}
