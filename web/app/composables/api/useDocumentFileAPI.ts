export interface UploadedDocumentFile {
	name: string
	size: number
	contentType: string
}

// the address ends in "<block id>-<file name>" so it reads as the file it
// is; the server identifies the file by the fixed-length id alone
export function buildDocumentFileSrc(
	documentId: string,
	blockId: string,
	name: string,
): string {
	const { coreAPIBaseHttpURL } = useRuntimeConfig().public

	return `${coreAPIBaseHttpURL}/api/documents/${documentId}/files/${blockId}-${encodeURIComponent(name)}`
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()

	const uploadDocumentFile = useMutation({
		mutation: async ({
			documentId,
			id,
			loc,
			kind,
			file,
		}: {
			documentId: string
			loc: DocumentFileLocation
			kind: DocumentFileKind
			id: string
			file: File
		}): Promise<UploadedDocumentFile> => {
			const body = new FormData()
			body.append("file", file)

			const response = await $coreAPIClient.raw<DocumentFileUpload>(
				`/api/documents/${documentId}/files?id=${encodeURIComponent(id)}&location=${loc}&kind=${kind}`,
				{
					method: "POST",
					body,
				},
			)
			const uploaded = response._data

			if (!uploaded) {
				throw new Error("missing upload response body")
			}

			return {
				name: uploaded.name,
				size: uploaded.size,
				contentType: uploaded.contentType,
			}
		},
	})

	return {
		uploadDocumentFile,
	}
}
