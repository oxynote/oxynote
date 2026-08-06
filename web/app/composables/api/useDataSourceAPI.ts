import { nanoid } from "nanoid"
import isDeepEqual from "fast-deep-equal"

const DATA_SOURCE_QUERY_KEYS = {
	list: ["data-sources"] as const,
	detail: (dataSourceId: string) => ["data-sources", dataSourceId] as const,
	connection: (dataSourceId: string) =>
		["data-sources", dataSourceId, "connection"] as const,
	query: (dataSourceId: string, query: string, timeRange: string) =>
		["generic-query", dataSourceId, "query", query, timeRange] as const,
	queries: (dataSourceId: string, queryStrings: string[], timeRange: string) =>
		[
			"generic-query",
			dataSourceId,
			"queries",
			...queryStrings,
			timeRange,
		] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()
	const queryCache = useQueryCache()

	const fetchDataSources = useQuery({
		key: DATA_SOURCE_QUERY_KEYS.list,
		query: async () => {
			return await $coreAPIClient<DataSourcesResponse>(`/api/data-sources`, {
				method: "GET",
			})
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 3 * 60 * 1000, // 3 mins
		autoRefetch: true,
	})

	function useFetchDataSourceById(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
	) {
		return useQuery({
			key: () => DATA_SOURCE_QUERY_KEYS.detail(toValue(dataSourceIdRef) || ""),
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				if (!dataSourceId) {
					return null
				}

				return await $coreAPIClient<DataSourceResponse>(
					`/api/data-sources/${dataSourceId}`,
					{
						method: "GET",
					},
				)
			},
			enabled: () =>
				toValue(dataSourceIdRef) !== null &&
				toValue(dataSourceIdRef) !== undefined,
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 3 * 60 * 1000, // 3 mins
			autoRefetch: true,
		})
	}

	function testDataSourceConnection(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
	) {
		return useQuery({
			key: () =>
				DATA_SOURCE_QUERY_KEYS.connection(toValue(dataSourceIdRef) || ""),
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				if (!dataSourceId) {
					return null
				}

				return await $coreAPIClient<DataSourceConnectionResponse>(
					`/api/data-sources/${dataSourceId}/connection`,
					{
						method: "GET",
					},
				)
			},
			enabled: () =>
				toValue(dataSourceIdRef) !== null &&
				toValue(dataSourceIdRef) !== undefined,
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 0, // always fetch fresh
			autoRefetch: false,
		})
	}

	const createDataSource = useMutation({
		onMutate: (req) => {
			const oldDataSources = clone(
				queryCache.getQueryData<DataSourcesResponse>(
					DATA_SOURCE_QUERY_KEYS.list,
				),
			)
			const newDataSource: DataSource = {
				id: nanoid(),
				name: req.name,
				type: req.type,
				url: req.url,
				status: DataSourceStatus.LocalOptimisticInsert,
				createdAt: new Date(),
				updatedAt: null,
			}
			const newDataSources = [newDataSource, ...(oldDataSources || [])]

			queryCache.setQueryData(DATA_SOURCE_QUERY_KEYS.list, newDataSources)
			queryCache.cancelQueries({ key: DATA_SOURCE_QUERY_KEYS.list })

			return { newDataSources, oldDataSources }
		},
		mutation: async (req: DataSourceCreateRequest) => {
			return await $coreAPIClient<DataSourceCreateResponse>(
				`/api/data-sources`,
				{
					method: "POST",
					body: req,
				},
			)
		},
		async onSuccess() {
			await queryCache.invalidateQueries({ key: DATA_SOURCE_QUERY_KEYS.list })
		},
		onError(_err, _req, { oldDataSources, newDataSources }) {
			const cachedDataSources = queryCache.getQueryData<DataSourcesResponse>(
				DATA_SOURCE_QUERY_KEYS.list,
			)
			if (!isDeepEqual(newDataSources, cachedDataSources)) {
				return
			}

			// rollback
			queryCache.setQueryData(DATA_SOURCE_QUERY_KEYS.list, oldDataSources)
		},
	})

	const updateDataSource = useMutation({
		onMutate: ({ dataSourceId, req }) => {
			if (!isXid(dataSourceId)) {
				// optimisticInserts use nanoid
				return
			}

			const oldDataSources = clone(
				queryCache.getQueryData<DataSourcesResponse>(
					DATA_SOURCE_QUERY_KEYS.list,
				),
			)
			const newDataSources = clone(oldDataSources) || []

			const oldDataSourceById = clone(
				queryCache.getQueryData<DataSourceResponse>(
					DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
				),
			)
			const newDataSourceById = clone(oldDataSourceById)

			for (const ds of newDataSources) {
				if (ds.id === dataSourceId) {
					if (req.name) {
						ds.name = req.name
					}

					if (req.url) {
						ds.url = req.url
					}

					ds.updatedAt = new Date()

					break
				}
			}

			if (oldDataSourceById && newDataSourceById) {
				if (req.name) {
					newDataSourceById.name = req.name
				}

				if (req.url) {
					newDataSourceById.url = req.url
				}

				newDataSourceById.updatedAt = new Date()

				queryCache.setQueryData(
					DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
					newDataSourceById,
				)
				queryCache.cancelQueries({
					key: DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
				})
			}

			queryCache.setQueryData(DATA_SOURCE_QUERY_KEYS.list, newDataSources)
			queryCache.cancelQueries({ key: DATA_SOURCE_QUERY_KEYS.list })

			return {
				newDataSources,
				oldDataSources,
				newDataSourceById,
				oldDataSourceById,
			}
		},
		mutation: async ({
			dataSourceId,
			req,
		}: {
			dataSourceId: string
			req: DataSourceUpdateRequest
		}) => {
			return await $coreAPIClient<DataSourceUpdateResponse>(
				`/api/data-sources/${dataSourceId}`,
				{
					method: "PUT",
					body: req,
				},
			)
		},
		async onSuccess(_data, { dataSourceId }, { newDataSourceById }) {
			if (!isXid(dataSourceId)) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: DATA_SOURCE_QUERY_KEYS.list,
			})

			if (newDataSourceById) {
				await queryCache.invalidateQueries({
					key: DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
				})
			}
		},
		onError(
			_err,
			{ dataSourceId },
			{ oldDataSources, newDataSources, newDataSourceById, oldDataSourceById },
		) {
			if (!isXid(dataSourceId)) {
				// optimisticInserts use nanoid
				return
			}

			const cachedDataSources = queryCache.getQueryData(
				DATA_SOURCE_QUERY_KEYS.list,
			)
			if (isDeepEqual(newDataSources, cachedDataSources)) {
				// rollback
				queryCache.setQueryData(DATA_SOURCE_QUERY_KEYS.list, oldDataSources)
			}

			const cachedDataSourceById = queryCache.getQueryData(
				DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
			)
			if (
				newDataSourceById &&
				isDeepEqual(newDataSourceById, cachedDataSourceById)
			) {
				// rollback
				queryCache.setQueryData(
					DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
					oldDataSourceById,
				)
			}
		},
	})

	const deleteDataSource = useMutation({
		onMutate: (dataSourceId) => {
			if (!isXid(dataSourceId)) {
				// optimisticInserts use nanoid
				return
			}

			const oldDataSources = clone(
				queryCache.getQueryData<DataSourcesResponse>(
					DATA_SOURCE_QUERY_KEYS.list,
				),
			)
			const newDataSources = clone(oldDataSources) || []

			const oldDataSourceById = clone(
				queryCache.getQueryData<DataSourceResponse>(
					DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
				),
			)

			const index = newDataSources.findIndex((ds) => ds.id === dataSourceId)
			if (index !== -1) {
				newDataSources.splice(index, 1)
			}

			if (oldDataSourceById) {
				queryCache.setQueryData(
					DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
					undefined,
				)
			}

			queryCache.setQueryData(DATA_SOURCE_QUERY_KEYS.list, newDataSources)
			queryCache.cancelQueries({ key: DATA_SOURCE_QUERY_KEYS.list })

			return { newDataSources, oldDataSources, oldDataSourceById }
		},
		mutation: async (dataSourceId: string) => {
			await $coreAPIClient(`/api/data-sources/${dataSourceId}`, {
				method: "DELETE",
			})
		},
		async onSuccess(_data, dataSourceId, { oldDataSourceById }) {
			if (!isXid(dataSourceId)) {
				// optimisticInserts use nanoid
				return
			}

			await queryCache.invalidateQueries({
				key: DATA_SOURCE_QUERY_KEYS.list,
			})

			if (oldDataSourceById) {
				await queryCache.invalidateQueries({
					key: DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
				})
			}
		},
		onError(
			_err,
			dataSourceId,
			{ oldDataSources, newDataSources, oldDataSourceById },
		) {
			if (!isXid(dataSourceId)) {
				// optimisticInserts use nanoid
				return
			}

			const cachedDataSources = queryCache.getQueryData(
				DATA_SOURCE_QUERY_KEYS.list,
			)
			if (isDeepEqual(newDataSources, cachedDataSources)) {
				// rollback
				queryCache.setQueryData(DATA_SOURCE_QUERY_KEYS.list, oldDataSources)
			}

			const cachedDataSourceById = queryCache.getQueryData(
				DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
			)
			if (oldDataSourceById && cachedDataSourceById === undefined) {
				// rollback
				queryCache.setQueryData(
					DATA_SOURCE_QUERY_KEYS.detail(dataSourceId),
					oldDataSourceById,
				)
			}
		},
	})

	function useGenericQuery(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<GenericQueryParams | null | undefined>,
	) {
		return useQuery({
			key: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				return DATA_SOURCE_QUERY_KEYS.query(
					dataSourceId || "",
					params?.q || "",
					params?.timeRangeKey || "",
				)
			},
			query: async () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.q) {
					return null
				}

				const queryParams = new URLSearchParams({
					q: params.q,
					chartType: params.chartType,
					...formatQueryTimeRange(params),
				})

				try {
					return await $coreAPIClient<GenericQueryResult>(
						`/api/data-sources/${dataSourceId}/query?${queryParams.toString()}`,
						{ method: "GET" },
					)
				} catch (err) {
					const parsedErr = parseQueryError(err)
					if (parsedErr) {
						return parsedErr
					}

					throw err
				}
			},
			enabled: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				return (
					dataSourceId !== undefined &&
					dataSourceId !== null &&
					params?.q !== undefined &&
					params?.q !== null &&
					params.q.trim().length > 0
				)
			},
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 4.5 * 1000, // 4.5 secs
			autoRefetch: false,
		})
	}

	function useMultipleGenericQueries(
		dataSourceIdRef: MaybeRefOrGetter<string | null | undefined>,
		paramsRef: MaybeRefOrGetter<
			GenericMultipleQueriesParams | null | undefined
		>,
		disableFetch: MaybeRefOrGetter<boolean>,
	) {
		return useQuery({
			key: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)
				const queries = clone(params?.queries)

				return DATA_SOURCE_QUERY_KEYS.queries(
					dataSourceId || "",
					queries?.sort() || [],
					params?.timeRangeKey || "",
				)
			},
			query: async (): Promise<GenericQueryResult[]> => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)

				if (!dataSourceId || !params?.queries?.length) {
					return []
				}

				const timeRangeParams = formatQueryTimeRange(params)
				const results = await Promise.all(
					params.queries.map(async (q) => {
						const queryParams = new URLSearchParams({
							q: q,
							chartType: params.chartType,
							...timeRangeParams,
						})

						try {
							return await $coreAPIClient<GenericQueryResult>(
								`/api/data-sources/${dataSourceId}/query?${queryParams.toString()}`,
								{ method: "GET" },
							)
						} catch (err) {
							const parsedErr = parseQueryError(err)
							if (parsedErr) {
								return parsedErr
							}

							throw err
						}
					}),
				)

				return results
			},
			enabled: () => {
				const dataSourceId = toValue(dataSourceIdRef)
				const params = toValue(paramsRef)
				const disable = toValue(disableFetch)

				return (
					dataSourceId !== undefined &&
					dataSourceId !== null &&
					params?.queries !== undefined &&
					params.queries.length > 0 &&
					params.queries.every((q) => q.trim().length > 0) &&
					!disable
				)
			},
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			autoRefetch: false,
			placeholderData: (previousData) => previousData, // to avoid "no data" state when params change
			// we don't want query data to be refreshed automatically when
			// metric blocks are dragged around in the editor (when they are
			// dropped they sometimes trigger a remount which causes the data
			// to be refreshed which ), so we set
			staleTime: 60 * 60 * 24 * 1000, // 24 hours (longest refresh interval allowed)
		})
	}

	function parseQueryError(err: unknown): GenericQueryResult | null {
		if (
			err &&
			typeof err === "object" &&
			"data" in err &&
			err.data &&
			typeof err.data === "object" &&
			"message" in err.data &&
			typeof err.data.message === "string" &&
			"code" in err.data &&
			err.data.code === "query.error"
		) {
			return {
				status: GenericQueryResultStatus.QueryError,
				data: [],
				queryErrorMessage: err.data.message,
			}
		}

		return null
	}

	return {
		useFetchDataSourceById,
		testDataSourceConnection,
		createDataSource,
		updateDataSource,
		fetchDataSources,
		deleteDataSource,
		useGenericQuery,
		useMultipleGenericQueries,
	}
}
