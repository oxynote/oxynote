import { Pool } from "pg"
import { Kysely, PostgresDialect } from "kysely"

// the columns this service reads. Better Auth owns writing these tables and
// core's migrations own creating them; what is declared here is only what a
// query below touches.
export interface Database {
	users: UsersTable
	user_accounts: UserAccountsTable
	organizations: OrganizationsTable
	organization_members: OrganizationMembersTable
	organization_invitations: OrganizationInvitationsTable
	oauth_consents: OAuthConsentsTable
}

export interface UsersTable {
	id: string
	email: string
}

export interface UserAccountsTable {
	fk_user_id: string
	provider_id: string
	password: string | null
}

export interface OrganizationsTable {
	id: string
}

export interface OrganizationMembersTable {
	fk_user_id: string
	fk_organization_id: string
}

export interface OrganizationInvitationsTable {
	id: string
	email: string
	status: string
	expires_at: Date
}

export interface OAuthConsentsTable {
	client_id: string
	fk_user_id: string
}

// every query this service runs, named by what it answers rather than by
// the SQL it builds. Nothing outside this module holds a query builder, so
// a table or column rename is a change to one file, and a caller can be
// tested against four stubbed methods instead of a mocked chain.
export interface Store {
	totalOrganizationCount(): Promise<number>
	organizationMemberCount(organizationId: string): Promise<number>
	hasPendingInvitation(email: string): Promise<boolean>
	credentialPasswordHash(email: string): Promise<string | null>
	userOrganizationId(userId: string): Promise<string | null>
	hasOAuthConsent(clientId: string, userId: string): Promise<boolean>
	isOrganizationMember(
		userId: string,
		organizationId: string,
	): Promise<boolean>
}

// better-auth needs the dialect rather than the query builder: handing it
// the Kysely instance alone leaves the CLI's schema generator unable to
// tell which database it is talking to.
export interface DatabaseHandle {
	store: Store
	dialect: PostgresDialect
	close(): Promise<void>
}

export function createStore(db: Kysely<Database>): Store {
	return {
		async totalOrganizationCount() {
			const res = await db
				.selectFrom("organizations")
				.select(db.fn.countAll<string>().as("count"))
				.executeTakeFirstOrThrow()

			// pg hands a bigint count back as a string.
			return Number(res.count)
		},

		async organizationMemberCount(organizationId) {
			const res = await db
				.selectFrom("organization_members")
				.where(
					"fk_organization_id",
					"=",
					organizationId,
				)
				.select(db.fn.countAll<string>().as("count"))
				.executeTakeFirstOrThrow()

			// pg hands a bigint count back as a string.
			return Number(res.count)
		},

		// better-auth stores both invitation and user addresses
		// lowercased.
		async hasPendingInvitation(email) {
			const res = await db
				.selectFrom("organization_invitations")
				.where("email", "=", email.toLowerCase())
				.where("status", "=", "pending")
				.where("expires_at", ">", new Date())
				.select("id")
				.executeTakeFirst()

			return res !== undefined
		},

		// the user's stored password hash. Null when the user does not
		// exist or has no password, like a social login.
		async credentialPasswordHash(email) {
			const res = await db
				.selectFrom("users")
				.innerJoin(
					"user_accounts",
					"user_accounts.fk_user_id",
					"users.id",
				)
				.where("users.email", "=", email)
				.where(
					"user_accounts.provider_id",
					"=",
					"credential",
				)
				.select("user_accounts.password")
				.executeTakeFirst()

			return res?.password ?? null
		},

		// a user belongs to at most one organization, which is what
		// makes "the user's organization" a single value everywhere
		// else in the service.
		async userOrganizationId(userId) {
			const res = await db
				.selectFrom("organization_members")
				.where("fk_user_id", "=", userId)
				.select("fk_organization_id")
				.executeTakeFirst()

			return res ? res.fk_organization_id : null
		},

		async hasOAuthConsent(clientId, userId) {
			const res = await db
				.selectFrom("oauth_consents")
				.where("client_id", "=", clientId)
				.where("fk_user_id", "=", userId)
				.select("client_id")
				.executeTakeFirst()

			return res !== undefined
		},

		async isOrganizationMember(userId, organizationId) {
			const res = await db
				.selectFrom("organization_members")
				.where("fk_user_id", "=", userId)
				.where(
					"fk_organization_id",
					"=",
					organizationId,
				)
				.select("fk_user_id")
				.executeTakeFirst()

			return res !== undefined
		},
	}
}

export function createDatabase(dsn: string): DatabaseHandle {
	const dialect = new PostgresDialect({
		pool: new Pool({ connectionString: dsn }),
	})

	const db = new Kysely<Database>({ dialect })

	return {
		store: createStore(db),
		dialect,
		// destroy ends the underlying pg pool through the dialect.
		close: () => db.destroy(),
	}
}
