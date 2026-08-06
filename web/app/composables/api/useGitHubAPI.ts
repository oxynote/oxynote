import type {
	GitHubConnectionStatus,
	GitHubFileTreeItem,
	GitHubInstallResponse,
	GitHubRepository,
} from "~/utils/api/github"
import isDeepEqual from "fast-deep-equal"

const GITHUB_QUERY_KEYS = {
	listRepositories: () => ["repositories"] as const,
	listBranches: (repoName: string) =>
		["repositories", "branches", repoName] as const,
	listFileTreeItems: (repoName: string, branchName: string) =>
		["repositories", "branches", repoName, "tree", branchName] as const,
	connected: ["github", "connected"] as const,
}

export default function () {
	const { $coreAPIClient } = useNuxtApp()
	const queryCache = useQueryCache()

	const fetchGitHubConnectionStatus = useQuery<GitHubConnectionStatus>({
		key: GITHUB_QUERY_KEYS.connected,
		query: async () => {
			return await $coreAPIClient<GitHubConnectionStatus>(`/api/github`, {
				method: "GET",
			})
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 3 * 60 * 1000, // 3mins
		autoRefetch: true,
	})

	// whether the server has the GitHub App integration configured at all.
	// Treats "unknown" (still loading) as configured so GitHub UI doesn't
	// flicker away on deployments that do have it.
	const gitHubConfigured = computed(
		() => fetchGitHubConnectionStatus.data.value?.configured !== false,
	)

	async function fetchGitHubInstallURL(): Promise<GitHubInstallResponse> {
		// we don't want to use useQuery here as installs are
		// typically one-off and we don't want to cache them
		return await $coreAPIClient<GitHubInstallResponse>(`/api/github/install`, {
			method: "GET",
		})
	}

	const connectGitHub = useMutation({
		// no optimistic updates possible here
		mutation: async (queryParams: URLSearchParams) => {
			const query = queryParams.toString()
			const url = query ? `/api/github/connect?${query}` : "/api/github/connect"

			return await $coreAPIClient(url, { method: "GET" })
		},
	})

	const disconnectGitHub = useMutation({
		onMutate: async () => {
			const oldStatus = clone(
				queryCache.getQueryData<GitHubConnectionStatus>(
					GITHUB_QUERY_KEYS.connected,
				),
			)
			const newStatus: GitHubConnectionStatus = {
				connected: false,
				configured: oldStatus?.configured ?? true,
			}

			queryCache.setQueryData(GITHUB_QUERY_KEYS.connected, newStatus)
			queryCache.cancelQueries({
				key: GITHUB_QUERY_KEYS.connected,
			})

			return { newStatus, oldStatus }
		},
		mutation: async () => {
			return await $coreAPIClient(`/api/github`, { method: "DELETE" })
		},
		async onSuccess() {
			await queryCache.invalidateQueries({ key: GITHUB_QUERY_KEYS.connected })
		},
		onError(_err, _data, { oldStatus, newStatus }) {
			const cachedStatus = queryCache.getQueryData<GitHubConnectionStatus>(
				GITHUB_QUERY_KEYS.connected,
			)
			if (!isDeepEqual(newStatus, cachedStatus)) {
				return
			}

			// rollback
			queryCache.setQueryData(GITHUB_QUERY_KEYS.connected, oldStatus)
		},
	})

	const fetchGitHubRepositories = useQuery<GitHubRepository[]>({
		key: GITHUB_QUERY_KEYS.listRepositories,
		query: async () => {
			if (!(await fetchGitHubConnectionStatus.refresh()).data?.connected) {
				return []
			}

			return await $coreAPIClient<GitHubRepository[]>(
				`/api/github/repositories`,
				{
					method: "GET",
				},
			)
		},
		refetchOnMount: false,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
		staleTime: 3 * 60 * 1000, // 3mins
		autoRefetch: true,
	})

	function useFetchGitHubBranchesByRepositoryName(
		repoNameRef: MaybeRefOrGetter<string | null | undefined>,
	) {
		return useQuery<string[]>({
			key: () => GITHUB_QUERY_KEYS.listBranches(toValue(repoNameRef) || ""),
			query: async () => {
				if (!(await fetchGitHubConnectionStatus.refresh()).data?.connected) {
					return []
				}

				const repoName = toValue(repoNameRef)
				if (!repoName) {
					return []
				}

				return await $coreAPIClient<string[]>(
					`/api/github/repositories/${repoName}/branches`,
					{ method: "GET" },
				)
			},
			enabled: () =>
				toValue(repoNameRef) !== null && toValue(repoNameRef) !== undefined,
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 3 * 60 * 1000, // 3 mins
			autoRefetch: true,
		})
	}

	function useFetchGitHubFileTreeByRepositoryNameAndBranch(
		repoNameRef: MaybeRefOrGetter<string | null | undefined>,
		branchNameRef: MaybeRefOrGetter<string | null | undefined>,
	) {
		return useQuery<GitHubFileTreeItem[]>({
			key: () =>
				GITHUB_QUERY_KEYS.listFileTreeItems(
					toValue(repoNameRef) || "",
					toValue(branchNameRef) || "",
				),
			query: async () => {
				if (!(await fetchGitHubConnectionStatus.refresh()).data?.connected) {
					return []
				}

				const repoName = toValue(repoNameRef)
				if (!repoName) {
					return []
				}

				const branchName = toValue(branchNameRef)

				let url = `/api/github/repositories/${repoName}/tree`
				if (branchName) {
					url += `?branch=${branchName}`
				}

				return await $coreAPIClient<GitHubFileTreeItem[]>(url, {
					method: "GET",
				})
			},
			enabled: () =>
				toValue(repoNameRef) !== null && toValue(repoNameRef) !== undefined,
			refetchOnMount: false,
			refetchOnWindowFocus: false,
			refetchOnReconnect: false,
			staleTime: 3 * 60 * 1000, // 3 mins
			autoRefetch: true,
		})
	}

	return {
		fetchGitHubConnectionStatus,
		gitHubConfigured,
		fetchGitHubInstallURL,
		connectGitHub,
		disconnectGitHub,
		fetchGitHubRepositories,
		useFetchGitHubBranchesByRepositoryName,
		useFetchGitHubFileTreeByRepositoryNameAndBranch,
	}
}
