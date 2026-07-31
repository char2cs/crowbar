import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'

/**
 * One collapsible section of the tree, as ONE box.
 *
 * Every section a collapse hides gets its own wrapper so the tween has a single
 * element to close rather than N rows to orchestrate in step. Height cannot be
 * composited, so the cheap thing available is to animate one box instead of one
 * per row — and only the section the user actually toggled.
 *
 * The inline height is handed back to the layout the moment an expand finishes.
 * Leaving `height: 240px` on the box is the silent failure here: the section
 * looks correct until a row is added to it, and then it can never grow again.
 */

/** Measured in the prototype; both directions share it. */
const FOLD_MS = 120
const FOLD_EASE = 'cubic-bezier(0.42, 0, 0.58, 1)'
/** A `transitionend` that never arrives — an interrupted tween, a hidden tab —
 *  must not strand the box mid-close, so the finish is armed on a timer too. */
const FOLD_TIMEOUT_MS = FOLD_MS + 60

function prefersReducedMotion(): boolean {
  return typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
    : false
}

/** Give the box back to the layout: no inline height, no clip, no transition. */
function releaseBox(el: HTMLElement): void {
  el.style.transition = ''
  el.style.height = ''
  el.style.opacity = ''
  el.style.overflow = ''
}

interface CollapseSectionProps {
  /** Whether the section is showing. Children unmount once a close finishes. */
  open: boolean
  /** ARIA role for the box; the tree's child sections are `group`s. */
  role?: string
  className?: string
  children: ReactNode
}

export function CollapseSection({ open, role, className, children }: CollapseSectionProps) {
  const ref = useRef<HTMLDivElement | null>(null)
  // The children outlive `open` by one animation: there has to be something in
  // the box for the close to close over.
  const [mounted, setMounted] = useState(open)
  // What the box is currently SHOWING, which lags `open` while a close runs.
  const shown = useRef(open)
  // Tears down a tween in flight without deciding anything about mounting.
  const cancel = useRef<(() => void) | null>(null)

  // Adjusting state during render rather than from an effect: an opening
  // section has to be in the DOM before the layout effect below can measure it,
  // and going through an effect would let its full height paint unanimated for
  // one frame first.
  if (open && !mounted) setMounted(true)

  useLayoutEffect(() => {
    if (shown.current === open) return
    shown.current = open
    // Whatever the last tween was doing, it is answering a stale question now.
    cancel.current?.()

    const el = ref.current
    if (!el) {
      if (!open) setMounted(false)
      return
    }

    const height = el.scrollHeight
    // Nothing measurable to tween between — a headless layout, or a section
    // whose rows have no height yet. Snap to the end state: an animation from
    // zero to zero is 120ms that hides nothing.
    if (height === 0 || prefersReducedMotion()) {
      releaseBox(el)
      if (!open) setMounted(false)
      return
    }

    const stop = () => {
      window.clearTimeout(timer)
      el.removeEventListener('transitionend', onEnd)
      cancel.current = null
      releaseBox(el)
    }
    const finish = () => {
      stop()
      if (!open) setMounted(false)
    }
    const onEnd = (e: TransitionEvent) => {
      if (e.target === el && e.propertyName === 'height') finish()
    }

    el.style.overflow = 'hidden'
    el.style.height = `${open ? 0 : height}px`
    el.style.opacity = open ? '0' : '1'
    // Read back, or the browser coalesces both writes and there is no start
    // value left to animate away from.
    el.getBoundingClientRect()
    el.style.transition = `height ${FOLD_MS}ms ${FOLD_EASE}, opacity ${FOLD_MS}ms ${FOLD_EASE}`
    el.style.height = `${open ? height : 0}px`
    el.style.opacity = open ? '1' : '0'

    const timer = window.setTimeout(finish, FOLD_TIMEOUT_MS)
    el.addEventListener('transitionend', onEnd)
    cancel.current = stop
    return stop
  }, [open])

  if (!mounted) return null

  return (
    <div
      ref={ref}
      role={role}
      className={className}
      data-collapse-section={open ? 'open' : 'closing'}
    >
      {children}
    </div>
  )
}
