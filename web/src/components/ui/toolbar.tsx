'use client'

import * as React from 'react'

import { Toggle } from '@base-ui/react/toggle'
import { Toolbar as ToolbarPrimitive } from '@base-ui/react/toolbar'
import { type VariantProps, cva } from 'class-variance-authority'
import { CaretDownIcon } from '@phosphor-icons/react'

import {
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { Separator } from '@/components/ui/separator'
import { WithTooltip, tooltipContentPrimary } from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

export function Toolbar({
  className,
  ...props
}: React.ComponentProps<typeof ToolbarPrimitive.Root>) {
  return (
    <ToolbarPrimitive.Root
      className={cn('relative flex select-none items-center', className)}
      {...props}
    />
  )
}

// From toggleVariants
const toolbarButtonVariants = cva(
  "inline-flex cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-md font-medium text-sm outline-none transition-[color,box-shadow] hover:bg-muted hover:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 data-pressed:bg-accent data-pressed:text-accent-foreground aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:shrink-0",
  {
    defaultVariants: {
      size: 'default',
      variant: 'default',
    },
    variants: {
      size: {
        default: 'h-9 min-w-9 px-2',
        lg: 'h-10 min-w-10 px-2.5',
        sm: 'h-8 min-w-8 px-1.5',
      },
      variant: {
        default: 'bg-transparent',
        outline:
          'border border-input bg-transparent shadow-xs hover:bg-accent hover:text-accent-foreground',
      },
    },
  },
)

type ToolbarButtonProps = {
  isDropdown?: boolean
  pressed?: boolean
} & Omit<React.ComponentProps<'button'>, 'value'> &
  VariantProps<typeof toolbarButtonVariants>

export const ToolbarButton = withTooltip(function ToolbarButton({
  children,
  className,
  isDropdown,
  pressed,
  size = 'sm',
  variant,
  ...props
}: ToolbarButtonProps) {
  const content = isDropdown ? (
    <>
      <div className="flex flex-1 items-center gap-2 whitespace-nowrap">{children}</div>
      <div>
        <CaretDownIcon className="size-3.5 text-muted-foreground" data-icon />
      </div>
    </>
  ) : (
    children
  )
  const buttonClassName = cn(
    toolbarButtonVariants({ size, variant }),
    isDropdown && (typeof pressed === 'boolean' ? 'justify-between gap-1 pr-1' : 'pr-1'),
    className,
  )

  // A `pressed` button is a toggle (aria-pressed / data-pressed); either way it
  // is one <button> in the toolbar's roving focus.
  return (
    <ToolbarPrimitive.Button
      className={buttonClassName}
      render={typeof pressed === 'boolean' ? <Toggle pressed={pressed} /> : undefined}
      {...props}
    >
      {typeof pressed === 'boolean' ? content : children}
    </ToolbarPrimitive.Button>
  )
})

export function ToolbarGroup({ children, className }: React.ComponentProps<'div'>) {
  return (
    <div className={cn('group/toolbar-group', 'relative hidden has-[button]:flex', className)}>
      <div className="flex items-center">{children}</div>

      <div className="group-last/toolbar-group:hidden! mx-1.5 py-0.5">
        <Separator orientation="vertical" />
      </div>
    </div>
  )
}

type TooltipProps<T extends React.ElementType> = {
  tooltip?: React.ReactNode
} & React.ComponentProps<T>

// The tooltip's props merge onto the button itself (see WithTooltip): every
// `ToolbarButton` resolves to a <button>, whether through `Toolbar.ToggleItem`
// (the `pressed` branch) or `Toolbar.Button`, and a wrapper <button> around it
// would be invalid HTML ("<button> cannot contain a nested button").
function withTooltip<T extends React.ElementType>(Component: T) {
  return function ExtendComponent({ tooltip, ...props }: TooltipProps<T>) {
    const component = <Component {...(props as React.ComponentProps<T>)} />
    if (!tooltip) return component
    return (
      <WithTooltip
        trigger={component}
        content={tooltip}
        sideOffset={4}
        popupClassName={tooltipContentPrimary}
      />
    )
  }
}

export function ToolbarMenuGroup({
  children,
  className,
  label,
  ...props
}: React.ComponentProps<typeof DropdownMenuRadioGroup> & { label?: string }) {
  return (
    <>
      <DropdownMenuSeparator
        className={cn(
          'hidden',
          'mb-0 shrink-0 peer-has-[[role=menuitem]]/menu-group:block peer-has-[[role=menuitemradio]]/menu-group:block peer-has-[[role=option]]/menu-group:block',
        )}
      />

      <DropdownMenuRadioGroup
        {...props}
        className={cn(
          'hidden',
          'peer/menu-group group/menu-group my-1.5 has-[[role=menuitem]]:block has-[[role=menuitemradio]]:block has-[[role=option]]:block',
          className,
        )}
      >
        {label && (
          <DropdownMenuLabel className="select-none font-semibold text-muted-foreground text-xs">
            {label}
          </DropdownMenuLabel>
        )}
        {children}
      </DropdownMenuRadioGroup>
    </>
  )
}
