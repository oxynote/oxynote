export enum DataSourceType {
	Prometheus = "prometheus",
	Loki = "loki",
	VictoriaLogs = "victorialogs",

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
	// the stored credentials, never their values; absent without any
	credentials?: DataSourceStoredCredentials
}

export interface DataSourceStoredCredentials {
	kind: DataSourceCredentialsKind
	// which credentials are set
	configured: Partial<Record<DataSourceCredentialKey, boolean>>
}

export type DataSourceCredentialsKind = "basic" | "token"

export type DataSourceCredentialKey =
	"username" | "password" | "token" | "caCertificate"

export interface DataSourceBasicCredentials {
	kind: "basic"
	username: string
	password: string
	caCertificate?: string
}

export interface DataSourceTokenCredentials {
	kind: "token"
	token: string
	caCertificate?: string
}

export type DataSourceCredentials =
	DataSourceBasicCredentials | DataSourceTokenCredentials

// an absent field keeps the stored value, and an absent kind the stored
// kind
export type DataSourceCredentialsUpdate = Partial<
	Record<DataSourceCredentialKey, string>
> & { kind?: DataSourceCredentialsKind }

export interface DataSourceCreateRequest {
	type: DataSourceType
	name: string
	url: string
	credentials: DataSourceCredentials
}

export interface DataSourceUpdateRequest {
	name?: string | null
	url?: string | null
	credentials?: DataSourceCredentialsUpdate | null
}

export type DataSourceResponse = DataSource
export type DataSourcesResponse = DataSource[]
export type DataSourceCreateResponse = DataSource
export type DataSourceUpdateResponse = DataSource

export interface DataSourceConnectionResponse {
	status: string
}
