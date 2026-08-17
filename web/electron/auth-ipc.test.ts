import { ipcMain } from "electron"
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest"
import { registerAuthIpcHandlers } from "./auth-ipc"

vi.mock("electron", () => ({
	ipcMain: {
		handle: vi.fn(),
	},
}))

// one leaf mock per whitelisted channel, mirroring the authClient method
// tree that auth-ipc delegates to
const { leaves } = vi.hoisted(() => {
	const leaves = {
		"auth:getSession": vi.fn(),
		"auth:signInEmailPassword": vi.fn(),
		"auth:signUpEmailPassword": vi.fn(),
		"auth:requestPasswordReset": vi.fn(),
		"auth:changePassword": vi.fn(),
		"auth:listAccounts": vi.fn(),
		"auth:updateUser": vi.fn(),
		"auth:changeEmail": vi.fn(),
		"auth:deleteUser": vi.fn(),
		"auth:getFullOrganization": vi.fn(),
		"auth:checkOrganizationSlug": vi.fn(),
		"auth:createOrganization": vi.fn(),
		"auth:setActiveOrganization": vi.fn(),
		"auth:acceptOrganizationInvitation": vi.fn(),
		"auth:updateOrganization": vi.fn(),
		"auth:inviteOrganizationMember": vi.fn(),
		"auth:cancelOrganizationInvitation": vi.fn(),
		"auth:removeOrganizationMember": vi.fn(),
	}

	return { leaves }
})

vi.mock("./auth-client", () => ({
	authClient: {
		getSession: leaves["auth:getSession"],
		signIn: { email: leaves["auth:signInEmailPassword"] },
		signUp: { email: leaves["auth:signUpEmailPassword"] },
		requestPasswordReset: leaves["auth:requestPasswordReset"],
		changePassword: leaves["auth:changePassword"],
		listAccounts: leaves["auth:listAccounts"],
		updateUser: leaves["auth:updateUser"],
		changeEmail: leaves["auth:changeEmail"],
		deleteUser: leaves["auth:deleteUser"],
		organization: {
			getFullOrganization: leaves["auth:getFullOrganization"],
			checkSlug: leaves["auth:checkOrganizationSlug"],
			create: leaves["auth:createOrganization"],
			setActive: leaves["auth:setActiveOrganization"],
			acceptInvitation: leaves["auth:acceptOrganizationInvitation"],
			update: leaves["auth:updateOrganization"],
			inviteMember: leaves["auth:inviteOrganizationMember"],
			cancelInvitation: leaves["auth:cancelOrganizationInvitation"],
			removeMember: leaves["auth:removeOrganizationMember"],
		},
	},
}))

const channels = Object.keys(leaves)

// channels whose handlers deliberately drop the renderer-provided args
const argslessChannels = new Set([
	"auth:getSession",
	"auth:listAccounts",
	"auth:getFullOrganization",
])

// eslint-disable-next-line @typescript-eslint/unbound-method -- the vi.mock replacement is a this-independent vi.fn
const handleMock = ipcMain.handle as Mock

function registeredHandler(channel: string) {
	const call = handleMock.mock.calls.find(([name]) => name === channel)
	if (!call) {
		throw new Error(`channel ${channel} was not registered`)
	}

	return call[1] as (event: unknown, args: unknown) => Promise<unknown>
}

// sequential by exception: these tests assert call accounting on shared
// module-level mocks (vi.mock singletons), which cannot be isolated
// across concurrently interleaving tests
describe("registerAuthIpcHandlers", { concurrent: false }, () => {
	// restoreMocks only covers vi.spyOn spies, so these hand-made vi.fn
	// singletons are reset here explicitly
	beforeEach(() => {
		handleMock.mockClear()
		for (const leaf of Object.values(leaves)) {
			leaf.mockReset()
		}
	})

	it("registers exactly the whitelisted channels", () => {
		registerAuthIpcHandlers()

		const registered = handleMock.mock.calls.map(([name]) => name as string)
		expect([...registered].sort()).toEqual([...channels].sort())
	})

	it.for(
		channels.map((channel) => ({
			name: `delegates ${channel} to its auth client method`,
			channel,
		})),
	)("$name", async ({ channel }, { expect }) => {
		const leaf = leaves[channel as keyof typeof leaves]
		const result = { ok: channel }
		leaf.mockResolvedValue(result)

		registerAuthIpcHandlers()
		const args = { payload: channel }

		await expect(registeredHandler(channel)({}, args)).resolves.toBe(result)

		if (argslessChannels.has(channel)) {
			expect(leaf).toHaveBeenCalledTimes(1)
			expect(leaf).toHaveBeenCalledWith()
		} else {
			expect(leaf).toHaveBeenCalledTimes(1)
			expect(leaf).toHaveBeenCalledWith(args)
		}

		// no other auth operation may fire as a side effect
		for (const [other, otherLeaf] of Object.entries(leaves)) {
			if (other !== channel) {
				expect(otherLeaf).toHaveBeenCalledTimes(0)
			}
		}
	})

	it("does not invoke any auth operation during registration", () => {
		registerAuthIpcHandlers()

		for (const leaf of Object.values(leaves)) {
			expect(leaf).toHaveBeenCalledTimes(0)
		}
	})
})
