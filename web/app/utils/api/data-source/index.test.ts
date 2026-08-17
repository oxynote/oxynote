import { describe, it } from "vitest"
import { DataSourceType, isDataSourceSQLBased } from "./index"

describe("isDataSourceSQLBased", () => {
	it.for([
		{ type: DataSourceType.PostgreSQL, expected: true },
		{ type: DataSourceType.MySQL, expected: true },
		{ type: DataSourceType.MariaDB, expected: true },
		{ type: DataSourceType.Prometheus, expected: false },
		{ type: null, expected: false },
		{ type: undefined, expected: false },
	])("returns $expected for $type", ({ type, expected }, { expect }) => {
		expect(isDataSourceSQLBased(type)).toBe(expected)
	})
})
