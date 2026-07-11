import { useState } from 'react'
import type React from 'react'
import { cn } from '@/lib/utils'

// Discover every flip-dot spinner SVG as raw markup at build time. Inlining the
// markup (not an <img src>) is load-bearing: it lets each SVG's fill="currentColor"
// dots inherit the Crowbar theme token from an ancestor and lets its declarative
// <animate> run (an <img>-sourced SVG can't inherit currentColor or self-animate
// reliably). Adding a spinner = drop an .svg in ./spinners — no index, no codegen.
const SPINNERS = Object.values(
  import.meta.glob('./spinners/*.svg', { eager: true, query: '?raw', import: 'default' }),
) as string[]

export function FlickerSpinner({
  className,
  ...props
}: React.ComponentProps<'span'>): React.ReactElement {
  // Random-pick one spinner per instance, stable for the component's lifetime
  // (mirrors the retired WorkspaceAgentSpinner's spinnerNames pick).
  const [markup] = useState(() => SPINNERS[Math.floor(Math.random() * SPINNERS.length)] ?? '')
  return (
    <span
      role="status"
      aria-label="Loading"
      // Size via className (default size-4). Color is NOT baked in here: the
      // SVG dots use fill="currentColor", so callers color this by wrapping it
      // in (or applying) a text-* theme token span — never a hardcoded color.
      className={cn('inline-flex size-4 items-center justify-center [&>svg]:size-full', className)}
      dangerouslySetInnerHTML={{ __html: markup }}
      {...props}
    />
  )
}
