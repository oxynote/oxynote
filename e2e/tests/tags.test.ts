import { expect, test } from "@playwright/test"
import { waitForEditor } from "../helpers/editor"
import { t } from "../helpers/i18n"
import { visit } from "../helpers/page"
import {
	makeReviewable,
	mergeDraftIntoMain,
	switchToBranch,
} from "../helpers/review"
import {
	createTagFromHeader,
	expandTag,
	headerTagPill,
	openTagActions,
	openTagVisibility,
	sidebarTag,
	taggedDocument,
} from "../helpers/tags"
import { signUpWithWorkspace } from "../helpers/workspace"

// the name core gives the first page of every workspace
const WELCOME_PAGE = "Welcome to Oxynote!"

test.describe("tags", () => {
	// the flow layers a reload and the review workflow's fork on top of the
	// usual signup-and-workspace setup
	test.beforeEach(() => {
		test.slow()
	})

	test("labels a page's branches and keeps the labels across a reload", async ({
		page,
		request,
	}) => {
		await signUpWithWorkspace(page, request)

		// a fresh workspace comes with three tags, and its welcome page
		// under the first of them
		await expect(sidebarTag(page, "Production")).toBeVisible()
		await expect(sidebarTag(page, "Staging")).toBeVisible()
		await expect(sidebarTag(page, "Incidents")).toBeVisible()
		await expect(headerTagPill(page, "Production")).toBeVisible()
		await expandTag(page, "Production")
		await expect(taggedDocument(page, "Production", WELCOME_PAGE)).toBeVisible()

		await createTagFromHeader(page, "Release")

		await expandTag(page, "Release")
		await expect(taggedDocument(page, "Release", WELCOME_PAGE)).toBeVisible()

		// the tag and its assignment are stored, not held by the page
		await visit(page, page.url())
		await waitForEditor(page)

		await expect(headerTagPill(page, "Release")).toBeVisible()
		await expandTag(page, "Release")
		await expect(taggedDocument(page, "Release", WELCOME_PAGE)).toBeVisible()

		// the draft the review workflow forks off main takes main's tags
		// with it, and main takes the draft's back when the draft is merged
		await makeReviewable(page)
		await switchToBranch(page, "draft")

		await expect(headerTagPill(page, "Release")).toBeVisible()

		await createTagFromHeader(page, "Hotfix")
		await mergeDraftIntoMain(page)

		await expect(headerTagPill(page, "Hotfix")).toBeVisible()
		await expandTag(page, "Hotfix")
		await expect(taggedDocument(page, "Hotfix", WELCOME_PAGE)).toBeVisible()

		// hiding is the caller's own view of the sidebar: the page keeps
		// the tag, and the section's visibility menu still lists it
		await openTagActions(page, "Release")
		await page
			.getByRole("menuitem", {
				name: t("sidebar.item-dropdown-menu-buttons.hide-tag"),
			})
			.click()

		await expect(sidebarTag(page, "Release")).toBeHidden()
		await expect(headerTagPill(page, "Release")).toBeVisible()

		await openTagVisibility(page)
		await page.getByRole("menuitem", { name: "Release" }).click()
		await page.keyboard.press("Escape")

		await expect(sidebarTag(page, "Release")).toBeVisible()

		// deleting takes the tag off every page
		await openTagActions(page, "Release")
		await page
			.getByRole("menuitem", {
				name: t("sidebar.item-dropdown-menu-buttons.delete-tag"),
			})
			.click()

		await expect(sidebarTag(page, "Release")).toBeHidden()
		await expect(headerTagPill(page, "Release")).toBeHidden()
	})
})
