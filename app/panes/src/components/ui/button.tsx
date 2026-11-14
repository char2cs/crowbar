'use client'

import { mergeProps } from '@base-ui/react/merge-props'
import { useRender } from '@base-ui/react/use-render'
import { type VariantProps } from 'class-variance-authority'
import type * as React from 'react'
import { cn } from '@/lib/utils'
import { buttonVariants } from '@/components/ui/button-variants'
import Keybinding from '@/components/ui/keybinding'
import { Spinner } from '@/components/ui/spinner'
import { WithTooltip } from '@/components/ui/tooltip'

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
    <WithTooltip
      trigger={buttonEl}
      side={tooltipSide}
      className={cn(shortcut && 'flex items-center gap-2')}
      content={
        <>
          {tooltip}
          {shortcut && <Keybinding binding={shortcut} />}
        </>
      }
    />
  )
}
