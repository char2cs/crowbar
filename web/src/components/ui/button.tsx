'use client'

import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { mergeProps } from '@base-ui/react/merge-props'
import { useRender } from '@base-ui/react/use-render'
import { type VariantProps } from 'class-variance-authority'
import type * as React from 'react'
import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button-variants'
import Keybinding from '@/components/ui/keybinding'
import { Spinner } from '@/components/ui/spinner'
import { tooltipContentBase } from '@/components/ui/tooltip'

export interface ButtonProps extends useRender.ComponentProps<'button'> {
  variant?: VariantProps<typeof buttonVariants>['variant']
  size?: VariantProps<typeof buttonVariants>['size']
  loading?: boolean
  /** Active state — adds bg-accent/20 highlight when true */
  active?: boolean
  /** Compact mode (compat, no visual effect) */
  compact?: boolean
  /** Tooltip text — renders a real tooltip on hover */
  tooltip?: string
  /** Keyboard shortcut shown in the tooltip */
  shortcut?: string
  /** Tooltip side preference */
  tooltipSide?: 'top' | 'right' | 'bottom' | 'left'
  /** Command ID for keybinding hints (compat, not rendered) */
  commandId?: string
}

export function Button({
  className,
  variant,
  size,
  render,
  children,
  loading = false,
  disabled: disabledProp,
  active,
  compact: _compact,
  tooltip,
  shortcut,
  tooltipSide = 'top',
  commandId: _commandId,
  ...props
}: ButtonProps): React.ReactElement {
  const isDisabled: boolean = Boolean(loading || disabledProp)
  const typeValue: React.ButtonHTMLAttributes<HTMLButtonElement>['type'] = render
    ? undefined
    : 'button'

  const defaultProps = {
    children: (
      <>
        {children}
        {loading && (
          <Spinner className="pointer-events-none absolute" data-slot="button-loading-indicator" />
        )}
      </>
    ),
    className: cn(buttonVariants({ className, size, variant }), active && 'bg-accent/20'),
    'aria-disabled': loading || undefined,
    'data-loading': loading ? '' : undefined,
    'data-slot': 'button',
    disabled: isDisabled,
    type: typeValue,
  }

  const buttonEl = useRender({
    defaultTagName: 'button',
    props: mergeProps<'button'>(defaultProps, props),
    render,
  })

  if (!tooltip) return buttonEl

  return (
    // skipDelayDuration=0, not Radix's 300ms default or the 100ms this used to
    // be: that window lets a SECOND trigger's tooltip open instantly, skipping
    // the normal 150ms delay, once one tooltip has already opened recently.
    // Live-measured (rAF-delta sampling) as the actual cause of sidebar rows
    // getting janky specifically while being hovered — sweeping the mouse
    // down a list of rows crosses many Fork/Thread buttons in quick
    // succession, and 100ms of "open instantly" is SHORTER than this
    // tooltip's own 150ms close animation (tw-animate-css's default
    // --tw-duration), during which Radix keeps the closing tooltip's full
    // floating-ui rig mounted (two ResizeObservers, an IntersectionObserver
    // that reconnects on every threshold cross, ancestor scroll/resize
    // listeners). A fast-enough sweep had two of those rigs alive and doing
    // setup/teardown at once. At 0, nothing ever skips the delay — a quick
    // pass over a row never opens a tooltip at all, which is also just the
    // right behaviour for a pass-through hover.
    <TooltipPrimitive.Provider delayDuration={150} skipDelayDuration={0} disableHoverableContent>
      <TooltipPrimitive.Root>
        <TooltipPrimitive.Trigger asChild>{buttonEl}</TooltipPrimitive.Trigger>
        <TooltipPrimitive.Portal>
          <TooltipPrimitive.Content
            side={tooltipSide}
            sideOffset={6}
            collisionPadding={8}
            className={cn(tooltipContentBase, shortcut && 'flex items-center gap-2')}
          >
            {tooltip}
            {shortcut && <Keybinding binding={shortcut} />}
          </TooltipPrimitive.Content>
        </TooltipPrimitive.Portal>
      </TooltipPrimitive.Root>
    </TooltipPrimitive.Provider>
  )
}
