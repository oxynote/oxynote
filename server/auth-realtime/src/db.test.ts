import { describe, it, vi } from "vitest"
import type { Kysely } from "kysely"
import { createDatabase, createStore, type Database } from "./db.js"

// kysely builds a query by chaining, so the stub returns itself from every
// builder call and answers at the terminal one. This is the only file that
// needs to know that — everywhere else takes a Store.
function stubDb(row: unknown) {
	const chain = {
		select: vi.fn(() => chain),
		where: vi.fn(() => chain),
		executeTakeFirst: vi.fn().mockResolvedValue(row),
		executeTakeFirstOrThrow: vi.fn().mockResolvedValue(row),
	}

	const selectFrom = vi.fn(() => chain)
	const db = {
		selectFrom,
		fn: { countAll: () => ({ as: (alias: string) => alias }) },
	} as unknown as Kysely<Database>

	return { store: createStore(db), chain, selectFrom }
}

describe("createStore", () => {
	describe("totalOrganizationCount", () => {
		it("returns the count the query answered with", async ({
			expect,
		}) => {
			const { store, selectFrom } = stubDb({ count: 7 })

			expect(await store.totalOrganizationCount()).toBe(7)
			expect(selectFrom).toHaveBeenCalledWith("organizations")
		})

		// the count decides whether another organization may be
		// created, so an unanswered query must not read as zero
		it("propagates a failed query", async ({ expect }) => {
			const failure = new Error("connection terminated")
			const { store, chain } = stubDb(null)
			chain.executeTakeFirstOrThrow.mockRejectedValue(failure)

			await expect(
				store.totalOrganizationCount(),
			).rejects.toBe(failure)
		})
	})

	describe("userOrganizationId", () => {
		it("returns the organization the user belongs to", async ({
			expect,
		}) => {
			const { store, chain, selectFrom } = stubDb({
				fk_organization_id: "org-1",
			})

			expect(await store.userOrganizationId("user-1")).toBe(
				"org-1",
			)
			expect(selectFrom).toHaveBeenCalledWith(
				"organization_members",
			)
			expect(chain.where).toHaveBeenCalledWith(
				"fk_user_id",
				"=",
				"user-1",
			)
			expect(chain.select).toHaveBeenCalledWith(
				"fk_organization_id",
			)
		})

		it("returns null when the user belongs to no organization", async ({
			expect,
		}) => {
			const { store } = stubDb(undefined)

			expect(
				await store.userOrganizationId("user-1"),
			).toBeNull()
		})

		it("propagates a failed query", async ({ expect }) => {
			const failure = new Error("connection terminated")
			const { store, chain } = stubDb(undefined)
			chain.executeTakeFirst.mockRejectedValue(failure)

			await expect(
				store.userOrganizationId("user-1"),
			).rejects.toBe(failure)
		})
	})

	describe("hasOAuthConsent", () => {
		it("matches the consent on both the client and the user", async ({
			expect,
		}) => {
			const { store, chain, selectFrom } = stubDb({
				client_id: "client-1",
			})

			expect(
				await store.hasOAuthConsent(
					"client-1",
					"user-1",
				),
			).toBe(true)
			expect(selectFrom).toHaveBeenCalledWith(
				"oauth_consents",
			)
			expect(chain.where).toHaveBeenCalledWith(
				"client_id",
				"=",
				"client-1",
			)
			expect(chain.where).toHaveBeenCalledWith(
				"fk_user_id",
				"=",
				"user-1",
			)
		})

		// revoking a client deletes the row, which is what cuts off the
		// access tokens it already holds
		it("reports no consent when the row is gone", async ({
			expect,
		}) => {
			const { store } = stubDb(undefined)

			expect(
				await store.hasOAuthConsent(
					"client-1",
					"user-1",
				),
			).toBe(false)
		})

		it("propagates a failed query", async ({ expect }) => {
			const failure = new Error("connection terminated")
			const { store, chain } = stubDb(undefined)
			chain.executeTakeFirst.mockRejectedValue(failure)

			await expect(
				store.hasOAuthConsent("client-1", "user-1"),
			).rejects.toBe(failure)
		})
	})

	describe("isOrganizationMember", () => {
		it("matches the membership on both the user and the organization", async ({
			expect,
		}) => {
			const { store, chain, selectFrom } = stubDb({
				fk_user_id: "user-1",
			})

			expect(
				await store.isOrganizationMember(
					"user-1",
					"org-1",
				),
			).toBe(true)
			expect(selectFrom).toHaveBeenCalledWith(
				"organization_members",
			)
			expect(chain.where).toHaveBeenCalledWith(
				"fk_user_id",
				"=",
				"user-1",
			)
			expect(chain.where).toHaveBeenCalledWith(
				"fk_organization_id",
				"=",
				"org-1",
			)
		})

		it("reports no membership when the user has left", async ({
			expect,
		}) => {
			const { store } = stubDb(undefined)

			expect(
				await store.isOrganizationMember(
					"user-1",
					"org-1",
				),
			).toBe(false)
		})

		it("propagates a failed query", async ({ expect }) => {
			const failure = new Error("connection terminated")
			const { store, chain } = stubDb(undefined)
			chain.executeTakeFirst.mockRejectedValue(failure)

			await expect(
				store.isOrganizationMember("user-1", "org-1"),
			).rejects.toBe(failure)
		})
	})
})

describe("createDatabase", () => {
	it("closes the underlying connection pool", async ({ expect }) => {
		// the pg pool is lazy and never dialled, so closing it is safe
		// without a database.
		const handle = createDatabase(
			"postgresql://devuser:devpass@localhost:5432/devdb",
		)

		await expect(handle.close()).resolves.toBeUndefined()
	})
})
