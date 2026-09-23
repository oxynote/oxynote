import type { CoreClient } from "./core.js"
import type { Store } from "./db.js"
import type { Env } from "./env.js"
import type { Logger } from "./logging.js"

// the first account of a single-organization deployment. No mail reaches
// the address, because example.com is reserved (RFC 2606). The password
// follows the signup form's rules: 16 characters, a digit and a symbol.
export const bootstrapAdmin = {
	email: "admin@example.com",
	password: "oxynote-admin-1234",
	name: "Admin",
}

const bootstrapOrganization = {
	name: "Oxynote",
	slug: "oxynote",
}

// the parts of better-auth's internal adapter that the bootstrap uses.
export interface AccountAdapter {
	findUserByEmail(email: string): Promise<{ user: { id: string } } | null>
	createUser(
		user: { email: string; name: string; emailVerified: boolean },
		source: { method: "email-password" },
	): Promise<{ id: string }>
	linkAccount(account: {
		userId: string
		providerId: string
		accountId: string
		password: string
	}): Promise<unknown>
}

export interface BootstrapDeps {
	env: Env
	store: Store
	core: CoreClient
	log: Logger
	accounts: AccountAdapter
	hashPassword: (password: string) => Promise<string>
	// better-auth's createOrganization. It runs the organization hooks
	// and makes the user the owner.
	createOrganization: (organization: {
		name: string
		slug: string
		userId: string
	}) => Promise<{ id: string }>
}

// bootstrapSingleOrganization creates the organization and its admin when a
// single-organization deployment has none yet. It runs before the service
// listens, so no signup can race it. An admin left over from a failed run
// is reused.
export async function bootstrapSingleOrganization({
	env,
	store,
	core,
	log,
	accounts,
	hashPassword,
	createOrganization,
}: BootstrapDeps): Promise<void> {
	if (env.maxOrganizations !== 1) {
		return
	}

	if ((await store.totalOrganizationCount()) > 0) {
		return
	}

	const existing = await accounts.findUserByEmail(bootstrapAdmin.email)
	let userId = existing?.user.id

	if (!userId) {
		// the admin starts verified. Its address cannot receive a
		// verification link.
		const user = await accounts.createUser(
			{
				email: bootstrapAdmin.email,
				name: bootstrapAdmin.name,
				emailVerified: true,
			},
			{ method: "email-password" },
		)

		await accounts.linkAccount({
			userId: user.id,
			providerId: "credential",
			accountId: user.id,
			password: await hashPassword(bootstrapAdmin.password),
		})

		userId = user.id
	}

	const organization = await createOrganization({
		...bootstrapOrganization,
		userId,
	})

	await core.setDefaultOrganizationLogo(organization.id)

	log.warn(
		`created the ${bootstrapOrganization.name} organization. Sign in as ${bootstrapAdmin.email} with the default password and change both.`,
	)
}

export interface DefaultAdmin {
	// whether the account still uses the default password.
	defaultPassword: boolean
}

export interface DefaultAdminLookupDeps {
	store: Store
	verifyPassword: (data: {
		hash: string
		password: string
	}) => Promise<boolean>
}

// createDefaultAdminLookup finds the bootstrap admin by its default email.
// It answers null once that account is gone or has a new email. Otherwise
// it tells whether the account still uses the default password. Checking a
// password is slow on purpose. So that answer is cached per stored hash,
// and checked again only when the hash changes.
export function createDefaultAdminLookup({
	store,
	verifyPassword,
}: DefaultAdminLookupDeps): () => Promise<DefaultAdmin | null> {
	let checked: { hash: string; matches: boolean } | undefined

	return async () => {
		const hash = await store.credentialPasswordHash(
			bootstrapAdmin.email,
		)
		if (!hash) {
			return null
		}

		if (checked?.hash !== hash) {
			checked = {
				hash,
				matches: await verifyPassword({
					hash,
					password: bootstrapAdmin.password,
				}),
			}
		}

		return { defaultPassword: checked.matches }
	}
}
