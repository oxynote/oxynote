export interface GitHubRepository {
	name: string
	defaultBranch: string
}

export interface GitHubConnectionStatus {
	connected: boolean
}

export interface GitHubInstallResponse {
	url: string
}

export interface GitHubFileTreeItem {
	type: "file" | "folder"
	name: string
	items: GitHubFileTreeItem[] | null | undefined
	checksum: string
}
