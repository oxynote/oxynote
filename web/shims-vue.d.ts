// typescript-eslint cannot resolve .vue files, so it typed every component
// import as an error. vue-tsc resolves the real file first and never
// reaches this fallback.
declare module "*.vue" {
	import type { DefineComponent } from "vue"

	const component: DefineComponent
	export default component
}
