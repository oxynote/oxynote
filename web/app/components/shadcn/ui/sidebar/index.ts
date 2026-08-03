import type { VariantProps } from "class-variance-authority"
import type { HTMLAttributes } from "vue"
import { cva } from "class-variance-authority"
import { cn } from "~/lib/utils"

export interface SidebarProps {
	variant?: "sidebar" | "floating" | "inset"
	class?: HTMLAttributes["class"]
}

export { default as Sidebar } from "./Sidebar.vue"
export { default as SidebarContent } from "./SidebarContent.vue"
export { default as SidebarFooter } from "./SidebarFooter.vue"
export { default as SidebarGroup } from "./SidebarGroup.vue"
export { default as SidebarGroupAction } from "./SidebarGroupAction.vue"
export { default as SidebarGroupContent } from "./SidebarGroupContent.vue"
export { default as SidebarGroupLabel } from "./SidebarGroupLabel.vue"
export { default as SidebarHeader } from "./SidebarHeader.vue"
export { default as SidebarInput } from "./SidebarInput.vue"
export { default as SidebarInset } from "./SidebarInset.vue"
export { default as SidebarMenu } from "./SidebarMenu.vue"
export { default as SidebarMenuAction } from "./SidebarMenuAction.vue"
export { default as SidebarMenuBadge } from "./SidebarMenuBadge.vue"
export { default as SidebarMenuButton } from "./SidebarMenuButton.vue"
export { default as SidebarMenuItem } from "./SidebarMenuItem.vue"
export { default as SidebarMenuSkeleton } from "./SidebarMenuSkeleton.vue"
export { default as SidebarMenuSub } from "./SidebarMenuSub.vue"
export { default as SidebarMenuSubButton } from "./SidebarMenuSubButton.vue"
export { default as SidebarMenuSubItem } from "./SidebarMenuSubItem.vue"
export { default as SidebarProvider } from "./SidebarProvider.vue"
export { default as SidebarSeparator } from "./SidebarSeparator.vue"
export { default as SidebarTrigger } from "./SidebarTrigger.vue"

export { useSidebar, useSidebarWidth, SIDEBAR_DEFAULT_WIDTH } from "./utils"

export const sidebarMenuButtonVariants = cva(
	cn(
		"peer/menu-button flex w-full items-center gap-2 overflow-hidden rounded-md p-2 text-left text-sm outline-hidden ring-sidebar-ring transition-[width,height,padding] focus-visible:ring-2 disabled:pointer-events-none disabled:opacity-50 group-has-data-[sidebar-menu-action-side=left]/menu-item:pl-7 group-has-data-[sidebar-menu-action-side=right]/menu-item:pr-8 aria-disabled:pointer-events-none aria-disabled:opacity-50 group-data-[collapsible=icon]:size-8! group-data-[collapsible=icon]:p-2! [&>span:last-child]:truncate [&>svg]:size-4 [&>svg]:shrink-0",
		"hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground data-[state=open]:hover:bg-sidebar-accent data-[state=open]:hover:text-sidebar-accent-foreground",
		"active:bg-sidebar-accent active:text-sidebar-accent-foreground data-[active=true]:bg-sidebar-accent/50 data-[active=true]:active:bg-sidebar-accent data-[active=true]:text-sidebar-accent-foreground",
	),
	{
		variants: {
			variant: {
				default:
					"hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground",
				outline:
					"bg-background shadow-[0_0_0_1px_hsl(var(--sidebar-border))] hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground hover:shadow-[0_0_0_1px_hsl(var(--sidebar-accent))]",
				placeholder: cn(
					"transition-colors duration-150 text-sidebar-foreground/60 hover:text-sidebar-foreground/40 data-[state=open]:hover:text-sidebar-foreground/40 hover:bg-transparent data-[state=open]:hover:bg-transparent group-has-data-[sidebar=menu-action]/menu-item:p-0",
					"active:text-sidebar-foreground/40 data-[active=true]:text-sidebar-foreground/40 active:bg-transparent data-[active=true]:bg-transparent",
				),
			},
			size: {
				default: "h-8 text-sm",
				sm: "h-7 text-xs",
				md: "h-7 text-2sm",
				lg: "h-12 text-sm group-data-[collapsible=icon]:p-0!",
			},
		},
		defaultVariants: {
			variant: "default",
			size: "default",
		},
	},
)

export type SidebarMenuButtonVariants = VariantProps<
	typeof sidebarMenuButtonVariants
>
