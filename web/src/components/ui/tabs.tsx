'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'

export interface TabProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  isActive?: boolean
  isDragged?: boolean
  action?: React.ReactNode
  /**
   * 'pill' (default, unchanged): the filled/rounded/shadowed treatment.
   * 'underline': flat, no radius, no fill at rest — a static 2px `bg-primary`
   * bar on the active tab, matching Main.dc.html's `.hitem`/`.hitem.is-on`
   * rule. This is NOT the compound `TabsList variant="underline"` system
   * (that one drives a single sliding indicator across `Tabs.Root`-managed
   * children, which doesn't fit a dnd-kit-sortable, dynamically-mutating tab
   * strip) — it's the same visual tokens (bg-primary, 2px), applied
   * per-button so any standalone `Tab` can carry it.
   * 'ghost': the IDE sector's own tab strip — the same toolbar-button recipe
   * as its row neighbours (`SplitToggleButton`/`CloseViewButton`/
   * `TabAddButton`: transparent at rest, `sidebar-element-hover` on
   * hover/active, rounded-sm, no underline bar) so a tab reads as the same
   * chrome family as the buttons beside it rather than a distinct pill or
   * underline treatment.
   */
  variant?: 'pill' | 'underline' | 'ghost'
  size?: 'xs' | 'sm' | 'md' | 'lg'
  labelPosition?: 'start' | 'center' | 'end'
  maxWidth?: number
}

const Tab = React.forwardRef<HTMLButtonElement, TabProps>(
  (
    {
      className,
      isActive,
      isDragged: _isDragged,
      action,
      variant = 'pill',
      size: _size,
      labelPosition: _labelPosition,
      maxWidth: _maxWidth,
      children,
      onMouseDown,
      onClick,
      ...props
    },
    ref,
  ) => (
    <button
      ref={ref}
      // A click activates the tab; it should not ALSO leave it visibly
      // focused. `:focus-visible` is meant to tell a mouse click and a
      // keyboard Tab-press apart on its own, but WebKit shows the ring for
      // both (Chromium doesn't) — caught live: every pane click left its
      // tab wearing the ring for as long as focus sat there, flashing on
      // every switch. `preventDefault()` on mousedown SHOULD be enough on
      // its own (it stops a click from moving focus at all; keyboard
      // navigation never goes through mousedown, so it loses nothing) — but
      // this WebKit build kept showing the ring even with it in place, on a
      // REAL trackpad click specifically (every synthetic click this was
      // tested against — WebDriver-dispatched, `element.click()` — never
      // reproduced it, confirmed with a 90-frame :focus-visible/:focus
      // logger across a fresh reload). `onClick`'s own blur below is the
      // belt-and-suspenders fix: it does not depend on WHICH heuristic this
      // engine actually used to decide the ring was earned, it just refuses
      // to let one survive a mouse click, full stop. Runs after a caller's
      // own handler and only if that handler left the event alone, so
      // dnd-kit's own drag-activation mousedown (the editor tab strip's
      // reordering) is untouched.
      onMouseDown={(e) => {
        onMouseDown?.(e)
        if (!e.defaultPrevented) e.preventDefault()
      }}
      // `e.detail` is the spec's own way to tell a mouse click from a
      // keyboard-activated one (Enter/Space on a focused button): a real
      // click reports the number of times the button was pressed at that
      // point (>= 1), a keyboard activation reports 0. Blurring only the
      // mouse case is what keeps this from also undoing REAL keyboard
      // focus — the one case `:focus-visible` exists to show a ring for.
      onClick={(e) => {
        onClick?.(e)
        if (e.detail !== 0) e.currentTarget.blur()
      }}
      className={cn(
        'relative inline-flex shrink-0 cursor-pointer items-center whitespace-nowrap border font-medium text-sm outline-none transition-colors',
        'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background',
        'disabled:pointer-events-none disabled:opacity-64',
        variant === 'underline'
          ? cn(
              'border-transparent hover:bg-accent',
              isActive ? 'text-foreground' : 'text-muted-foreground hover:text-foreground',
            )
          : variant === 'ghost'
            ? cn(
                'rounded-sm border-transparent hover:bg-sidebar-element-hover',
                isActive
                  ? 'bg-sidebar-element-hover text-foreground'
                  : 'text-muted-foreground hover:text-foreground',
              )
            : cn(
                'rounded-full',
                isActive
                  ? 'rounded-full border-background bg-background text-foreground shadow-xs shadow-black/10 not-disabled:inset-shadow-[0_1px_var(--elevated-highlight)] active:inset-shadow-[0_1px_--theme(--color-black/8%)] active:shadow-none'
                  : 'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
              ),
        className,
      )}
      {...props}
    >
      {children}
      {variant === 'underline' && isActive && (
        <span
          aria-hidden="true"
          data-testid="tab-underline"
          className="pointer-events-none absolute inset-x-1.5 bottom-0 h-0.5 rounded-[1px] bg-primary"
        />
      )}
      {action}
    </button>
  ),
)
Tab.displayName = 'Tab'

export { Tab }
