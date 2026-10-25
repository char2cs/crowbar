'use client'

import { mergeProps } from '@base-ui/react/merge-props'
import { Tabs as TabsPrimitive } from '@base-ui/react/tabs'
import { useRender } from '@base-ui/react/use-render'
import * as React from 'react'
import { cn } from '@/lib/utils'

type SegmentedControlSize = 'default' | 'lg' | 'sm'

const itemSizeClassNames: Record<SegmentedControlSize, string> = {
  default: 'h-8.5 px-[calc(--spacing(2.5)-1px)] sm:h-7.5',
  lg: 'h-9.5 px-[calc(--spacing(3)-1px)] sm:h-8.5',
  sm: 'h-7.5 px-[calc(--spacing(2)-1px)] sm:h-6.5',
}

const SegmentedControlContext = React.createContext<SegmentedControlSize>('default')

/**
 * CossUI Segmented Control. The raised pill slides via Base UI's own Tabs
 * indicator, so the root must be rendered as a Base UI `Tabs.List`.
 */
export function SegmentedControl({
  className,
  children,
  render,
  size = 'default',
  ...props
}: useRender.ComponentProps<'div'> & { size?: SegmentedControlSize }): React.ReactElement {
  const defaultProps = {
    children: (
      <>
        {children}
        <TabsPrimitive.Indicator
          className="-z-1 absolute bottom-0 left-0 h-(--active-tab-height) w-(--active-tab-width) translate-x-(--active-tab-left) -translate-y-(--active-tab-bottom) rounded-md bg-background shadow-sm/5 transition-[width,translate] duration-200 ease-in-out motion-reduce:transition-none dark:bg-input"
          data-slot="segmented-control-indicator"
        />
      </>
    ),
    className: cn(
      'relative z-0 flex w-fit items-center justify-center gap-0.5 rounded-lg bg-muted p-0.5',
      className,
    ),
    'data-slot': 'segmented-control',
  }
  const element = useRender({
    defaultTagName: 'div',
    props: mergeProps<'div'>(defaultProps, props),
    render,
  })
  return <SegmentedControlContext.Provider value={size}>{element}</SegmentedControlContext.Provider>
}

export function SegmentedControlItem({
  className,
  render,
  size,
  ...props
}: useRender.ComponentProps<'button'> & { size?: SegmentedControlSize }): React.ReactElement {
  const contextSize = React.useContext(SegmentedControlContext)
  const resolvedSize = size ?? contextSize
  const defaultProps = {
    className: cn(
      "relative inline-flex shrink-0 cursor-pointer select-none items-center justify-center gap-1.5 whitespace-nowrap rounded-md border border-transparent font-medium text-base text-muted-foreground/72 outline-2 outline-transparent transition-[color,outline-color] hover:text-muted-foreground focus-visible:outline-ring data-disabled:pointer-events-none data-disabled:opacity-64 data-active:text-foreground sm:text-sm [&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:shrink-0",
      itemSizeClassNames[resolvedSize],
      className,
    ),
    'data-slot': 'segmented-control-item',
    ...(render ? {} : { type: 'button' as const }),
  }
  return useRender({
    defaultTagName: 'button',
    props: mergeProps<'button'>(defaultProps, props),
    render,
  })
}
