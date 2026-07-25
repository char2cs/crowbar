// Adapted from the Plate registry (`https://platejs.org/r/floating-toolbar-kit.json`
// -> `floating-toolbar`). Trimmed: upstream also hides the toolbar while an
// AI chat panel is open (`usePluginOption({ key: KEYS.aiChat }, 'open')`) —
// no AI kit is registered here, so that check is dropped (the floating link
// editor's own `isFloatingLinkOpen` check is kept — LinkKit IS registered).
// Also dropped the `scrollbar-hide` utility class: it comes from a Tailwind
// v3-style plugin (`tailwind-scrollbar-hide`) that doesn't fit this app's
// Tailwind v4 CSS-only config, and this toolbar's button set is short enough
// to never need horizontal scrolling in the first place.
'use client'

import * as React from 'react'

import {
  type FloatingToolbarState,
  flip,
  offset,
  useFloatingToolbar,
  useFloatingToolbarState,
} from '@platejs/floating'
import { KEYS } from 'platejs'
import { useComposedRef, useEditorId, useEventEditorValue, usePluginOption } from 'platejs/react'

import { cn } from '@/lib/utils'

import { Toolbar } from './toolbar'

export function FloatingToolbar({
  children,
  className,
  state,
  ...props
}: React.ComponentProps<typeof Toolbar> & {
  state?: FloatingToolbarState
}) {
  const editorId = useEditorId()
  const focusedEditorId = useEventEditorValue('focus')
  const isFloatingLinkOpen = !!usePluginOption({ key: KEYS.link }, 'mode')

  const floatingToolbarState = useFloatingToolbarState({
    editorId,
    focusedEditorId,
    hideToolbar: isFloatingLinkOpen,
    ...state,
    floatingOptions: {
      middleware: [
        offset(12),
        flip({
          fallbackPlacements: ['top-start', 'top-end', 'bottom-start', 'bottom-end'],
          padding: 12,
        }),
      ],
      placement: 'top',
      ...state?.floatingOptions,
    },
  })

  const {
    clickOutsideRef,
    hidden,
    props: rootProps,
    ref: floatingRef,
  } = useFloatingToolbar(floatingToolbarState)

  const ref = useComposedRef<HTMLDivElement>(props.ref, floatingRef)

  if (hidden) return null

  return (
    <div ref={clickOutsideRef}>
      <Toolbar
        {...props}
        {...rootProps}
        ref={ref}
        className={cn(
          'absolute z-40 overflow-x-auto whitespace-nowrap rounded-md border border-border bg-popover p-1 text-popover-foreground opacity-100 shadow-md print:hidden',
          'max-w-[80vw]',
          className,
        )}
      >
        {children}
      </Toolbar>
    </div>
  )
}
