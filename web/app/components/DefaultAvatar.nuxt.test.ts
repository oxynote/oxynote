import { mountSuspended } from "@nuxt/test-utils/runtime"
import { describe, it } from "vitest"
import DefaultAvatar from "./DefaultAvatar.vue"
import { t } from "./test-helpers"

describe("<DefaultAvatar>", () => {
	it("shows the default picture of its seed", async ({ expect }) => {
		const wrapper = await mountSuspended(DefaultAvatar, {
			props: { kind: "user", seed: "ada", name: "Ada" },
		})

		expect(wrapper.get("img").attributes("src")).toBe(
			defaultAvatar("user", "ada"),
		)
	})

	it("follows a change of seed", async ({ expect }) => {
		const wrapper = await mountSuspended(DefaultAvatar, {
			props: { kind: "user", seed: "ada", name: "Ada" },
		})

		await wrapper.setProps({ seed: "grace" })

		expect(wrapper.get("img").attributes("src")).toBe(
			defaultAvatar("user", "grace"),
		)
	})

	it.for([
		{
			name: "names the user it stands for",
			input: { kind: "user", name: "Ada" },
			expected: () => t("general.default-avatar.user", { name: "Ada" }),
		},
		{
			name: "describes a user it has no name for",
			input: { kind: "user" },
			expected: () => t("general.default-avatar.user-unnamed"),
		},
		{
			name: "names the workspace it stands for",
			input: { kind: "organization", name: "Acme" },
			expected: () =>
				t("general.default-avatar.organization", { name: "Acme" }),
		},
		{
			name: "describes a workspace it has no name for",
			input: { kind: "organization" },
			expected: () => t("general.default-avatar.organization-unnamed"),
		},
		{
			name: "says a deleted user's picture is one",
			input: { kind: "deleted-user", name: "Ada" },
			expected: () => t("general.default-avatar.deleted-user"),
		},
		{
			name: "says an invited user's picture is one",
			input: { kind: "invited-user", name: "Ada" },
			expected: () => t("general.default-avatar.invited-user"),
		},
		{
			name: "stays silent when the name is written beside it",
			input: { kind: "user", name: "Ada", decorative: true },
			expected: () => "",
		},
	] as const)("$name", async ({ input, expected }, { expect }) => {
		const wrapper = await mountSuspended(DefaultAvatar, {
			props: { seed: "seed-1", ...input },
		})

		expect(wrapper.get("img").attributes("alt")).toBe(expected())
	})
})
