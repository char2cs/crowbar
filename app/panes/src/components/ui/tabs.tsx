'use client'

import { Tabs as TabsPrimitive } from '@base-ui/react/tabs'
import * as React from 'react'
import { cn } from '@/lib/utils'

export function Tabs({ className, ...props }: TabsPrimitive.Root.Props): React.ReactElement {
  return (
    <TabsPrimitive.Root
      className={cn('flex flex-col gap-2 data-[orientation=vertical]:flex-row', className)}
      data-slot="tabs"
      {...props}
    />
  )
}

interface TabProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  isActive?: boolean
  isDragged?: boolean
  action?: React.ReactNode
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
      onMouseDown={(e) => {
        onMouseDown?.(e)
        if (!e.defaultPrevented) e.preventDefault()
      }}
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
