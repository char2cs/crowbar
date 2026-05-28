"use client"

import * as React from "react"
import * as TabsPrimitive from "@radix-ui/react-tabs"

import { cn } from "@/lib/utils"

const Tabs = TabsPrimitive.Root

export interface TabsListProps extends React.ComponentPropsWithoutRef<typeof TabsPrimitive.List> {
  /** Visual variant (Athas compatibility) */
  variant?: string
}

const TabsList = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.List>,
  TabsListProps
>(({ className, variant: _variant, ...props }, ref) => (
  <TabsPrimitive.List
    ref={ref}
    className={cn(
      "inline-flex h-10 items-center justify-center rounded-md bg-muted p-1 text-muted-foreground",
      className
    )}
    {...props}
  />
))
TabsList.displayName = TabsPrimitive.List.displayName

export interface TabsTriggerProps extends React.ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger> {
  /** Whether this tab is active (Athas compatibility) */
  isActive?: boolean
  /** Whether this tab is being dragged (Athas compatibility) */
  isDragged?: boolean
  /** Action element rendered inside the tab (Athas compatibility) */
  action?: React.ReactNode
  /** Visual variant (Athas compatibility) */
  variant?: string
  /** Size variant (Athas compatibility) */
  size?: "xs" | "sm" | "md" | "lg"
}

const TabsTrigger = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Trigger>,
  TabsTriggerProps
>(({ className, isActive: _isActive, isDragged: _isDragged, action: _action, variant: _variant, size: _size, ...props }, ref) => (
  <TabsPrimitive.Trigger
    ref={ref}
    className={cn(
      "inline-flex items-center justify-center whitespace-nowrap rounded-sm px-3 py-1.5 text-sm font-medium ring-offset-background transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-background data-[state=active]:text-foreground data-[state=active]:shadow-sm",
      className
    )}
    {...props}
  />
))
TabsTrigger.displayName = TabsPrimitive.Trigger.displayName

const TabsContent = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.Content>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.Content
    ref={ref}
    className={cn(
      "mt-2 ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
      className
    )}
    {...props}
  />
))
TabsContent.displayName = TabsPrimitive.Content.displayName

/** Standalone tab button used by Athas feature modules (does not require Radix value prop) */
export interface TabProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  isActive?: boolean
  isDragged?: boolean
  action?: React.ReactNode
  variant?: string
  size?: "xs" | "sm" | "md" | "lg"
  labelPosition?: "start" | "center" | "end"
  maxWidth?: number
}

const Tab = React.forwardRef<HTMLButtonElement, TabProps>(
  (
    {
      className,
      isActive,
      isDragged: _isDragged,
      action,
      variant: _variant,
      size: _size,
      labelPosition: _labelPosition,
      maxWidth: _maxWidth,
      children,
      ...props
    },
    ref,
  ) => (
    <button
      ref={ref}
      className={cn(
        "relative inline-flex shrink-0 cursor-pointer items-center whitespace-nowrap rounded-lg border font-medium text-sm outline-none transition-colors",
        "focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
        "disabled:pointer-events-none disabled:opacity-64",
        isActive
          ? "border-border bg-background text-foreground"
          : "border-transparent text-muted-foreground hover:bg-accent hover:text-foreground",
        className,
      )}
      {...props}
    >
      {children}
      {action}
    </button>
  ),
)
Tab.displayName = "Tab"

/** Athas tab item descriptor */
export interface TabsItem {
  id: string
  icon?: React.ReactNode
  label?: string
  isActive?: boolean
  onClick?: () => void
  role?: string
  ariaLabel?: string
  className?: string
  tabIndex?: number
  title?: string
  tooltip?: {
    content: string
    shortcut?: string
    side?: "top" | "right" | "bottom" | "left"
    className?: string
  }
}

export { Tabs, TabsList, TabsTrigger, TabsContent, Tab }
