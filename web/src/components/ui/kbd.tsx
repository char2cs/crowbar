import type * as React from 'react'
import { cn } from '@/lib/utils'

/**
 * Carries `data-oracle-content-sized` and deliberately **not**
 * `data-oracle-line-sized`.
 *
 * A keycap's used width is its label's max-content width plus `px-1`, floored
 * by `min-w-5` — and `min-w-*` is a floor, never a stretch — so
 * `native/oracle/ANCHORS.md` v1.5 holds. v1.6 does not: `h-5` *authors* the box
 * at 20px around a 16px line box, and the test that rule states is whether the
 * height is derived from the line box, not whether the element paints text.
 * Declaring it would compare 20 against 16 and manufacture a 4px delta on this
 * surface's only anchor — `badge`'s precedent, in a second place.
 */
export function Kbd({ className, ...props }: React.ComponentProps<'kbd'>): React.ReactElement {
  return (
    <kbd
      className={cn(
        "pointer-events-none inline-flex h-5 min-w-5 select-none items-center justify-center gap-1 rounded bg-muted px-1 font-medium font-sans text-muted-foreground text-xs [&_svg:not([class*='size-'])]:size-3",
        className,
      )}
      data-oracle-content-sized="true"
      data-oracle-id="kbd"
      data-slot="kbd"
      {...props}
    />
  )
}

/**
 * Deliberately carries **no `data-oracle-id`**.
 *
 * `data-oracle-id="kbd"` lives on the primitive above, so every cap inside a
 * group carries it. A snapshot rooted at the group would therefore contain that
 * id twice, which `native/oracle/ANCHORS.md` v1.8 ranks a *refusal* rather than
 * a delta — the differ matches by id and would have no way to say which of the
 * two it compared. The group is ported (`crowbar_ui::components::kbd::KbdGroup`)
 * as the layout its caps sit in, and is measured through them.
 */
export function KbdGroup({ className, ...props }: React.ComponentProps<'kbd'>): React.ReactElement {
  return (
    <kbd
      className={cn('inline-flex items-center gap-1', className)}
      data-slot="kbd-group"
      {...props}
    />
  )
}
