export enum DataSourceType {
	Prometheus = "prometheus",

	// SQL
	PostgreSQL = "postgresql",
	Supabase = "supabase",
	MySQL = "mysql",
	MariaDB = "mariadb",
}

export function isDataSourceSQLBased(
	type: DataSourceType | null | undefined,
): boolean {
	switch (type) {
		case DataSourceType.PostgreSQL:
		case DataSourceType.Supabase:
		case DataSourceType.MySQL:
		case DataSourceType.MariaDB:
			return true
		default:
			return false
	}
}

export enum DataSourceStatus {
	Success = "success",
	Unauthorized = "unauthorized",
	Unreachable = "unreachable",
	VersionNotSupported = "version_not_supported",
	NotReadOnly = "not_read_only",
	InvalidEncryptionKey = "invalid_encryption_key",
	InvalidCACertificate = "invalid_ca_certificate",

	// local-only
	LocalOptimisticInsert = "local_optimistic_insert", // aka pending
}

export const DataSourceErrorStatuses = [
	DataSourceStatus.Unauthorized,
	DataSourceStatus.Unreachable,
	DataSourceStatus.VersionNotSupported,
	DataSourceStatus.NotReadOnly,
	DataSourceStatus.InvalidEncryptionKey,
	DataSourceStatus.InvalidCACertificate,
] as const

export interface DataSource {
	id: string
	name: string
	type: DataSourceType
	url: string
	status: DataSourceStatus
	createdAt: Date | string
	updatedAt: Date | string | null
	// which credentials are set, never their values; an older server omits it
	configured?: Partial<Record<DataSourceCredentialKey, boolean>>
}

export type DataSourceCredentialKey = "username" | "password" | "caCertificate"

export interface DataSourceCredentialsPrometheus {
	username: string
	password: string
	caCertificate?: string
}

export interface DataSourceCredentialsSQL {
	username: string
	password: string
	caCertificate?: string
}

export type DataSourceCredentials =
	DataSourceCredentialsPrometheus | DataSourceCredentialsSQL

export interface DataSourceCreateRequest {
	type: DataSourceType
	name: string
	url: string
	credentials: DataSourceCredentials
}

export interface DataSourceUpdateRequest {
	name?: string | null
	url?: string | null
	credentials?: Partial<DataSourceCredentials> | null
}

export type DataSourceResponse = DataSource
export type DataSourcesResponse = DataSource[]
export type DataSourceCreateResponse = DataSource
export type DataSourceUpdateResponse = DataSource

export interface DataSourceConnectionResponse {
	status: string
}
