export enum DataSourceType {
	Prometheus = "prometheus",

	// SQL
	PostgreSQL = "postgresql",
	MySQL = "mysql",
	MariaDB = "mariadb",
}

export function isDataSourceSQLBased(
	type: DataSourceType | null | undefined,
): boolean {
	switch (type) {
		case DataSourceType.PostgreSQL:
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
	InvalidSigningSecret = "invalid_signing_secret",

	// local-only
	LocalOptimisticInsert = "local_optimistic_insert", // aka pending
}

export const DataSourceErrorStatuses = [
	DataSourceStatus.Unauthorized,
	DataSourceStatus.Unreachable,
	DataSourceStatus.VersionNotSupported,
	DataSourceStatus.NotReadOnly,
	DataSourceStatus.InvalidSigningSecret,
] as const

export interface DataSource {
	id: string
	name: string
	type: DataSourceType
	url: string
	status: DataSourceStatus
	createdAt: Date | string
	updatedAt: Date | string | null
}

export interface DataSourceCredentialsPrometheus {
	username: string
	password: string
}

export interface DataSourceCredentialsSQL {
	username: string
	password: string
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
