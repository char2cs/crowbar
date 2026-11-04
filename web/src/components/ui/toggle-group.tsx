'use client'

import { Toggle as TogglePrimitive } from '@base-ui/react/toggle'
import { ToggleGroup as ToggleGroupPrimitive } from '@base-ui/react/toggle-group'
import * as React from 'react'
import { cn } from '@/lib/utils'

export function ToggleGroup({
  className,
  ...props
}: ToggleGroupPrimitive.Props): React.ReactElement {
  return (
    <ToggleGroupPrimitive
      className={cn('flex w-fit items-center gap-0.5 rounded-lg bg-muted p-0.5', className)}
      data-slot="toggle-group"
      {...props}
    />
  )
}

export function Toggle({ className, ...props }: TogglePrimitive.Props): React.ReactElement {
  return (
    <TogglePrimitive
      className={cn(
        "relative inline-flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-md border border-transparent font-medium text-foreground outline-none transition-[color,background-color,box-shadow] hover:bg-transparent hover:text-muted-foreground data-pressed:hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-64 data-pressed:bg-background data-pressed:shadow-sm/5 dark:data-pressed:bg-input [&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:shrink-0",
        className,
      )}
      data-slot="toggle"
      {...props}
    />
  )
}
