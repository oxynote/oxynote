import { flushPromises } from "@vue/test-utils"
import { afterEach, beforeEach, describe, it } from "vitest"
import type {
	VariableGroupTreeElement,
	VariableRequest,
	VariableTreeElement,
	VariableTreeResponse,
} from "~/utils"
import {
	ANY_STRING,
	clearQueryCache,
	disposeMockEndpoints,
	makeXid,
	mockDeferredEndpoint,
	mockEndpoint,
	readQueryData,
	runInApp,
	seedAuthOrganization,
	seedAuthSession,
	seedQueryData,
} from "./test-helpers"
import useVariableAPI from "./useVariableAPI"

const TREE_KEY = ["variables", "tree"] as const
const VARIABLES_URL = "/api/variables"
const TREE_URL = "/api/variables/tree"

const ORG_ID = "org-1"
const USER_ID = "user-1"
const GROUP_BUILTIN = makeXid("grpbuiltin")
const GROUP_A = makeXid("grpa")
const GROUP_B = makeXid("grpb")
const GROUP_MISSING = makeXid("grpmissing")
const VAR_LIST = makeXid("varlist")
const VAR_TEXT = makeXid("vartext")
const VAR_MISSING = makeXid("varmissing")
const OPTION_A = makeXid("opta")
const OPTION_B = makeXid("optb")
const OPTION_MISSING = makeXid("optmissing")
const SHORT_ID = "local-1"

const LIST_REQ: VariableRequest = {
	groupId: GROUP_A,
	key: "env",
	name: "Environment",
	description: null,
	type: "list",
	options: [{ id: OPTION_A, value: "production" }, { value: "staging" }],
}

const TEXT_REQ: VariableRequest = {
	groupId: GROUP_A,
	key: "region",
	name: "Primary region",
	description: "Where the stack runs",
	type: "text",
	defaultValue: "eu-central-1",
}

type VariableAPI = ReturnType<typeof useVariableAPI>

