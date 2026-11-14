import {expect, test} from "@playwright/test"
import { BASE_URL } from "../helpers/config";
import { visit } from "../helpers/page";

test.describe('Theme - Auto mode', () => {



    test.describe('When the system is in light mode', () =>{

        test.use({colorScheme: 'light'});

        test('Renders light mode', async({context, page}) =>{

            await context.addCookies([
                {
                    name: "color-mode",
                    value: "auto",
                    url: BASE_URL
                },
                {
                    name:"last-auto-resolved-color-mode",
                    value: "dark",
                    url: BASE_URL
                }
            ]);

            await visit(page,"/");

            await expect(page.locator("html")).not.toHaveClass(/\bdark\b/);
        });
    });

    test.describe('When the system is in dark mode', () =>{

        test.use({colorScheme: 'dark'});

        test('Renders dark mode', async({context, page}) =>{

            await context.addCookies([
                {
                    name: "color-mode",
                    value: "auto",
                    url: BASE_URL
                },
                {
                    name:"last-auto-resolved-color-mode",
                    value: "light",
                    url: BASE_URL
                }
            ]);

            await visit(page,"/");

            await expect(page.locator("html")).toHaveClass(/\bdark\b/);
        });

        test('Updates when the system changes to light', async({context, page}) =>{
            await context.addCookies([
                {
                    name: "color-mode",
                    value: "auto",
                    url: BASE_URL
                },
                {
                    name:"last-auto-resolved-color-mode",
                    value: "dark",
                    url: BASE_URL
                }
            ]);

            await visit(page,"/");
            await expect(page.locator("html")).toHaveClass(/\bdark\b/);

            await page.emulateMedia({ colorScheme: "light" });

            await expect(page.locator("html")).not.toHaveClass(/\bdark\b/);
        });
    });



})