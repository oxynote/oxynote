import { describe, it, vi } from "vitest"
import { AxiosHeaders } from "axios"
import { createCoreClient, type HttpResponse } from "./core.js"

const BASE_URL = "http://core:8080"

function stubHttp(response: HttpResponse = { status: 200, data: null }) {
	return {
		get: vi.fn().mockResolvedValue(response),
		post: vi.fn().mockResolvedValue(response),
		put: vi.fn().mockResolvedValue(response),
	}
}

describe("createCoreClient", () => {
	describe("sendEmail", () => {
		it("posts the template and its data to core's internal email endpoint", async ({
			expect,
		}) => {
			const http = stubHttp()

			await createCoreClient(BASE_URL, http).sendEmail(
				"password_reset",
				{ email: "a@b.c", link: "http://link" },
			)

			expect(http.post).toHaveBeenCalledTimes(1)
			expect(http.post).toHaveBeenCalledWith(
				"http://core:8080/api/x/email",
				{
					template: "password_reset",
					data: {
						email: "a@b.c",
						link: "http://link",
					},
				},
			)
			expect.soft(http.get).toHaveBeenCalledTimes(0)
			expect.soft(http.put).toHaveBeenCalledTimes(0)
		})

		it("propagates the transport's rejection untouched", async ({
			expect,
		}) => {
			const failure = new Error("connect ECONNREFUSED")
			const http = stubHttp()
			http.post.mockRejectedValue(failure)

			await expect(
				createCoreClient(BASE_URL, http).sendEmail(
					"password_reset",
					{ email: "a@b.c" },
				),
			).rejects.toBe(failure)
		})
	})

	describe("initializeOrganization", () => {
		it("posts to the organization's initialize endpoint", async ({
			expect,
		}) => {
			const http = stubHttp()

			await createCoreClient(
				BASE_URL,
				http,
			).initializeOrganization("org-1")

			expect(http.post).toHaveBeenCalledTimes(1)
			expect(http.post).toHaveBeenCalledWith(
				"http://core:8080/api/x/organizations/org-1/initialize",
			)
		})
	})

	describe("teardownOrganization", () => {
		it("posts to the organization's teardown endpoint", async ({
			expect,
		}) => {
			const http = stubHttp()

			await createCoreClient(
				BASE_URL,
				http,
			).teardownOrganization("org-1")

			expect(http.post).toHaveBeenCalledTimes(1)
			expect(http.post).toHaveBeenCalledWith(
				"http://core:8080/api/x/organizations/org-1/teardown",
			)
		})

		// the organization survives a failed teardown rather than
		// leaving core's resources orphaned, which only works if the
		// rejection reaches better-auth's hook
		it("propagates a failed teardown", async ({ expect }) => {
			const failure = new Error("teardown failed")
			const http = stubHttp()
			http.post.mockRejectedValue(failure)

			await expect(
				createCoreClient(
					BASE_URL,
					http,
				).teardownOrganization("org-1"),
			).rejects.toBe(failure)
		})
	})

	describe("fetchBranches", () => {
		it("returns the branch list core responded with", async ({
			expect,
		}) => {
			const branches = [
				{ branchId: "branch-1", default: true },
				{ branchId: "branch-2", default: false },
			]
			const http = stubHttp({ status: 200, data: branches })

			const result = await createCoreClient(
				BASE_URL,
				http,
			).fetchBranches("doc-1")

			expect(result).toEqual(branches)
			expect(http.get).toHaveBeenCalledTimes(1)
			expect(http.get).toHaveBeenCalledWith(
				"http://core:8080/api/x/documents/doc-1/branches",
			)
		})
	})

	describe("fetchBranchContent", () => {
		it("returns the branch core responded with", async ({
			expect,
		}) => {
			const branch = {
				documentName: "Runbook",
				content: { type: "doc", content: [] },
				icon: "lucide:file",
				rawContent: "AAEC",
			}
			const http = stubHttp({ status: 200, data: branch })

			const result = await createCoreClient(
				BASE_URL,
				http,
			).fetchBranchContent("doc-1", "branch-1")

			expect(result).toEqual(branch)
			expect(http.get).toHaveBeenCalledTimes(1)
			expect(http.get).toHaveBeenCalledWith(
				"http://core:8080/api/x/documents/doc-1/branch/branch-1",
			)
		})
	})

	describe("storeBranchContent", () => {
		it("puts the update to the branch's internal endpoint", async ({
			expect,
		}) => {
			const http = stubHttp()
			const update = {
				name: "Runbook",
				icon: "lucide:file",
				content: { type: "doc", content: [] },
				maintainers: ["user-1"],
				rawContent: "AAEC",
				system: false,
			}

			await createCoreClient(
				BASE_URL,
				http,
			).storeBranchContent("doc-1", "branch-1", update)

			expect(http.put).toHaveBeenCalledTimes(1)
			expect(http.put).toHaveBeenCalledWith(
				"http://core:8080/api/x/documents/doc-1/branch/branch-1",
				update,
			)
			expect.soft(http.get).toHaveBeenCalledTimes(0)
			expect.soft(http.post).toHaveBeenCalledTimes(0)
		})
	})

	describe("verifyDocumentAccess", () => {
		// the check goes through core's session-authed surface rather
		// than /api/x, because core decides from the forwarded headers
		// whether the caller may see the document
		it("gets core's public access route with the caller's headers", async ({
			expect,
		}) => {
			const http = stubHttp({ status: 204, data: null })
			const headers = new AxiosHeaders()
			headers.set("cookie", "auth.session=abc")

			await createCoreClient(
				BASE_URL,
				http,
			).verifyDocumentAccess("doc-1", { headers })

			expect(http.get).toHaveBeenCalledTimes(1)
			expect(http.get).toHaveBeenCalledWith(
				"http://core:8080/api/documents/doc-1/access",
				{ headers },
			)
			expect.soft(http.post).toHaveBeenCalledTimes(0)
			expect.soft(http.put).toHaveBeenCalledTimes(0)
		})

		// the connection is refused with core's own verdict, which only
		// works if the rejection reaches the onAuthenticate hook
		it("propagates a denied access untouched", async ({
			expect,
		}) => {
			const failure = new Error(
				"Request failed with status code 403",
			)
			const http = stubHttp()
			http.get.mockRejectedValue(failure)

			await expect(
				createCoreClient(
					BASE_URL,
					http,
				).verifyDocumentAccess("doc-1", {}),
			).rejects.toBe(failure)
		})
	})

	describe("mergeBranches", () => {
		// the merge is the one call that goes to core's session-authed
		// surface rather than /api/x, because core decides from the
		// forwarded headers whether the caller may merge
		it("puts to core's public merge route with the caller's headers", async ({
			expect,
		}) => {
			const http = stubHttp({
				status: 200,
				data: {
					documentName: "Runbook",
					content: { type: "doc", content: [] },
					icon: "lucide:file",
				},
			})
			const headers = new AxiosHeaders()
			headers.set("cookie", "auth.session=abc")

			const result = await createCoreClient(
				BASE_URL,
				http,
			).mergeBranches("doc-1", "branch-2", "branch-1", {
				headers,
			})

			expect(http.put).toHaveBeenCalledTimes(1)
			expect(http.put).toHaveBeenCalledWith(
				"http://core:8080/api/documents/doc-1/merge",
				{
					fromBranchId: "branch-2",
					toBranchId: "branch-1",
				},
				{ headers },
			)
			expect(result.status).toBe(200)
			expect(result.data.documentName).toBe("Runbook")
		})

		it("reports the status core answered with", async ({
			expect,
		}) => {
			const http = stubHttp({ status: 409, data: {} })

			const result = await createCoreClient(
				BASE_URL,
				http,
			).mergeBranches("doc-1", "branch-2", "branch-1", {})

			expect(result.status).toBe(409)
		})
	})
})
