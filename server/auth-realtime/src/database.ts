import { Pool } from 'pg'
import { Kysely, PostgresDialect } from 'kysely'

export const dialect = new PostgresDialect({
  pool: new Pool({
	connectionString: process.env.HEIMDALL_GUARD_DB_DSN,
  })
})

export const db = new Kysely<Database>({
  dialect,
})

export interface Database {
	users: UsersTable
	organizations: OrganizationsTable
	organization_members: OrganizationMembersTable
}

export interface UsersTable {
	id: string
	email: string
}

export interface OrganizationsTable {
	id: string
}

export interface OrganizationMembersTable {
	fk_user_id: string
	fk_organization_id: string
}
