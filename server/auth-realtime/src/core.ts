import axios from "axios"
import type { AxiosHeaders } from "axios"

// the six emails core renders on this service's behalf. Naming them here
// keeps a typo in a template name a compile error rather than a silently
// dropped email.
export type EmailTemplate =
	| "email_verification"
	| "password_reset"
	| "account_exists"
	| "signup_verification"
	| "organization_invitation"
	| "user_deletion"

export interface BranchSummary {
	branchId: string
	default: boolean
	// whether the branch refuses every write but core's own.
	protected: boolean
}

export interface BranchContent {
	documentName: string
	// prosemirror JSON. Left unknown because nothing on this side
	// validates it — the editor schema is the only thing that can.
	content: unknown
	icon: string
	// the canonical Yjs state, base64-encoded. Absent on a branch that
	// has never been opened by an editor.
	rawContent?: string | null
}

export interface BranchUpdate {
	name: string
	icon: string
	content: unknown
	maintainers: string[]
	rawContent: string
	system: boolean
}

export interface MergedDocument {
	documentName: string
	content: unknown
	icon: string
}

// the branch requests this service forwards for the web client. Their
// bodies are core's to validate, so they travel through untyped.
interface BranchRequest {
	body: unknown
}

export interface RequestOptions {
	headers?: AxiosHeaders
}

export interface HttpResponse {
	status: number
	data: unknown
}

// the minimum of axios this service uses. Declaring it structurally lets a
// test hand createCoreClient a plain object instead of intercepting a
// real axios instance. Response bodies arrive as unknown: this client is
// the one place that states what core returns, and it does so by naming
// the shape at each call rather than trusting a generic parameter.
export interface HttpClient {
	get(url: string, config?: RequestOptions): Promise<HttpResponse>
	post(
		url: string,
		data?: unknown,
		config?: RequestOptions,
	): Promise<HttpResponse>
	put(
		url: string,
		data?: unknown,
		config?: RequestOptions,
	): Promise<HttpResponse>
	delete(url: string, config?: RequestOptions): Promise<HttpResponse>
}

// every call this service makes into core. Transport only: a failed
// request rejects with the underlying error untouched, because the callers
// differ in what they do about it — the merge route forwards core's status
// to its own caller, while the hocuspocus store hook swallows it and warns
// the connected editors instead.
export interface CoreClient {
	sendEmail(
		template: EmailTemplate,
		data: Record<string, string>,
	): Promise<void>
	initializeOrganization(organizationId: string): Promise<void>
	teardownOrganization(organizationId: string): Promise<void>
	fetchBranches(documentId: string): Promise<BranchSummary[]>
	fetchBranchContent(
		documentId: string,
		branchId: string,
	): Promise<BranchContent>
	storeBranchContent(
		documentId: string,
		branchId: string,
		update: BranchUpdate,
	): Promise<void>
	verifyDocumentAccess(
		documentId: string,
		options: RequestOptions,
	): Promise<void>
	mergeBranches(
		documentId: string,
		fromBranchId: string,
		toBranchId: string,
		options: RequestOptions,
	): Promise<{ status: number; data: MergedDocument }>
	createBranch(
		documentId: string,
		request: BranchRequest,
		options: RequestOptions,
	): Promise<HttpResponse>
	updateBranch(
		documentId: string,
		branchId: string,
		request: BranchRequest,
		options: RequestOptions,
	): Promise<HttpResponse>
	deleteBranch(
		documentId: string,
		branchId: string,
		options: RequestOptions,
	): Promise<HttpResponse>
}

export function createCoreClient(
	baseUrl: string,
	http: HttpClient = axios,
): CoreClient {
	// /api/x is core's internal, sessionless surface — the reverse proxy
	// blocks it at the front door, so these calls only work from inside
	// the container network.
	const internal = `${baseUrl}/api/x`

	return {
		async sendEmail(template, data) {
			await http.post(`${internal}/email`, { template, data })
		},

		async initializeOrganization(organizationId) {
			await http.post(
				`${internal}/organizations/${organizationId}/initialize`,
			)
		},

		async teardownOrganization(organizationId) {
			await http.post(
				`${internal}/organizations/${organizationId}/teardown`,
			)
		},

		async fetchBranches(documentId) {
			const response = await http.get(
				`${internal}/documents/${documentId}/branches`,
			)

			return response.data as BranchSummary[]
		},

		async fetchBranchContent(documentId, branchId) {
			const response = await http.get(
				`${internal}/documents/${documentId}/branch/${branchId}`,
			)

			return response.data as BranchContent
		},

		async storeBranchContent(documentId, branchId, update) {
			await http.put(
				`${internal}/documents/${documentId}/branch/${branchId}`,
				update,
			)
		},

		// the access check goes through core's session-authed surface,
		// not /api/x: the caller's own headers are forwarded so core
		// decides whether this user may see the document, and rejects
		// with core's status when they may not.
		async verifyDocumentAccess(documentId, options) {
			await http.get(
				`${baseUrl}/api/documents/${documentId}/access`,
				options,
			)
		},

		// the branch operations live on /api/x so that only this service
		// can reach them, but core still authorizes them from the
		// caller's own headers, which is why those are forwarded.
		async mergeBranches(
			documentId,
			fromBranchId,
			toBranchId,
			options,
		) {
			const response = await http.put(
				`${internal}/documents/${documentId}/merge`,
				{ fromBranchId, toBranchId },
				options,
			)

			return {
				status: response.status,
				data: response.data as MergedDocument,
			}
		},

		async createBranch(documentId, request, options) {
			return http.post(
				`${internal}/documents/${documentId}/branches`,
				request.body,
				options,
			)
		},

		async updateBranch(documentId, branchId, request, options) {
			return http.put(
				`${internal}/documents/${documentId}/branches/${branchId}`,
				request.body,
				options,
			)
		},

		async deleteBranch(documentId, branchId, options) {
			return http.delete(
				`${internal}/documents/${documentId}/branches/${branchId}`,
				options,
			)
		},
	}
}
