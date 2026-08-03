import type { VariantProps } from "class-variance-authority"
import { cva } from "class-variance-authority"
import { cn } from "~/lib/utils"

export { default as Button } from "./Button.vue"

export const buttonVariants = cva(
	"inline-flex items-center cursor-pointer justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-all disabled:cursor-not-allowed disabled:opacity-50 dark:disabled:opacity-50 [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 shrink-0 [&_svg]:shrink-0 outline-none aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 aria-invalid:border-destructive",
	{
		variants: {
			variant: {
				default:
					"bg-primary text-primary-foreground shadow-xs [&:not(:disabled):hover:not(:active)]:bg-primary/80 [&:not(:disabled):active]:bg-primary/60",
				destructive:
					"bg-destructive text-white shadow-xs [&:not(:disabled):hover:not(:active)]:bg-destructive/60 [&:not(:disabled):active]:bg-destructive/80",
				outline:
					"border bg-background shadow-xs [&:not(:disabled):hover:not(:active)]:bg-accent/70 [&:not(:disabled):hover:not(:active)]:text-accent-foreground [&:not(:disabled):active]:bg-accent [&:not(:disabled):active]:text-accent-foreground dark:bg-input/30 dark:border-input dark:[&:not(:disabled):hover:not(:active)]:bg-input/50",
				"outline-no-effect": "border bg-background shadow-xs",
				"outline-transparent": cn(
					"border data-[status=active]:bg-transparent data-[status=active]:text-accent-foreground data-[status=active]:[&:not(:disabled):hover:not(:active)]:bg-transparent data-[status=active]:[&:not(:disabled):active]:bg-transparent",
					"[&:not(:disabled):hover:not(:active)]:bg-transparent [&:not(:disabled):hover:not(:active)]:text-accent-foreground/70 [&:not(:disabled):hover:not(:active)]:*:opacity-50 focus:bg-transparent focus:text-accent-foreground/90 dark:[&:not(:disabled):hover:not(:active)]:bg-transparent dark:focus:bg-transparent",
					"[&:not(:disabled):active]:*:opacity-70",
					"*:transition-opacity *:duration-150",
				),
				"outline-dim": cn(
					"border data-[status=active]:bg-transparent data-[status=active]:text-accent-foreground/70 data-[status=active]:[&:not(:disabled):hover:not(:active)]:bg-transparent data-[status=active]:[&:not(:disabled):active]:bg-transparent",
					"text-accent-foreground/30 bg-transparent [&:not(:disabled):hover:not(:active)]:bg-transparent [&:not(:disabled):hover:not(:active)]:text-accent-foreground/40 focus:bg-transparent focus:text-accent-foreground/50 dark:[&:not(:disabled):hover:not(:active)]:bg-transparent dark:focus:bg-transparent",
				),
				secondary:
					"bg-secondary text-secondary-foreground shadow-xs [&:not(:disabled):hover:not(:active)]:bg-secondary/80 [&:not(:disabled):hover:not(:active)]:text-secondary-foreground/80 [&:not(:disabled):active]:bg-secondary/60 [&:not(:disabled):active]:text-secondary-foreground/60",
				ghost: cn(
					"data-[status=active]:bg-accent/50 data-[status=active]:text-accent-foreground data-[status=active]:[&:not(:disabled):hover:not(:active)]:bg-accent/70 data-[status=active]:[&:not(:disabled):active]:bg-accent ",
					"[&:not(:disabled):hover:not(:active)]:bg-accent/70 [&:not(:disabled):hover:not(:active)]:text-accent-foreground [&:not(:disabled):active]:bg-accent [&:not(:disabled):active]:text-accent-foreground dark:[&:not(:disabled):hover:not(:active)]:bg-accent/60 dark:[&:not(:disabled):active]:bg-accent",
				),
				"ghost-plain": cn(
					"data-[status=active]:bg-transparent data-[status=active]:text-accent-foreground data-[status=active]:[&:not(:disabled):hover:not(:active)]:bg-transparent data-[status=active]:[&:not(:disabled):active]:bg-transparent",
					"[&:not(:disabled):hover:not(:active)]:bg-transparent [&:not(:disabled):hover:not(:active)]:text-accent-foreground/70 focus:bg-transparent focus:text-accent-foreground/90 dark:[&:not(:disabled):hover:not(:active)]:bg-transparent dark:focus:bg-transparent",
				),
				"ghost-plain-destructive": cn(
					"data-[status=active]:bg-transparent data-[status=active]:text-destructive-foreground data-[status=active]:[&:not(:disabled):hover:not(:active)]:bg-transparent data-[status=active]:[&:not(:disabled):active]:bg-transparent",
					"text-destructive-foreground [&:not(:disabled):hover:not(:active)]:bg-transparent [&:not(:disabled):hover:not(:active)]:text-destructive-foreground/60 focus:bg-transparent focus:text-destructive-foreground/80 dark:[&:not(:disabled):hover:not(:active)]:bg-transparent dark:focus:bg-transparent",
				),
				dim: cn(
					"data-[status=active]:bg-transparent data-[status=active]:text-accent-foreground/70 data-[status=active]:[&:not(:disabled):hover:not(:active)]:bg-transparent data-[status=active]:[&:not(:disabled):active]:bg-transparent",
					"text-accent-foreground/30 bg-transparent [&:not(:disabled):hover:not(:active)]:bg-transparent [&:not(:disabled):hover:not(:active)]:text-accent-foreground/40 focus:bg-transparent focus:text-accent-foreground/50 dark:[&:not(:disabled):hover:not(:active)]:bg-transparent dark:focus:bg-transparent",
				),
				link: "text-primary underline-offset-4 [&:not(:disabled):hover:not(:active)]:underline",
			},
			size: {
				default: "h-9 px-4 py-2 has-[>svg]:px-3",
				"2sm": "h-7 rounded-md gap-1.5 px-3 has-[>svg]:px-2.5 text-2sm",
				sm: "h-8 rounded-md gap-1.5 px-3 has-[>svg]:px-2.5",
				lg: "h-10 rounded-md px-6 has-[>svg]:px-4",
				icon: "size-9", // default icon
				"icon-sm": "size-6.5 rounded-md",
				"icon-xsm": "size-5",
				"icon-2xsm": "size-3.75",
				custom: "",
			},
		},
		defaultVariants: {
			variant: "default",
			size: "default",
		},
	},
)

export type ButtonVariants = VariantProps<typeof buttonVariants>
