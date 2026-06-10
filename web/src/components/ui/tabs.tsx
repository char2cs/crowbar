'use client'

import { Tabs as TabsPrimitive } from '@base-ui/react/tabs'
import * as React from 'react'
import { cn } from '@/lib/utils'

export type TabsVariant = 'default' | 'underline' | 'segmented'

export function Tabs({ className, ...props }: TabsPrimitive.Root.Props): React.ReactElement {
  return (
    <TabsPrimitive.Root
      className={cn('flex flex-col gap-2 data-[orientation=vertical]:flex-row', className)}
      data-slot="tabs"
      {...props}
    />
  )
}

export function TabsList({
  variant = 'default',
  className,
  children,
  ...props
}: TabsPrimitive.List.Props & {
  variant?: TabsVariant
}): React.ReactElement {
  return (
    <TabsPrimitive.List
      className={cn(
        'relative z-0 flex w-fit items-center justify-center gap-x-0.5 text-muted-foreground',
        'data-[orientation=vertical]:flex-col',
        variant === 'default'
          ? 'rounded-lg bg-muted p-0.5 text-muted-foreground/72'
          : 'data-[orientation=vertical]:px-1 data-[orientation=horizontal]:py-1 *:data-[slot=tabs-tab]:hover:bg-accent',
        className,
      )}
      data-slot="tabs-list"
      {...props}
    >
      {children}
      <TabsPrimitive.Indicator
        className={cn(
          'absolute bottom-0 left-0 h-(--active-tab-height) w-(--active-tab-width) translate-x-(--active-tab-left) -translate-y-(--active-tab-bottom) transition-[width,translate] duration-200 ease-in-out',
          variant === 'underline'
            ? 'z-10 bg-primary data-[orientation=horizontal]:h-0.5 data-[orientation=vertical]:w-0.5 data-[orientation=vertical]:-translate-x-px data-[orientation=horizontal]:translate-y-px'
            : '-z-1 rounded-md bg-background shadow-sm/5 dark:bg-input',
        )}
        data-slot="tab-indicator"
      />
    </TabsPrimitive.List>
  )
}

export function TabsTab({ className, ...props }: TabsPrimitive.Tab.Props): React.ReactElement {
  return (
    <TabsPrimitive.Tab
      className={cn(
        "relative flex h-9 shrink-0 grow cursor-pointer items-center justify-center gap-1.5 whitespace-nowrap rounded-md border border-transparent px-[calc(--spacing(2.5)-1px)] font-medium text-base outline-none transition-[color,background-color,box-shadow] hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring data-disabled:pointer-events-none data-[orientation=vertical]:w-full data-[orientation=vertical]:justify-start data-active:text-foreground data-disabled:opacity-64 sm:h-8 sm:text-sm [&_svg:not([class*='size-'])]:size-4.5 sm:[&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:-mx-0.5 [&_svg]:shrink-0",
        className,
      )}
      data-slot="tabs-tab"
      {...props}
    />
  )
}

export function TabsPanel({ className, ...props }: TabsPrimitive.Panel.Props): React.ReactElement {
  return (
    <TabsPrimitive.Panel
      className={cn('flex-1 outline-none', className)}
      data-slot="tabs-content"
      {...props}
    />
  )
}

/** Standalone tab button used by Crowbar feature modules (does not require a tabs value context) */
export interface TabProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  isActive?: boolean
  isDragged?: boolean
  action?: React.ReactNode
  variant?: string
  size?: 'xs' | 'sm' | 'md' | 'lg'
  labelPosition?: 'start' | 'center' | 'end'
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
        'relative inline-flex shrink-0 cursor-pointer items-center whitespace-nowrap rounded-full border font-medium text-sm outline-none transition-colors',
        'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background',
        'disabled:pointer-events-none disabled:opacity-64',
        isActive
          ? 'rounded-full border-background bg-background text-foreground shadow-xs shadow-black/10 not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)] active:inset-shadow-[0_1px_--theme(--color-black/8%)] active:shadow-none'
          : 'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
        className,
      )}
      {...props}
    >
      {children}
      {action}
    </button>
  ),
)
Tab.displayName = 'Tab'

/** Crowbar tab item descriptor */
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
    side?: 'top' | 'right' | 'bottom' | 'left'
    className?: string
  }
}

export { Tab }

export { TabsPrimitive, TabsTab as TabsTrigger, TabsPanel as TabsContent }
