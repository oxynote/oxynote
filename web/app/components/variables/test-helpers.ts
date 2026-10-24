// shared fixtures for the variables panel suites. Test-only: nothing here
// is imported by app code, and the app/**/test-helpers.ts coverage
// exclude keeps it out of the denominator.
import type {
	VariableGroupTreeElement,
	VariableTreeElement,
	VariableTreeResponse,
} from "~/utils"
import {
	makeXid,
	mockEndpoint,
	runInApp,
	seedQueryData,
} from "~/composables/api/test-helpers"
import { useGroupOpenState } from "./group-state"

export const TREE_URL = "/api/variables/tree"

export const GROUP_METRICS = makeXid("grpmetrics")
export const GROUP_ENV = makeXid("grpenv")
export const GROUP_DEPLOYS = makeXid("grpdeploys")
export const VAR_RANGE = makeXid("varrange")
export const VAR_REFRESH = makeXid("varrefresh")
export const VAR_ENV = makeXid("varenv")
export const VAR_REGION = makeXid("varregion")
const OPTION_5M = makeXid("opt5m")
export const OPTION_1M = makeXid("opt1m")
export const OPTION_PROD = makeXid("optprod")
export const OPTION_STAGING = makeXid("optstaging")
export const OPTION_EU = makeXid("opteu")
export const OPTION_US = makeXid("optus")

// the built-in group every organization starts with
export function metricsGroup(): VariableGroupTreeElement {
	return makeGroup(
		{
			id: GROUP_METRICS,
			groupName: "Metrics",
			color: "blue",
			builtin: true,
		},
		[
			makeVariable({
				id: VAR_RANGE,
				groupId: GROUP_METRICS,
				key: "metric_range",
				name: "Metric time range",
				type: "time-range",
				defaultValue: "last_24_hours",
				builtin: true,
			}),
			makeVariable({
				id: VAR_REFRESH,
				groupId: GROUP_METRICS,
				key: "metric_refresh",
				name: "Metric refresh interval",
				type: "list",
				defaultValue: OPTION_5M,
				builtin: true,
				options: [
					{ id: OPTION_5M, variableId: VAR_REFRESH, value: "5m", sortIndex: 0 },
					{ id: OPTION_1M, variableId: VAR_REFRESH, value: "1m", sortIndex: 1 },
				],
			}),
		],
	)
}

export function environmentGroup(): VariableGroupTreeElement {
	return makeGroup(
		{ id: GROUP_ENV, groupName: "Environment", color: "green" },
		[
			makeVariable({
				id: VAR_ENV,
				groupId: GROUP_ENV,
				key: "env",
				name: "Environment",
				type: "list",
				defaultValue: OPTION_PROD,
				options: [
					{
						id: OPTION_PROD,
						variableId: VAR_ENV,
						value: "production",
						sortIndex: 0,
					},
					{
						id: OPTION_STAGING,
						variableId: VAR_ENV,
						value: "staging",
						sortIndex: 1,
					},
				],
			}),
			makeVariable({
				id: VAR_REGION,
				groupId: GROUP_ENV,
				key: "region",
				name: "Region",
				type: "list",
				defaultValue: OPTION_EU,
				value: OPTION_US,
				options: [
					{
						id: OPTION_EU,
						variableId: VAR_REGION,
						value: "eu-west-1",
						sortIndex: 0,
					},
					{
						id: OPTION_US,
						variableId: VAR_REGION,
						value: "us-east-1",
						sortIndex: 1,
					},
				],
			}),
		],
	)
}

// a user group that holds nothing yet
function deploysGroup(): VariableGroupTreeElement {
	return makeGroup({
		id: GROUP_DEPLOYS,
		groupName: "Deploys",
		color: "orange",
		sortIndex: 1,
	})
}

export function fullTree(): VariableTreeResponse {
	return [metricsGroup(), environmentGroup(), deploysGroup()]
}

export function builtinTree(): VariableTreeResponse {
	return [metricsGroup()]
}

export function makeGroup(
	group: Partial<VariableGroupTreeElement>,
	variables: VariableTreeElement[] = [],
): VariableGroupTreeElement {
	return {
		id: makeXid("grp"),
		organizationId: "org-1",
		groupName: "Group",
		color: "green",
		sortIndex: 0,
		builtin: false,
		createdAt: "2026-10-01T00:00:00Z",
		createdBy: null,
		...group,
		variables: variables,
	}
}

export function makeVariable(
	variable: Partial<VariableTreeElement>,
): VariableTreeElement {
	return {
		id: makeXid("var"),
		organizationId: "org-1",
		groupId: GROUP_ENV,
		key: "key",
		name: "Variable",
		description: null,
		type: "text",
		defaultValue: null,
		sortIndex: 0,
		builtin: false,
		createdAt: "2026-10-01T00:00:00Z",
		createdBy: null,
		options: [],
		value: null,
		...variable,
	}
}

// every mutation edits the cached tree before its request goes out and
// refetches it on success, so a suite that sends one needs both
export function mockTree(tree: VariableTreeResponse) {
	seedQueryData(["variables", "tree"], tree)

	return mockEndpoint("GET", TREE_URL, () => tree)
}

// the open state lives for a page load, which every test in the app shares.
// Opening every group lets a suite read all of their rows.
export function openAllGroups() {
	setGroupState([GROUP_METRICS, GROUP_ENV, GROUP_DEPLOYS])
}

// forgets every choice and every created group, as a fresh page load does
export function resetGroupState() {
	setGroupState([])
}

function setGroupState(created: string[]) {
	runInApp(() => {
		const state = useGroupOpenState()
		state.chosen.value = {}
		state.created.value = created
	})
}
