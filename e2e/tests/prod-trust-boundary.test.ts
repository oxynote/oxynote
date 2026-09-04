import { expect, test } from "@playwright/test"
import {
	FRONT_DOOR,
	INTERNAL_SERVICES,
	probeFromNetwork,
	rawRequest,
} from "../helpers/prod"

// the production image's whole security model is that caddy is the only
// thing anyone can talk to. Core's /api/x/* and auth-realtime's
// /api/internal/* carry no authentication at all — they trust the caller by
// construction — so two independent things have to hold, and these tests
// assert them separately because either one alone is a single point of
// failure:
//
//   1. the services bind the container's loopback, so nothing on the
//      network can address them however it spells the request; and
//   2. caddy blocks the internal prefixes at the front door, so the one
//      reachable listener refuses to proxy them.
//
// They only mean anything against the all-in-one image — the dev stack puts
// each service in its own container on a shared network by design — so they
// live here rather than beside the flow tests.
//
// A file the tests use as their vocabulary: the endpoints below are the real
// internal surface, taken from core's internalRouter and auth-realtime's
// /internal routes. Reaching any of them without a session means reading or
// writing any document in any workspace, tearing an organization down, or
// sending mail as the deployment.

// the raw path of an /api/x endpoint, as it would arrive at the front door.
const CORE_INTERNAL_ENDPOINTS = [
	{ name: "the version report", method: "GET", path: "/core/api/x/version" },
	{ name: "the metrics scrape", method: "GET", path: "/core/api/x/metrics" },
	{
		name: "the pprof index",
		method: "GET",
		path: "/core/api/x/debug/pprof/",
	},
	{
		name: "a pprof heap dump",
		method: "GET",
		path: "/core/api/x/debug/pprof/heap",
	},
	{
		name: "organization initialization",
		method: "POST",
		path: "/core/api/x/organizations/org-e2e/initialize",
	},
	{
		name: "organization teardown",
		method: "POST",
		path: "/core/api/x/organizations/org-e2e/teardown",
	},
	{
		name: "an unauthorized branch listing",
		method: "GET",
		path: "/core/api/x/documents/doc-e2e/branches",
	},
	{
		name: "an unauthorized branch read",
		method: "GET",
		path: "/core/api/x/documents/doc-e2e/branch/branch-e2e/",
	},
	{
		name: "an unauthorized branch write",
		method: "PUT",
		path: "/core/api/x/documents/doc-e2e/branch/branch-e2e/",
	},
	{ name: "outgoing email", method: "POST", path: "/core/api/x/email" },
	// session-authed, but internal all the same: auth-realtime stores the
	// editors' pending changes before core reads the branch, and a client
	// reaching these directly would skip that.
	{
		name: "a branch merge",
		method: "PUT",
		path: "/core/api/x/documents/doc-e2e/merge",
	},
	{
		name: "a branch fork",
		method: "POST",
		path: "/core/api/x/documents/doc-e2e/branches",
	},
	{
		name: "a branch protection change",
		method: "PUT",
		path: "/core/api/x/documents/doc-e2e/branches/branch-e2e",
	},
] as const

const AUTH_REALTIME_INTERNAL_ENDPOINTS = [
	{
		name: "MCP session validation",
		method: "GET",
		path: "/auth-realtime/api/internal/mcp/session",
	},
	{
		name: "direct document operations",
		method: "POST",
		path: "/auth-realtime/api/internal/documents/doc-e2e/branches/branch-e2e/operations",
	},
] as const

// spellings of a blocked path that a proxy is expected to still recognise.
// Caddy matches on the unescaped path and does so case-insensitively, and
// it merges duplicate slashes, so each of these is the same request as
// /core/api/x/version and has to be refused as one.
const EQUIVALENT_SPELLINGS = [
	{ name: "an uppercase segment", path: "/core/api/X/version" },
	{ name: "an uppercase prefix", path: "/CORE/API/X/version" },
	{ name: "a percent-encoded segment", path: "/core/api/%78/version" },
	{ name: "a percent-encoded separator", path: "/core/api/x%2fversion" },
	{ name: "a doubled separator", path: "/core//api/x/version" },
	{ name: "a doubled root", path: "//core/api/x/version" },
	{ name: "a current-directory segment", path: "/core/./api/x/version" },
	{ name: "a query string", path: "/core/api/x/version?probe=1" },
] as const

