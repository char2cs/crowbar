'use client'

import { Tabs as TabsPrimitive } from '@base-ui/react/tabs'
import * as React from 'react'
import { cn } from '@/lib/utils'
import { SELECTED_PILL } from './selected-pill'

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
  className,
  children,
  ...props
}: TabsPrimitive.List.Props): React.ReactElement {
  return (
    <TabsPrimitive.List
      className={cn(
        'relative z-0 flex w-fit items-center justify-center gap-x-0.5 rounded-lg bg-muted p-0.5 text-muted-foreground/72 data-[orientation=vertical]:flex-col',
        className,
      )}
      data-slot="tabs-list"
      {...props}
    >
      {children}
      <TabsPrimitive.Indicator
        className={cn(
          'absolute bottom-0 left-0 -z-1 h-(--active-tab-height) w-(--active-tab-width) translate-x-(--active-tab-left) -translate-y-(--active-tab-bottom) transition-[width,translate] duration-200 ease-in-out',
          SELECTED_PILL,
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
        "relative flex h-9 shrink-0 grow cursor-pointer items-center justify-center gap-1.5 whitespace-nowrap rounded-md border border-transparent px-[calc(--spacing(2.5)-1px)] font-medium text-base outline-none transition-[color,background-color,box-shadow] hover:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring data-disabled:pointer-events-none data-[orientation=vertical]:w-full data-[orientation=vertical]:justify-start data-active:text-control-foreground data-active:[--muted-foreground:var(--control-muted-foreground)] data-disabled:opacity-64 sm:h-8 sm:text-sm [&_svg:not([class*='size-'])]:size-4.5 sm:[&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:-mx-0.5 [&_svg]:shrink-0",
        className,
      )}
      data-slot="tabs-tab"
      {...props}
    />
  )
}

interface TabProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  isActive?: boolean
  isDragged?: boolean
  action?: React.ReactNode
  /** 'pill' (default): filled and rounded. 'underline': a static 2px `bg-primary` bar per button,
   *  not TabsList's sliding indicator (which can't follow a sortable strip). 'ghost': the IDE
   *  strip's toolbar-button recipe, so tabs read as the same chrome as their neighbours. */
  variant?: 'pill' | 'underline' | 'ghost'
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
      variant = 'pill',
      size: _size,
      labelPosition: _labelPosition,
      maxWidth: _maxWidth,
      children,
      onMouseDown,
      onClick,
      ...props
    },
    ref,
  ) => (
    <button
      ref={ref}
      // A click must not leave the focus ring: WebKit shows `:focus-visible` for real trackpad
      // clicks. preventDefault on mousedown stops the focus move (keyboard never uses mousedown),
      // unless a caller's handler (dnd-kit's drag activation) already took the event.
      onMouseDown={(e) => {
        onMouseDown?.(e)
        if (!e.defaultPrevented) e.preventDefault()
      }}
      // And blur on a mouse click (`detail >= 1`; keyboard activation reports 0) as a backstop for
      // whatever WebKit heuristic still earns the ring, keeping real keyboard focus visible.
      onClick={(e) => {
        onClick?.(e)
        if (e.detail !== 0) e.currentTarget.blur()
      }}
      className={cn(
        'relative inline-flex shrink-0 cursor-pointer items-center whitespace-nowrap border font-medium text-sm outline-none transition-colors',
        'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background',
        'disabled:pointer-events-none disabled:opacity-64',
        variant === 'underline'
          ? cn(
              'border-transparent hover:bg-accent',
              isActive ? 'text-foreground' : 'text-muted-foreground hover:text-foreground',
            )
          : variant === 'ghost'
            ? cn(
                'rounded-sm border-transparent hover:bg-sidebar-element-hover',
                isActive
                  ? 'bg-sidebar-element-hover text-foreground'
                  : 'text-muted-foreground hover:text-foreground',
              )
            : cn(
                'rounded-full',
                isActive
                  ? 'control-surface rounded-full border shadow-xs shadow-black/10 not-disabled:inset-shadow-[0_1px_var(--elevated-highlight)] active:inset-shadow-[0_1px_--theme(--color-black/8%)] active:shadow-none'
                  : 'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
              ),
        className,
      )}
      {...props}
    >
      {children}
      {variant === 'underline' && isActive && (
        <span
          aria-hidden="true"
          data-testid="tab-underline"
          className="pointer-events-none absolute inset-x-1.5 bottom-0 h-0.5 rounded-[1px] bg-primary"
        />
      )}
      {action}
    </button>
  ),
)
Tab.displayName = 'Tab'

export { Tab }
