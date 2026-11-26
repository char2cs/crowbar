'use client'

import { Tabs as TabsPrimitive } from '@base-ui/react/tabs'
import type * as React from 'react'
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