// spellings that try to arrive at the internal surface by a route the block
// does not literally match. Where caddy normalises them back they are
// refused, and where it does not they land somewhere harmless — what must
// never happen is that one of them succeeds.
const TRAVERSAL_SPELLINGS = [
	{ name: "a parent segment", path: "/core/api/x/../x/version" },
	{ name: "a parent segment from a sibling", path: "/core/api/y/../x/version" },
	{ name: "an encoded parent segment", path: "/core/api/%2e%2e/api/x/version" },
	{ name: "a path parameter", path: "/core/api/x/version;/" },
	{ name: "a trailing dot", path: "/core/api/x/version/." },
	{
		name: "a parent segment into auth-realtime",
		path: "/auth-realtime/api/foo/../internal/mcp/session",
	},
	{
		name: "an encoded parent segment into auth-realtime",
		path: "/auth-realtime/api/%2e%2e/api/internal/mcp/session",
	},
	// the block matches the prefix with a separator after it, so the bare
	// prefix is not caught by it — nothing useful is served there, and this
	// is the case that says so out loud.
	{ name: "the bare core prefix", path: "/core/api/x" },
	{
		name: "the bare auth-realtime prefix",
		path: "/auth-realtime/api/internal",
	},
] as const

test.describe("internal service binds", () => {
	for (const service of INTERNAL_SERVICES) {
		test(`refuses ${service.name} to another container on the network`, async () => {
			const result = await probeFromNetwork(service.port, service.path)

			expect(result.reached, result.detail).toBe(false)
		})
	}

	// without this the tests above pass for the wrong reason — a probe
	// container that cannot resolve the image at all, or a stack that never
	// came up, refuses every port just as convincingly as a loopback bind.
	test("reaches the front door from the same container", async () => {
		const result = await probeFromNetwork(FRONT_DOOR.port, FRONT_DOOR.path)

		expect(result.reached, result.detail).toBe(true)
	})
})

test.describe("front door blocks", () => {
	for (const endpoint of [
		...CORE_INTERNAL_ENDPOINTS,
		...AUTH_REALTIME_INTERNAL_ENDPOINTS,
	]) {
		test(`refuses ${endpoint.name}`, async () => {
			const response = await rawRequest(endpoint.path, endpoint.method)

			expect(response.status).toBe(403)
		})
	}

	for (const spelling of EQUIVALENT_SPELLINGS) {
		test(`refuses a blocked path written with ${spelling.name}`, async () => {
			const response = await rawRequest(spelling.path)

			expect(response.status).toBe(403)
		})
	}

	for (const spelling of TRAVERSAL_SPELLINGS) {
		test(`never serves a blocked path reached through ${spelling.name}`, async () => {
			const response = await rawRequest(spelling.path)

			expect(
				response.status,
				`${spelling.path} answered ${String(response.status)}`,
			).toBeGreaterThanOrEqual(300)
			// core reports its build on /api/x/version, which is the one
			// endpoint here that would say so in its body if a bypass had
			// worked and the status alone had not given it away.
			expect(response.body).not.toContain("version")
		})
	}
})

test.describe("front door routing", () => {
	// the blocks above are only evidence if the same front door serves
	// everything else. A stack that answered 403 to every request would
	// pass every test in the group above and none of these.
	test("serves the app", async () => {
		const response = await rawRequest("/login")

		expect(response.status).toBe(200)
	})

	test("reaches core and leaves its session check in place", async () => {
		const response = await rawRequest("/core/api/capabilities")

		expect(response.status).toBe(401)
	})

	test("reaches auth-realtime", async () => {
		const response = await rawRequest("/auth-realtime/api/auth-config")

		expect(response.status).toBe(200)
	})

	// /api/apps/* is the deliberate opposite of /api/x/*: GitHub and Slack
	// deliver webhooks and OAuth callbacks there with no session, proving
	// themselves with a request signature instead. It answers 404 here only
	// because no GitHub App is configured for the stack — what matters is
	// that the internal block does not swallow it.
	test("leaves the public app callbacks open", async () => {
		const response = await rawRequest("/core/api/apps/github/events", "POST")

		expect(response.status).not.toBe(403)
	})

	test("does not name the server it runs", async () => {
		const response = await rawRequest("/login")

		expect(response.headers.server).toBeUndefined()
	})
})
