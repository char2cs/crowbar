'use client'

import * as React from 'react'

import * as ToolbarPrimitive from '@radix-ui/react-toolbar'
import * as TooltipPrimitive from '@radix-ui/react-tooltip'
import { type VariantProps, cva } from 'class-variance-authority'
import { ChevronDown } from 'lucide-react'

import {
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuSeparator,
} from '@/components/ui/dropdown-menu'
import { Separator } from '@/components/ui/separator'
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

function ToolbarToggleGroup({
  className,
  ...props
}: React.ComponentProps<typeof ToolbarPrimitive.ToolbarToggleGroup>) {
  return (
    <ToolbarPrimitive.ToolbarToggleGroup
      className={cn('flex items-center', className)}
      {...props}
    />
  )
}

// From toggleVariants
const toolbarButtonVariants = cva(
  "inline-flex cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-md font-medium text-sm outline-none transition-[color,box-shadow] hover:bg-muted hover:text-muted-foreground focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-checked:bg-accent aria-checked:text-accent-foreground aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:shrink-0",
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
} & Omit<React.ComponentPropsWithoutRef<typeof ToolbarToggleItem>, 'asChild' | 'value'> &
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
  return typeof pressed === 'boolean' ? (
    <ToolbarToggleGroup disabled={props.disabled} value="single" type="single">
      <ToolbarToggleItem
        className={cn(
          toolbarButtonVariants({
            size,
            variant,
          }),
          isDropdown && 'justify-between gap-1 pr-1',
          className,
        )}
        value={pressed ? 'single' : ''}
        {...props}
      >
        {isDropdown ? (
          <>
            <div className="flex flex-1 items-center gap-2 whitespace-nowrap">{children}</div>
            <div>
              <ChevronDown className="size-3.5 text-muted-foreground" data-icon />
            </div>
          </>
        ) : (
          children
        )}
      </ToolbarToggleItem>
    </ToolbarToggleGroup>
  ) : (
    <ToolbarPrimitive.Button
      className={cn(
        toolbarButtonVariants({
          size,
          variant,
        }),
        isDropdown && 'pr-1',
        className,
      )}
      {...props}
    >
      {children}
    </ToolbarPrimitive.Button>
  )
})

function ToolbarToggleItem({
  className,
  size = 'sm',
  variant,
  ...props
}: React.ComponentProps<typeof ToolbarPrimitive.ToggleItem> &
  VariantProps<typeof toolbarButtonVariants>) {
  return (
    <ToolbarPrimitive.ToggleItem
      className={cn(toolbarButtonVariants({ size, variant }), className)}
      {...props}
    />
  )
}

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
  tooltipContentProps?: Omit<React.ComponentPropsWithoutRef<typeof TooltipContent>, 'children'>
  tooltipProps?: Omit<React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Root>, 'children'>
  tooltipTriggerProps?: React.ComponentPropsWithoutRef<typeof TooltipPrimitive.Trigger>
} & React.ComponentProps<T>

// Uses `@radix-ui/react-tooltip` primitives directly (rather than the
// app's `@/components/ui/tooltip`, which has a bespoke non-compound API)
// so this file stays a self-contained, registry-generated Plate kit file.
function withTooltip<T extends React.ElementType>(Component: T) {
  return function ExtendComponent({
    tooltip,
    tooltipContentProps,
    tooltipProps,
    tooltipTriggerProps,
    ...props
  }: TooltipProps<T>) {
    const [mounted, setMounted] = React.useState(false)

    // react-doctor-disable-next-line rendering-hydration-no-flicker -- vendored Plate registry code. This is the classic SSR-hydration guard (defer the Radix tooltip portal until after the first client paint). Crowbar is a client-only Vite SPA inside Tauri with no server render, so there is nothing to flicker against; the gate only delays the tooltip wrapper by one commit. Neither remedy the rule proposes applies: there is no external store to read (`useSyncExternalStore`) and `suppressHydrationWarning` addresses a warning this app never emits. Removing the gate outright would change when the tooltip becomes interactive, so it is kept as vendored.
    React.useEffect(() => {
      setMounted(true)
    }, [])

    const component = <Component {...(props as React.ComponentProps<T>)} />

    if (tooltip && mounted) {
      return (
        <TooltipPrimitive.Root {...tooltipProps}>
          <TooltipPrimitive.Trigger {...tooltipTriggerProps}>{component}</TooltipPrimitive.Trigger>

          <TooltipContent {...tooltipContentProps}>{tooltip}</TooltipContent>
        </TooltipPrimitive.Root>
      )
    }

    return component
  }
}

function TooltipContent({
  children,
  className,
  // CHANGE
  sideOffset = 4,
  ...props
}: React.ComponentProps<typeof TooltipPrimitive.Content>) {
  return (
    <TooltipPrimitive.Portal>
      <TooltipPrimitive.Content
        className={cn(
          'z-50 w-fit origin-(--radix-tooltip-content-transform-origin) text-balance rounded-md bg-primary px-3 py-1.5 text-primary-foreground text-xs',
          className,
        )}
        data-slot="tooltip-content"
        sideOffset={sideOffset}
        {...props}
      >
        {children}
        {/* CHANGE */}
        {/* <TooltipPrimitive.Arrow className="z-50 size-2.5 translate-y-[calc(-50%_-_2px)] rotate-45 rounded-[2px] bg-primary fill-primary" /> */}
      </TooltipPrimitive.Content>
    </TooltipPrimitive.Portal>
  )
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
