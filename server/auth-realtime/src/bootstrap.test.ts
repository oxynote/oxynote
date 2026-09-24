import { describe, it, vi } from "vitest"
import {
	bootstrapSingleOrganization,
	createDefaultAdminLookup,
} from "./bootstrap.js"
import { stubLog, stubStore, testEnv } from "./test-helpers.js"

// a single-organization deployment with no organization and no admin yet,
// unless a test says otherwise.
function deps(
	overrides: {
		maxOrganizations?: number
		organizations?: number
		existingAdmin?: string
	} = {},
) {
	const store = stubStore()
	store.totalOrganizationCount.mockResolvedValue(
		overrides.organizations ?? 0,
	)

	const accounts = {
		findUserByEmail: vi.fn().mockResolvedValue(
			overrides.existingAdmin
				? {
						user: {
							id: overrides.existingAdmin,
						},
					}
				: null,
		),
		createUser: vi.fn().mockResolvedValue({ id: "admin-1" }),
		linkAccount: vi.fn().mockResolvedValue({}),
	}

	return {
		env: testEnv({
			maxOrganizations: overrides.maxOrganizations ?? 1,
		}),
		store,
		log: stubLog(),
		accounts,
		hashPassword: vi.fn().mockResolvedValue("hashed"),
		createOrganization: vi.fn().mockResolvedValue({}),
	}
}

describe("bootstrapSingleOrganization", () => {
	it("creates the admin, the organization and its logo", async ({
		expect,
	}) => {
		const d = deps()

		await bootstrapSingleOrganization(d)

		expect(d.accounts.createUser).toHaveBeenCalledWith(
			{
				email: "admin@example.com",
				name: "Admin",
				emailVerified: true,
			},
			{ method: "email-password" },
		)
		expect(d.hashPassword).toHaveBeenCalledWith(
			"oxynote-admin-1234",
		)
		expect(d.accounts.linkAccount).toHaveBeenCalledWith({
			userId: "admin-1",
			providerId: "credential",
			accountId: "admin-1",
			password: "hashed",
		})
		expect(d.createOrganization).toHaveBeenCalledWith({
			name: "Oxynote",
			slug: "oxynote",
			userId: "admin-1",
		})
		expect.soft(d.log.warn).toHaveBeenCalledTimes(1)
	})

	it("reuses an admin left over from a failed run", async ({
		expect,
	}) => {
		const d = deps({ existingAdmin: "admin-0" })

		await bootstrapSingleOrganization(d)

		expect(d.createOrganization).toHaveBeenCalledWith({
			name: "Oxynote",
			slug: "oxynote",
			userId: "admin-0",
		})
		expect.soft(d.accounts.createUser).not.toHaveBeenCalled()
		expect.soft(d.accounts.linkAccount).not.toHaveBeenCalled()
		expect.soft(d.hashPassword).not.toHaveBeenCalled()
	})

	it.for([
		{
			name: "more than one organization is allowed",
			input: { maxOrganizations: 100 },
		},
		{
			name: "an organization exists",
			input: { organizations: 1 },
		},
	])("does nothing when $name", async ({ input }, { expect }) => {
		const d = deps(input)

		await bootstrapSingleOrganization(d)

		expect.soft(d.accounts.findUserByEmail).not.toHaveBeenCalled()
		expect.soft(d.createOrganization).not.toHaveBeenCalled()
		expect.soft(d.log.warn).not.toHaveBeenCalled()
	})

	it("propagates a failed organization creation", async ({ expect }) => {
		const failure = new Error("core unreachable")
		const d = deps()
		d.createOrganization.mockRejectedValue(failure)

		await expect(bootstrapSingleOrganization(d)).rejects.toBe(
			failure,
		)
		expect.soft(d.log.warn).not.toHaveBeenCalled()
	})

	it("propagates a failed admin creation", async ({ expect }) => {
		const failure = new Error("connection terminated")
		const d = deps()
		d.accounts.createUser.mockRejectedValue(failure)

		await expect(bootstrapSingleOrganization(d)).rejects.toBe(
			failure,
		)
		expect.soft(d.createOrganization).not.toHaveBeenCalled()
	})
})

describe("createDefaultAdminLookup", () => {
	it.for([
		{ name: "the default password", input: true },
		{ name: "a changed password", input: false },
	])("finds the admin with $name", async ({ input }, { expect }) => {
		const store = stubStore()
		store.credentialPasswordHash.mockResolvedValue("hash-1")
		const verifyPassword = vi.fn().mockResolvedValue(input)

		expect(
			await createDefaultAdminLookup({
				store,
				verifyPassword,
			})(),
		).toEqual({ defaultPassword: input })
		expect.soft(store.credentialPasswordHash).toHaveBeenCalledWith(
			"admin@example.com",
		)
		expect.soft(verifyPassword).toHaveBeenCalledWith({
			hash: "hash-1",
			password: "oxynote-admin-1234",
		})
	})

	it("answers null without verifying once the default email is gone", async ({
		expect,
	}) => {
		const store = stubStore()
		store.credentialPasswordHash.mockResolvedValue(null)
		const verifyPassword = vi.fn().mockResolvedValue(true)

		expect(
			await createDefaultAdminLookup({
				store,
				verifyPassword,
			})(),
		).toBeNull()
		expect.soft(verifyPassword).not.toHaveBeenCalled()
	})

	it("verifies an unchanged hash only once", async ({ expect }) => {
		const store = stubStore()
		store.credentialPasswordHash.mockResolvedValue("hash-1")
		const verifyPassword = vi.fn().mockResolvedValue(true)
		const check = createDefaultAdminLookup({
			store,
			verifyPassword,
		})

		await check()
		const answer = await check()

		expect(answer).toEqual({ defaultPassword: true })
		expect.soft(store.credentialPasswordHash).toHaveBeenCalledTimes(
			2,
		)
		expect.soft(verifyPassword).toHaveBeenCalledTimes(1)
	})

	it("verifies again once the password changes", async ({ expect }) => {
		const store = stubStore()
		store.credentialPasswordHash
			.mockResolvedValueOnce("hash-1")
			.mockResolvedValueOnce("hash-2")
		const verifyPassword = vi
			.fn()
			.mockResolvedValueOnce(true)
			.mockResolvedValueOnce(false)
		const check = createDefaultAdminLookup({
			store,
			verifyPassword,
		})

		await check()
		const answer = await check()

		expect(answer).toEqual({ defaultPassword: false })
		expect.soft(verifyPassword).toHaveBeenCalledTimes(2)
	})

	it("propagates a failed lookup", async ({ expect }) => {
		const failure = new Error("connection terminated")
		const store = stubStore()
		store.credentialPasswordHash.mockRejectedValue(failure)

		await expect(
			createDefaultAdminLookup({
				store,
				verifyPassword: vi.fn(),
			})(),
		).rejects.toBe(failure)
	})
})
