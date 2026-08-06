export function buildDocumentFileSrc(
	documentId: string,
	blockId: string,
): string {
	const { coreAPIBaseHttpURL } = useRuntimeConfig().public
	return `${coreAPIBaseHttpURL}/api/documents/${documentId}/files/${blockId}`
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()

	const uploadDocumentFile = useMutation({
		mutation: async ({
			documentId,
			id,
			loc,
			file,
		}: {
			documentId: string
			loc: DocumentFileLocation
			id: string
			file: File
		}) => {
			const body = new FormData()
			body.append("file", file)

			const response = await $coreAPIClient.raw(
				`/api/documents/${documentId}/files?id=${encodeURIComponent(id)}&location=${loc}`,
				{
					method: "POST",
					body,
				},
			)
			const location = response.headers.get("location")

			if (!location) {
				throw new Error("missing location header")
			}

			return location
		},
	})

	return {
		uploadDocumentFile,
	}
}