// every mutation writes the tree through one cache entry and invalidates it
// on success, so the tests share a cache and cannot interleave
describe("useVariableAPI", { concurrent: false }, () => {
	beforeEach(clearQueryCache)
	afterEach(disposeMockEndpoints)

	describe("fetchVariables", () => {
		it("fetches the groups with their variables", async ({ expect }) => {
			const calls = mockEndpoint("GET", TREE_URL, () => makeTree())
			const api = makeVariableAPI()

			await api.fetchVariables.refresh()

			expect.soft(calls).toHaveLength(1)
			expect.soft(api.fetchVariables.data.value).toEqual(makeTree())
		})

		it("exposes the error when the request fails", async ({ expect }) => {
			const calls = mockEndpoint("GET", TREE_URL, () => {
				throw createError({ statusCode: 400 })
			})
			const api = makeVariableAPI()

			await api.fetchVariables.refresh()

			expect.soft(calls).toHaveLength(1)
			expect.soft(api.fetchVariables.error.value).toBeInstanceOf(Error)
			expect.soft(api.fetchVariables.data.value).toBeUndefined()
		})
	})

	describe("createVariable", () => {
		it("shows the new variable at the end of its group and refreshes the tree", async ({
			expect,
		}) => {
			seedAuth()
			const treeCalls = mockTree()
			const post = mockSnapshotEndpoint(
				"POST",
				VARIABLES_URL,
				makeListVariable(),
			)
			const api = makeVariableAPI()

			const created = await api.createVariable.mutateAsync(LIST_REQ)
			await flushPromises()

			const optimistic = groupOf(post.snapshots[0], GROUP_A)
			expect(optimistic?.variables.map((v) => v.id)).toEqual([
				VAR_LIST,
				VAR_TEXT,
				ANY_STRING,
			])
			expect(optimistic?.variables[2]).toMatchObject({
				organizationId: ORG_ID,
				groupId: GROUP_A,
				key: "env",
				type: "list",
				defaultValue: OPTION_A,
				value: null,
				createdBy: USER_ID,
				localOptimisticInsert: true,
			})
			expect(
				optimistic?.variables[2]?.options.map((o) => [o.id, o.value]),
			).toEqual([
				[OPTION_A, "production"],
				[ANY_STRING, "staging"],
			])
			expect.soft(created).toEqual(makeListVariable())
			expect.soft(post.calls).toHaveLength(1)
			expect.soft(post.calls[0]?.body).toEqual(LIST_REQ)
			expect.soft(treeCalls).toHaveLength(1)
			expect.soft(readTree()).toEqual([])
		})

		it("sends the create without an optimistic insert when session data is missing", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const post = mockSnapshotEndpoint(
				"POST",
				VARIABLES_URL,
				makeListVariable(),
			)
			const api = makeVariableAPI()

			await api.createVariable.mutateAsync(LIST_REQ)
			await flushPromises()

			expect.soft(post.snapshots[0]).toEqual(makeTree())
			expect.soft(post.calls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("rejects without a request when the group is not in the tree", async ({
			expect,
		}) => {
			seedAuth()
			const treeCalls = mockTree()
			const postCalls = mockEndpoint("POST", VARIABLES_URL, () => ({}))
			const api = makeVariableAPI()

			await expect(
				api.createVariable.mutateAsync({ ...LIST_REQ, groupId: GROUP_MISSING }),
			).rejects.toThrow("invalid variable data")

			expect.soft(postCalls).toHaveLength(0)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})

		it("drops the new variable again when the request fails", async ({
			expect,
		}) => {
			seedAuth()
			const treeCalls = mockTree()
			const postCalls = mockFailingEndpoint("POST", VARIABLES_URL)
			const api = makeVariableAPI()

			await expect(api.createVariable.mutateAsync(LIST_REQ)).rejects.toThrow()
			await flushPromises()

			expect.soft(postCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	describe("moveVariable", () => {
		it.for([
			{
				name: "moves a variable into another group",
				input: { id: VAR_TEXT, groupId: GROUP_B, sortIndex: 0 },
				expected: { [GROUP_A]: [VAR_LIST], [GROUP_B]: [VAR_TEXT] },
			},
			{
				name: "moves a variable inside its own group",
				input: { id: VAR_TEXT, groupId: GROUP_A, sortIndex: 0 },
				expected: { [GROUP_A]: [VAR_TEXT, VAR_LIST], [GROUP_B]: [] },
			},
		])(
			"$name and refreshes the tree",
			async ({ input, expected }, { expect }) => {
				const treeCalls = mockTree()
				const put = mockSnapshotEndpoint("PUT", TREE_URL)
				const api = makeVariableAPI()

				await expect(
					api.moveVariable.mutateAsync(input),
				).resolves.toBeUndefined()
				await flushPromises()

				const snapshot = put.snapshots[0]
				expect({
					[GROUP_A]: variableIds(snapshot, GROUP_A),
					[GROUP_B]: variableIds(snapshot, GROUP_B),
				}).toEqual(expected)
				expect(findVariable(snapshot, VAR_TEXT)?.groupId).toBe(input.groupId)
				expect.soft(put.calls).toHaveLength(1)
				expect.soft(put.calls[0]?.body).toEqual(input)
				expect.soft(treeCalls).toHaveLength(1)
			},
		)

		it.for([
			{ name: "variable", input: { id: VAR_MISSING, groupId: GROUP_A } },
			{ name: "group", input: { id: VAR_TEXT, groupId: GROUP_MISSING } },
		])(
			"rejects without a request when the $name is not in the tree",
			async ({ input }, { expect }) => {
				const putCalls = mockEndpoint("PUT", TREE_URL, () => ({}))
				mockTree()
				const api = makeVariableAPI()

				await expect(
					api.moveVariable.mutateAsync({ ...input, sortIndex: 0 }),
				).rejects.toThrow("invalid variable move data")

				expect.soft(putCalls).toHaveLength(0)
				expect.soft(readTree()).toEqual(makeTree())
			},
		)

		it("puts the old order back when the move fails", async ({ expect }) => {
			const treeCalls = mockTree()
			const putCalls = mockFailingEndpoint("PUT", TREE_URL)
			const api = makeVariableAPI()

			await expect(
				api.moveVariable.mutateAsync({
					id: VAR_TEXT,
					groupId: GROUP_B,
					sortIndex: 0,
				}),
			).rejects.toThrow()
			await flushPromises()

			expect.soft(putCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	describe("updateVariable", () => {
		it("rewrites the variable in place and refreshes the tree", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const put = mockSnapshotEndpoint("PUT", `/api/variables/${VAR_TEXT}`)
			const api = makeVariableAPI()

			await expect(
				api.updateVariable.mutateAsync({ id: VAR_TEXT, req: TEXT_REQ }),
			).resolves.toBeUndefined()
			await flushPromises()

			const snapshot = put.snapshots[0]
			expect(variableIds(snapshot, GROUP_A)).toEqual([VAR_LIST, VAR_TEXT])
			expect(findVariable(snapshot, VAR_TEXT)).toEqual({
				...makeTextVariable(),
				key: "region",
				name: "Primary region",
				description: "Where the stack runs",
				defaultValue: "eu-central-1",
			})
			expect.soft(put.calls).toHaveLength(1)
			expect.soft(put.calls[0]?.body).toEqual(TEXT_REQ)
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("moves the variable to the end of another group", async ({ expect }) => {
			mockTree()
			const put = mockSnapshotEndpoint("PUT", `/api/variables/${VAR_LIST}`)
			const api = makeVariableAPI()

			await api.updateVariable.mutateAsync({
				id: VAR_LIST,
				req: { ...LIST_REQ, groupId: GROUP_B },
			})

			const snapshot = put.snapshots[0]
			expect.soft(variableIds(snapshot, GROUP_A)).toEqual([VAR_TEXT])
			expect.soft(variableIds(snapshot, GROUP_B)).toEqual([VAR_LIST])
			expect.soft(findVariable(snapshot, VAR_LIST)?.groupId).toBe(GROUP_B)
		})

		it.for([
			{
				name: "keeps the value of a text variable",
				input: { id: VAR_TEXT, req: TEXT_REQ },
				expected: { value: "us-east-1", defaultValue: "eu-central-1" },
			},
			{
				name: "drops the value when the type changes",
				input: { id: VAR_TEXT, req: { ...TEXT_REQ, type: "number" } },
				expected: { value: null, defaultValue: "eu-central-1" },
			},
			{
				name: "keeps a picked option that stays",
				input: {
					id: VAR_LIST,
					req: {
						...LIST_REQ,
						options: [
							{ id: OPTION_B, value: "staging" },
							{ id: OPTION_A, value: "production" },
						],
					},
				},
				expected: { value: OPTION_B, defaultValue: OPTION_B },
			},
			{
				name: "drops a picked option that is removed",
				input: {
					id: VAR_LIST,
					req: { ...LIST_REQ, options: [{ id: OPTION_A, value: "prod" }] },
				},
				expected: { value: null, defaultValue: OPTION_A },
			},
			{
				name: "leaves a list without options with no default",
				input: { id: VAR_LIST, req: { ...LIST_REQ, options: undefined } },
				expected: { value: null, defaultValue: null },
			},
		] satisfies {
			name: string
			input: { id: string; req: VariableRequest }
			expected: { value: string | null; defaultValue: string | null }
		}[])("$name", async ({ input, expected }, { expect }) => {
			mockTree()
			const put = mockSnapshotEndpoint("PUT", `/api/variables/${input.id}`)
			const api = makeVariableAPI()

			await api.updateVariable.mutateAsync(input)

			const variable = findVariable(put.snapshots[0], input.id)
			expect.soft(variable?.value).toBe(expected.value)
			expect.soft(variable?.defaultValue).toBe(expected.defaultValue)
		})

		it.for([
			{ name: "variable", input: { id: VAR_MISSING, groupId: GROUP_A } },
			{ name: "group", input: { id: VAR_TEXT, groupId: GROUP_MISSING } },
		])(
			"rejects without a request when the $name is not in the tree",
			async ({ input }, { expect }) => {
				const putCalls = mockEndpoint(
					"PUT",
					`/api/variables/${input.id}`,
					() => ({}),
				)
				mockTree()
				const api = makeVariableAPI()

				await expect(
					api.updateVariable.mutateAsync({
						id: input.id,
						req: { ...TEXT_REQ, groupId: input.groupId },
					}),
				).rejects.toThrow("invalid variable data")

				expect.soft(putCalls).toHaveLength(0)
				expect.soft(readTree()).toEqual(makeTree())
			},
		)

		it("puts the old variable back when the update fails", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const putCalls = mockFailingEndpoint("PUT", `/api/variables/${VAR_TEXT}`)
			const api = makeVariableAPI()

			await expect(
				api.updateVariable.mutateAsync({ id: VAR_TEXT, req: TEXT_REQ }),
			).rejects.toThrow()
			await flushPromises()

			expect.soft(putCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	describe("deleteVariable", () => {
		it("removes the variable and refreshes the tree", async ({ expect }) => {
			const treeCalls = mockTree()
			const del = mockSnapshotEndpoint("DELETE", `/api/variables/${VAR_TEXT}`)
			const api = makeVariableAPI()

			await expect(
				api.deleteVariable.mutateAsync(VAR_TEXT),
			).resolves.toBeUndefined()
			await flushPromises()

			expect.soft(variableIds(del.snapshots[0], GROUP_A)).toEqual([VAR_LIST])
			expect.soft(del.calls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("puts the variable back when the deletion fails", async ({ expect }) => {
			const treeCalls = mockTree()
			const delCalls = mockFailingEndpoint(
				"DELETE",
				`/api/variables/${VAR_TEXT}`,
			)
			const api = makeVariableAPI()

			await expect(api.deleteVariable.mutateAsync(VAR_TEXT)).rejects.toThrow()
			await flushPromises()

			expect.soft(delCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})

		it("keeps a tree written after the deletion when it fails", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const del = mockDeferredEndpoint("DELETE", `/api/variables/${VAR_TEXT}`)
			const api = makeVariableAPI()

			const pending = api.deleteVariable.mutateAsync(VAR_TEXT)
			await del.reached

			// the deletion shows, then newer data lands before it fails
			expect(variableIds(readTree(), GROUP_A)).toEqual([VAR_LIST])
			seedQueryData(TREE_KEY, [])
			del.reject(createError({ statusCode: 500 }))

			await expect(pending).rejects.toThrow()
			await flushPromises()

			expect.soft(del.calls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual([])
		})
	})

	describe("setVariableDefault", () => {
		it("moves the chosen option of a list to the top and refreshes the tree", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const put = mockSnapshotEndpoint(
				"PUT",
				`/api/variables/${VAR_LIST}/default`,
			)
			const api = makeVariableAPI()

			await expect(
				api.setVariableDefault.mutateAsync({
					id: VAR_LIST,
					req: { value: OPTION_B },
				}),
			).resolves.toBeUndefined()
			await flushPromises()

			const variable = findVariable(put.snapshots[0], VAR_LIST)
			expect(variable?.defaultValue).toBe(OPTION_B)
			expect(variable?.options.map((o) => [o.id, o.sortIndex])).toEqual([
				[OPTION_B, 0],
				[OPTION_A, 1],
			])
			expect.soft(put.calls).toHaveLength(1)
			expect.soft(put.calls[0]?.body).toEqual({ value: OPTION_B })
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("sets the default of a text variable", async ({ expect }) => {
			mockTree()
			const put = mockSnapshotEndpoint(
				"PUT",
				`/api/variables/${VAR_TEXT}/default`,
			)
			const api = makeVariableAPI()

			await api.setVariableDefault.mutateAsync({
				id: VAR_TEXT,
				req: { value: "ap-south-1" },
			})

			expect.soft(findVariable(put.snapshots[0], VAR_TEXT)).toEqual({
				...makeTextVariable(),
				defaultValue: "ap-south-1",
			})
		})

		it.for([
			{ name: "variable", input: { id: VAR_MISSING, value: OPTION_B } },
			{ name: "option", input: { id: VAR_LIST, value: OPTION_MISSING } },
		])(
			"rejects without a request when the $name is not in the tree",
			async ({ input }, { expect }) => {
				const putCalls = mockEndpoint(
					"PUT",
					`/api/variables/${input.id}/default`,
					() => ({}),
				)
				mockTree()
				const api = makeVariableAPI()

				await expect(
					api.setVariableDefault.mutateAsync({
						id: input.id,
						req: { value: input.value },
					}),
				).rejects.toThrow("invalid variable default data")

				expect.soft(putCalls).toHaveLength(0)
				expect.soft(readTree()).toEqual(makeTree())
			},
		)

		it("puts the old default back when the request fails", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const putCalls = mockFailingEndpoint(
				"PUT",
				`/api/variables/${VAR_LIST}/default`,
			)
			const api = makeVariableAPI()

			await expect(
				api.setVariableDefault.mutateAsync({
					id: VAR_LIST,
					req: { value: OPTION_B },
				}),
			).rejects.toThrow()
			await flushPromises()

			expect.soft(putCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	describe("setVariableValue", () => {
		it("shows the caller's value and refreshes the tree", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const put = mockSnapshotEndpoint(
				"PUT",
				`/api/variables/${VAR_LIST}/value`,
			)
			const api = makeVariableAPI()

			await expect(
				api.setVariableValue.mutateAsync({
					id: VAR_LIST,
					req: { value: OPTION_A },
				}),
			).resolves.toBeUndefined()
			await flushPromises()

			expect(findVariable(put.snapshots[0], VAR_LIST)?.value).toBe(OPTION_A)
			expect.soft(put.calls).toHaveLength(1)
			expect.soft(put.calls[0]?.body).toEqual({ value: OPTION_A })
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("refreshes the tree only after the last value save in flight", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const listPut = mockDeferredEndpoint(
				"PUT",
				`/api/variables/${VAR_LIST}/value`,
			)
			const textPut = mockDeferredEndpoint(
				"PUT",
				`/api/variables/${VAR_TEXT}/value`,
			)
			const listSave = makeVariableAPI().setVariableValue.mutateAsync({
				id: VAR_LIST,
				req: { value: OPTION_A },
			})
			const textSave = makeVariableAPI().setVariableValue.mutateAsync({
				id: VAR_TEXT,
				req: { value: "us-east-1" },
			})
			await Promise.all([listPut.reached, textPut.reached])

			listPut.resolve(null)
			await listSave
			await flushPromises()

			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(findVariable(readTree(), VAR_TEXT)?.value).toBe("us-east-1")

			textPut.resolve(null)
			await textSave
			await flushPromises()

			expect.soft(treeCalls).toHaveLength(1)
		})

		it("rejects without a request when the variable is not in the tree", async ({
			expect,
		}) => {
			const putCalls = mockEndpoint(
				"PUT",
				`/api/variables/${VAR_MISSING}/value`,
				() => ({}),
			)
			mockTree()
			const api = makeVariableAPI()

			await expect(
				api.setVariableValue.mutateAsync({
					id: VAR_MISSING,
					req: { value: "x" },
				}),
			).rejects.toThrow("invalid variable value data")

			expect.soft(putCalls).toHaveLength(0)
		})

		it("puts the old value back when the request fails", async ({ expect }) => {
			const treeCalls = mockTree()
			const putCalls = mockFailingEndpoint(
				"PUT",
				`/api/variables/${VAR_LIST}/value`,
			)
			const api = makeVariableAPI()

			await expect(
				api.setVariableValue.mutateAsync({
					id: VAR_LIST,
					req: { value: OPTION_A },
				}),
			).rejects.toThrow()
			await flushPromises()

			expect.soft(putCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	describe("resetVariableValue", () => {
		it("clears the caller's value and refreshes the tree", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const del = mockSnapshotEndpoint(
				"DELETE",
				`/api/variables/${VAR_LIST}/value`,
			)
			const api = makeVariableAPI()

			await expect(
				api.resetVariableValue.mutateAsync(VAR_LIST),
			).resolves.toBeUndefined()
			await flushPromises()

			expect(findVariable(del.snapshots[0], VAR_LIST)?.value).toBeNull()
			expect.soft(del.calls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("rejects without a request when the variable is not in the tree", async ({
			expect,
		}) => {
			const delCalls = mockEndpoint(
				"DELETE",
				`/api/variables/${VAR_MISSING}/value`,
				() => ({}),
			)
			mockTree()
			const api = makeVariableAPI()

			await expect(
				api.resetVariableValue.mutateAsync(VAR_MISSING),
			).rejects.toThrow("invalid variable value data")

			expect.soft(delCalls).toHaveLength(0)
		})

		it("puts the old value back when the request fails", async ({ expect }) => {
			const treeCalls = mockTree()
			const delCalls = mockFailingEndpoint(
				"DELETE",
				`/api/variables/${VAR_LIST}/value`,
			)
			const api = makeVariableAPI()

			await expect(
				api.resetVariableValue.mutateAsync(VAR_LIST),
			).rejects.toThrow()
			await flushPromises()

			expect.soft(delCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	describe("createVariableGroup", () => {
		it("shows the new group at the end and refreshes the tree", async ({
			expect,
		}) => {
			seedAuth()
			const treeCalls = mockTree()
			const post = mockSnapshotEndpoint(
				"POST",
				"/api/variables/groups",
				makeGroup(GROUP_B, "Teams"),
			)
			const api = makeVariableAPI()

			const created = await api.createVariableGroup.mutateAsync({
				groupName: "Services",
				color: "orange",
			})
			await flushPromises()

			const snapshot = post.snapshots[0]
			expect(snapshot?.map((g) => g.id)).toEqual([
				GROUP_BUILTIN,
				GROUP_A,
				GROUP_B,
				ANY_STRING,
			])
			expect(snapshot?.[3]).toMatchObject({
				organizationId: ORG_ID,
				groupName: "Services",
				color: "orange",
				sortIndex: 3,
				builtin: false,
				createdBy: USER_ID,
				variables: [],
				localOptimisticInsert: true,
			})
			expect.soft(created).toEqual(makeGroup(GROUP_B, "Teams"))
			expect.soft(post.calls).toHaveLength(1)
			expect
				.soft(post.calls[0]?.body)
				.toEqual({ groupName: "Services", color: "orange" })
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("sends the create without an optimistic insert when session data is missing", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const post = mockSnapshotEndpoint("POST", "/api/variables/groups")
			const api = makeVariableAPI()

			await api.createVariableGroup.mutateAsync({
				groupName: "Services",
				color: "orange",
			})
			await flushPromises()

			expect.soft(post.snapshots[0]).toEqual(makeTree())
			expect.soft(post.calls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("drops the new group again when the request fails", async ({
			expect,
		}) => {
			seedAuth()
			const treeCalls = mockTree()
			const postCalls = mockFailingEndpoint("POST", "/api/variables/groups")
			const api = makeVariableAPI()

			await expect(
				api.createVariableGroup.mutateAsync({
					groupName: "Services",
					color: "orange",
				}),
			).rejects.toThrow()
			await flushPromises()

			expect.soft(postCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	describe("moveVariableGroup", () => {
		it("moves a group, counting only the groups after the built-in one", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const put = mockSnapshotEndpoint("PUT", "/api/variables/groups/tree")
			const api = makeVariableAPI()

			await expect(
				api.moveVariableGroup.mutateAsync({ id: GROUP_B, sortIndex: 0 }),
			).resolves.toBeUndefined()
			await flushPromises()

			expect(put.snapshots[0]?.map((g) => g.id)).toEqual([
				GROUP_BUILTIN,
				GROUP_B,
				GROUP_A,
			])
			expect.soft(put.calls).toHaveLength(1)
			expect.soft(put.calls[0]?.body).toEqual({ id: GROUP_B, sortIndex: 0 })
			expect.soft(treeCalls).toHaveLength(1)
		})

		it.for([
			{ name: "a group missing from the tree", input: GROUP_MISSING },
			{ name: "the built-in group", input: GROUP_BUILTIN },
		])("rejects without a request for $name", async ({ input }, { expect }) => {
			const putCalls = mockEndpoint(
				"PUT",
				"/api/variables/groups/tree",
				() => ({}),
			)
			mockTree()
			const api = makeVariableAPI()

			await expect(
				api.moveVariableGroup.mutateAsync({ id: input, sortIndex: 1 }),
			).rejects.toThrow("invalid variable group move data")

			expect.soft(putCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})

		it("puts the old order back when the move fails", async ({ expect }) => {
			const treeCalls = mockTree()
			const putCalls = mockFailingEndpoint("PUT", "/api/variables/groups/tree")
			const api = makeVariableAPI()

			await expect(
				api.moveVariableGroup.mutateAsync({ id: GROUP_B, sortIndex: 0 }),
			).rejects.toThrow()
			await flushPromises()

			expect.soft(putCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	describe("updateVariableGroup", () => {
		it("applies only the fields sent and refreshes the tree", async ({
			expect,
		}) => {
			const treeCalls = mockTree()
			const put = mockSnapshotEndpoint(
				"PUT",
				`/api/variables/groups/${GROUP_A}`,
			)
			const api = makeVariableAPI()

			await expect(
				api.updateVariableGroup.mutateAsync({
					id: GROUP_A,
					req: { color: "orange" },
				}),
			).resolves.toBeUndefined()
			await flushPromises()

			const group = groupOf(put.snapshots[0], GROUP_A)
			expect(group?.color).toBe("orange")
			expect(group?.groupName).toBe("Infrastructure")
			expect.soft(put.calls).toHaveLength(1)
			expect.soft(put.calls[0]?.body).toEqual({ color: "orange" })
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("rejects without a request when the group is not in the tree", async ({
			expect,
		}) => {
			const putCalls = mockEndpoint(
				"PUT",
				`/api/variables/groups/${GROUP_MISSING}`,
				() => ({}),
			)
			mockTree()
			const api = makeVariableAPI()

			await expect(
				api.updateVariableGroup.mutateAsync({
					id: GROUP_MISSING,
					req: { groupName: "Services" },
				}),
			).rejects.toThrow("invalid variable group data")

			expect.soft(putCalls).toHaveLength(0)
		})

		it("puts the old group back when the update fails", async ({ expect }) => {
			const treeCalls = mockTree()
			const putCalls = mockFailingEndpoint(
				"PUT",
				`/api/variables/groups/${GROUP_A}`,
			)
			const api = makeVariableAPI()

			await expect(
				api.updateVariableGroup.mutateAsync({
					id: GROUP_A,
					req: { groupName: "Services" },
				}),
			).rejects.toThrow()
			await flushPromises()

			expect.soft(putCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	describe("deleteVariableGroup", () => {
		it("removes the group and refreshes the tree", async ({ expect }) => {
			const treeCalls = mockTree()
			const del = mockSnapshotEndpoint(
				"DELETE",
				`/api/variables/groups/${GROUP_A}`,
			)
			const api = makeVariableAPI()

			await expect(
				api.deleteVariableGroup.mutateAsync(GROUP_A),
			).resolves.toBeUndefined()
			await flushPromises()

			expect(del.snapshots[0]?.map((g) => g.id)).toEqual([
				GROUP_BUILTIN,
				GROUP_B,
			])
			expect.soft(del.calls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(1)
		})

		it("puts the group back when the deletion fails", async ({ expect }) => {
			const treeCalls = mockTree()
			const delCalls = mockFailingEndpoint(
				"DELETE",
				`/api/variables/groups/${GROUP_A}`,
			)
			const api = makeVariableAPI()

			await expect(
				api.deleteVariableGroup.mutateAsync(GROUP_A),
			).rejects.toThrow()
			await flushPromises()

			expect.soft(delCalls).toHaveLength(1)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		})
	})

	// a short id stands for an optimistic insert core has not stored yet,
	// so the mutation skips the cache edit, the request and the refresh
	it.for([
		{
			name: "createVariable",
			method: "POST",
			url: VARIABLES_URL,
			invoke: (api: VariableAPI) =>
				api.createVariable.mutateAsync({ ...LIST_REQ, groupId: SHORT_ID }),
		},
		{
			name: "moveVariable",
			method: "PUT",
			url: TREE_URL,
			invoke: (api: VariableAPI) =>
				api.moveVariable.mutateAsync({
					id: SHORT_ID,
					groupId: GROUP_A,
					sortIndex: 0,
				}),
		},
		{
			name: "updateVariable",
			method: "PUT",
			url: `/api/variables/${SHORT_ID}`,
			invoke: (api: VariableAPI) =>
				api.updateVariable.mutateAsync({ id: SHORT_ID, req: TEXT_REQ }),
		},
		{
			name: "deleteVariable",
			method: "DELETE",
			url: `/api/variables/${SHORT_ID}`,
			invoke: (api: VariableAPI) => api.deleteVariable.mutateAsync(SHORT_ID),
		},
		{
			name: "setVariableDefault",
			method: "PUT",
			url: `/api/variables/${SHORT_ID}/default`,
			invoke: (api: VariableAPI) =>
				api.setVariableDefault.mutateAsync({
					id: SHORT_ID,
					req: { value: "x" },
				}),
		},
		{
			name: "setVariableValue",
			method: "PUT",
			url: `/api/variables/${SHORT_ID}/value`,
			invoke: (api: VariableAPI) =>
				api.setVariableValue.mutateAsync({
					id: SHORT_ID,
					req: { value: "x" },
				}),
		},
		{
			name: "resetVariableValue",
			method: "DELETE",
			url: `/api/variables/${SHORT_ID}/value`,
			invoke: (api: VariableAPI) =>
				api.resetVariableValue.mutateAsync(SHORT_ID),
		},
		{
			name: "moveVariableGroup",
			method: "PUT",
			url: "/api/variables/groups/tree",
			invoke: (api: VariableAPI) =>
				api.moveVariableGroup.mutateAsync({ id: SHORT_ID, sortIndex: 0 }),
		},
		{
			name: "updateVariableGroup",
			method: "PUT",
			url: `/api/variables/groups/${SHORT_ID}`,
			invoke: (api: VariableAPI) =>
				api.updateVariableGroup.mutateAsync({
					id: SHORT_ID,
					req: { color: "orange" },
				}),
		},
		{
			name: "deleteVariableGroup",
			method: "DELETE",
			url: `/api/variables/groups/${SHORT_ID}`,
			invoke: (api: VariableAPI) =>
				api.deleteVariableGroup.mutateAsync(SHORT_ID),
		},
	] as const)(
		"$name bails out for an optimistic insert id",
		async ({ method, url, invoke }, { expect }) => {
			seedAuth()
			const treeCalls = mockTree()
			const calls = mockEndpoint(method, url, () => ({}))
			const api = makeVariableAPI()

			await expect(invoke(api)).resolves.toBeUndefined()
			await flushPromises()

			expect.soft(calls).toHaveLength(0)
			expect.soft(treeCalls).toHaveLength(0)
			expect.soft(readTree()).toEqual(makeTree())
		},
	)
})

function makeVariableAPI(): VariableAPI {
	return runInApp(() => useVariableAPI())
}

function seedAuth() {
	seedAuthSession(USER_ID)
	seedAuthOrganization(ORG_ID)
}

// seeds the cached tree and answers its refetch with an empty one, so a
// test can count the refreshes and see that one landed
function mockTree() {
	seedQueryData(TREE_KEY, makeTree())

	return mockEndpoint("GET", TREE_URL, () => [])
}

// records the cached tree as each request arrives, which is the optimistic
// state the mutation wrote
function mockSnapshotEndpoint(
	method: "POST" | "PUT" | "DELETE",
	url: string,
	response: unknown = {},
) {
	const snapshots: (VariableTreeResponse | undefined)[] = []
	const calls = mockEndpoint(method, url, () => {
		snapshots.push(readTree())

		return response
	})

	return { calls, snapshots }
}

function mockFailingEndpoint(method: "POST" | "PUT" | "DELETE", url: string) {
	return mockEndpoint(method, url, () => {
		throw createError({ statusCode: 500 })
	})
}

function readTree() {
	return readQueryData(TREE_KEY) as VariableTreeResponse | undefined
}

function groupOf(tree: VariableTreeResponse | undefined, id: string) {
	return tree?.find((g) => g.id === id)
}

function variableIds(tree: VariableTreeResponse | undefined, groupId: string) {
	return groupOf(tree, groupId)?.variables.map((v) => v.id)
}

function findVariable(tree: VariableTreeResponse | undefined, id: string) {
	return tree?.flatMap((g) => g.variables).find((v) => v.id === id)
}

function makeTree(): VariableTreeResponse {
	return [
		makeGroup(GROUP_BUILTIN, "Built-in", [], true),
		makeGroup(GROUP_A, "Infrastructure", [
			makeListVariable(),
			makeTextVariable(),
		]),
		makeGroup(GROUP_B, "Teams"),
	]
}

function makeGroup(
	id: string,
	groupName: string,
	variables: VariableTreeElement[] = [],
	builtin = false,
): VariableGroupTreeElement {
	return {
		id,
		organizationId: ORG_ID,
		groupName,
		color: "green",
		sortIndex: 0,
		builtin,
		createdAt: "2026-10-13T10:00:00Z",
		createdBy: builtin ? null : USER_ID,
		variables,
	}
}

function makeListVariable(): VariableTreeElement {
	return {
		id: VAR_LIST,
		organizationId: ORG_ID,
		groupId: GROUP_A,
		key: "env",
		name: "Environment",
		description: null,
		type: "list",
		defaultValue: OPTION_A,
		sortIndex: 0,
		builtin: false,
		createdAt: "2026-10-13T10:00:00Z",
		createdBy: USER_ID,
		value: OPTION_B,
		options: [
			{ id: OPTION_A, variableId: VAR_LIST, value: "production", sortIndex: 0 },
			{ id: OPTION_B, variableId: VAR_LIST, value: "staging", sortIndex: 1 },
		],
	}
}

function makeTextVariable(): VariableTreeElement {
	return {
		id: VAR_TEXT,
		organizationId: ORG_ID,
		groupId: GROUP_A,
		key: "region",
		name: "Region",
		description: null,
		type: "text",
		defaultValue: "eu-west-1",
		sortIndex: 1,
		builtin: false,
		createdAt: "2026-10-13T10:00:00Z",
		createdBy: USER_ID,
		value: "us-east-1",
		options: [],
	}
}
