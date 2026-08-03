/** @type {import('prettier').Config & import('prettier-plugin-tailwindcss').PluginOptions} */
export default {
	plugins: ["prettier-plugin-tailwindcss"],
	tailwindStylesheet: "./app/assets/css/main.css",
	trailingComma: "all",
	useTabs: true,
	tabWidth: 2,
	semi: false,
	htmlWhitespaceSensitivity: "ignore",
}
