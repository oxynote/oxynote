import { vi, type Mock } from "vitest"
import type * as Y from "yjs"
import type { CoreClient } from "./core.js"
import type { Store } from "./db.js"
import type { Env } from "./env.js"

export type StubStore = {
	[K in keyof Store]: Mock<Store[K]>
}

// the store as its callers see it: four questions with answers. Defaults
// describe a user who belongs to an organization and a client whose
// consent still stands, so a test states only the condition it is about.
export function stubStore(): StubStore {
	return {
		totalOrganizationCount: vi.fn().mockResolvedValue(0),
		userOrganizationId: vi.fn().mockResolvedValue("org-1"),
		hasOAuthConsent: vi.fn().mockResolvedValue(true),
		isOrganizationMember: vi.fn().mockResolvedValue(true),
	}
}

// the serialized XML of one of a document's fragments. toJSON is the
// string-typed reader; toString comes off yjs's untyped base class.
export function fragmentXml(doc: Y.Doc, name: string): string {
	return doc.getXmlFragment(name).toJSON()
}

export type StubCore = {
	[K in keyof CoreClient]: Mock<CoreClient[K]>
}

// a fully wired core client whose every call resolves. A test that
// cares about one call overrides it and reads the others' call counts,
// which is what makes "nothing else was persisted" assertable.
export function stubCore(): StubCore {
	return {
		sendEmail: vi.fn().mockResolvedValue(undefined),
		initializeOrganization: vi.fn().mockResolvedValue(undefined),
		teardownOrganization: vi.fn().mockResolvedValue(undefined),
		fetchBranches: vi.fn().mockResolvedValue([]),
		fetchBranchContent: vi.fn().mockResolvedValue({
			documentName: "Doc",
			content: { type: "doc", content: [] },
			icon: "lucide:file",
			rawContent: null,
		}),
		storeBranchContent: vi.fn().mockResolvedValue(undefined),
		verifyDocumentAccess: vi.fn().mockResolvedValue(undefined),
		mergeBranches: vi.fn().mockResolvedValue({
			status: 200,
			data: {
				documentName: "Doc",
				content: { type: "doc", content: [] },
				icon: "lucide:file",
			},
		}),
	}
}

// a complete, valid configuration. Tests override only the field under
// test so an unrelated env change cannot quietly alter what they assert.
export function testEnv(overrides: Partial<Env> = {}): Env {
	return {
		coreUrl: "http://core:8080",
		databaseDSN: "postgresql://devuser:devpass@postgres/devdb",
		valkeyUrl: "redis://valkey:6379",
		publicAuthBaseUrl: "http://localhost:8080/auth-realtime",
		authOrigin: "http://localhost:8080",
		betterAuthSecret: "sup3rs3cr3t",
		cookieDomain: "localhost",
		frontendUrl: "http://localhost:8080",
		organizationInvitationUrl:
			"http://localhost:8080/accept-invite",
		trustedOrigins: ["http://localhost:8080"],
		mcpResource: "http://localhost:8080/core/api/mcp",
		mcpTokenIssuer: "http://localhost:8080/api/auth",
		socialProviders: {},
		maxOrganizations: 100,
		maxOrganizationMembers: 5,
		rateLimitEnabled: true,
		...overrides,
	}
}
