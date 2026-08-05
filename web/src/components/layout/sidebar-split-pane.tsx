import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  type CSSProperties,
  type KeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from 'react'
import { cn } from '@/lib/utils'

const HANDLE_TRACK_PX = 1
const DEFAULT_CONTENT_MIN_RATIO = 0.2
const KEYBOARD_STEP_PX = 16

interface SidebarSplitPaneProps {
  side: 'left' | 'right'
  open: boolean
  preferredWidth: number
  minWidth: number
  maxWidth: number
  sidebar: ReactNode
  children: ReactNode
  onOpenChange: (open: boolean) => void
  onWidthCommit: (width: number) => void
  contentMinRatio?: number
}

interface ActiveDrag {
  cleanup: () => void
}

function clampOpenWidth(
  width: number,
  containerWidth: number,
  minWidth: number,
  maxWidth: number,
  contentMinRatio: number,
): number {
  const available = Math.max(0, containerWidth - HANDLE_TRACK_PX)
  const maximum = Math.min(maxWidth, available * (1 - contentMinRatio))
  const minimum = Math.min(minWidth, maximum)
  return Math.min(maximum, Math.max(minimum, width))
}

function widthFromDrag(
  width: number,
  containerWidth: number,
  minWidth: number,
  maxWidth: number,
  contentMinRatio: number,
): number {
  // Match the collapsible-panel gesture: the sidebar stays open at its minimum
  // until the pointer crosses halfway to zero, then closes. The same threshold
  // makes dragging outward from a closed sidebar deliberate rather than letting
  // a one-pixel wobble reopen it.
  if (width < minWidth / 2) return 0
  return clampOpenWidth(width, containerWidth, minWidth, maxWidth, contentMinRatio)
}

/**
 * The IDE's outer split, deliberately narrower than a general panel library.
 *
 * Its important contract is negative: while idle it has no document/window
 * pointermove listener and performs no geometry reads. A listener exists only
 * between a primary-button pointerdown on the separator and its matching
 * pointerup/cancel. During that drag the container is measured once; subsequent
 * moves only update one CSS custom property, coalesced to animation frames.
 */
export function SidebarSplitPane({
  side,
  open,
  preferredWidth,
  minWidth,
  maxWidth,
  sidebar,
  children,
  onOpenChange,
  onWidthCommit,
  contentMinRatio = DEFAULT_CONTENT_MIN_RATIO,
}: SidebarSplitPaneProps) {
  const rootRef = useRef<HTMLDivElement>(null)
  const sidebarRef = useRef<HTMLDivElement>(null)
  const handleRef = useRef<HTMLDivElement>(null)
  const activeDragRef = useRef<ActiveDrag | null>(null)
  const liveWidthRef = useRef(open ? preferredWidth : 0)

  const writeWidth = useCallback((width: number) => {
    liveWidthRef.current = width
    rootRef.current?.style.setProperty('--sidebar-track-width', `${width}px`)
    handleRef.current?.setAttribute('aria-valuenow', String(Math.round(width)))
  }, [])

  // Programmatic open/close and a restored preference update the track without
  // remounting either side. Never compete with the imperative value mid-drag.
  useLayoutEffect(() => {
    if (activeDragRef.current) return
    writeWidth(open ? preferredWidth : 0)
  }, [open, preferredWidth, writeWidth])

  useEffect(
    () => () => {
      activeDragRef.current?.cleanup()
    },
    [],
  )

  const commitKeyboardWidth = useCallback(
    (requestedWidth: number) => {
      const container = rootRef.current
      if (!container) return
      const containerWidth = container.getBoundingClientRect().width
      const nextWidth =
        requestedWidth <= 0
          ? 0
          : clampOpenWidth(requestedWidth, containerWidth, minWidth, maxWidth, contentMinRatio)
      writeWidth(nextWidth)
      const nextOpen = nextWidth > 0
      onOpenChange(nextOpen)
      if (nextOpen) onWidthCommit(Math.round(nextWidth))
    },
    [contentMinRatio, maxWidth, minWidth, onOpenChange, onWidthCommit, writeWidth],
  )

  const handlePointerDown = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      if (event.button !== 0 || activeDragRef.current) return
      const container = rootRef.current
      const sidebarElement = sidebarRef.current
      if (!container || !sidebarElement) return

      event.preventDefault()
      event.currentTarget.focus({ preventScroll: true })

      // These are the only layout reads in the pointer path, and they occur once
      // after the user has explicitly grabbed the separator.
      const containerWidth = container.getBoundingClientRect().width
      const startWidth = sidebarElement.getBoundingClientRect().width
      const startX = event.clientX
      const pointerId = event.pointerId
      let pendingWidth = startWidth
      let appliedWidth = startWidth
      let animationFrame = 0
      let moved = false
      let finished = false

      const applyPendingWidth = () => {
        animationFrame = 0
        appliedWidth = pendingWidth
        writeWidth(appliedWidth)
      }

      const cleanup = () => {
        if (finished) return
        finished = true
        if (animationFrame !== 0) cancelAnimationFrame(animationFrame)
        window.removeEventListener('pointermove', onPointerMove)
        window.removeEventListener('pointerup', onPointerUp)
        window.removeEventListener('pointercancel', onPointerCancel)
        window.removeEventListener('blur', onWindowBlur)
        activeDragRef.current = null

        if (moved) {
          document.documentElement.removeAttribute('data-pane-resizing')
          window.dispatchEvent(new CustomEvent('pane-resize-end'))
        }
      }

      const flushPendingWidth = () => {
        if (animationFrame !== 0) {
          cancelAnimationFrame(animationFrame)
          animationFrame = 0
        }
        if (appliedWidth !== pendingWidth) applyPendingWidth()
      }

      const onPointerMove = (moveEvent: globalThis.PointerEvent) => {
        if (moveEvent.pointerId !== pointerId) return
        const physicalDelta = moveEvent.clientX - startX
        if (physicalDelta === 0 && !moved) return
        if (!moved) {
          moved = true
          document.documentElement.setAttribute('data-pane-resizing', '1')
        }
        const sidebarDelta = side === 'left' ? physicalDelta : -physicalDelta
        pendingWidth = widthFromDrag(
          startWidth + sidebarDelta,
          containerWidth,
          minWidth,
          maxWidth,
          contentMinRatio,
        )
        if (animationFrame === 0) animationFrame = requestAnimationFrame(applyPendingWidth)
      }

      const finish = (finishEvent: globalThis.PointerEvent, cancelled: boolean) => {
        if (finishEvent.pointerId !== pointerId) return
        if (!moved) {
          cleanup()
          finishEvent.preventDefault()
          return
        }

        if (cancelled) pendingWidth = startWidth
        flushPendingWidth()
        cleanup()

        if (!cancelled) {
          const finalWidth = Math.round(appliedWidth)
          const nextOpen = finalWidth > 0
          onOpenChange(nextOpen)
          if (nextOpen) onWidthCommit(finalWidth)
        }
        finishEvent.preventDefault()
      }

      const onPointerUp = (upEvent: globalThis.PointerEvent) => finish(upEvent, false)
      const onPointerCancel = (cancelEvent: globalThis.PointerEvent) => finish(cancelEvent, true)
      const onWindowBlur = () => {
        if (!moved) {
          cleanup()
          return
        }
        pendingWidth = startWidth
        flushPendingWidth()
        cleanup()
      }

      activeDragRef.current = { cleanup }
      window.addEventListener('pointermove', onPointerMove)
      window.addEventListener('pointerup', onPointerUp)
      window.addEventListener('pointercancel', onPointerCancel)
      window.addEventListener('blur', onWindowBlur)
    },
    [contentMinRatio, maxWidth, minWidth, onOpenChange, onWidthCommit, side, writeWidth],
  )

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      let nextWidth: number | null = null
      const currentWidth = liveWidthRef.current
      const step = event.shiftKey ? KEYBOARD_STEP_PX * 4 : KEYBOARD_STEP_PX

      switch (event.key) {
        case 'ArrowLeft':
          nextWidth = currentWidth + (side === 'left' ? -step : step)
          break
        case 'ArrowRight':
          nextWidth = currentWidth + (side === 'left' ? step : -step)
          break
        case 'Home':
          nextWidth = 0
          break
        case 'End':
          nextWidth = maxWidth
          break
        case 'Enter':
          nextWidth = open ? 0 : preferredWidth
          break
      }

      if (nextWidth === null) return
      event.preventDefault()
      commitKeyboardWidth(nextWidth)
    },
    [commitKeyboardWidth, maxWidth, open, preferredWidth, side],
  )

  const sidebarMaxPercent = `${(1 - contentMinRatio) * 100}%`
  const gridTemplateColumns =
    side === 'left'
      ? `min(var(--sidebar-track-width), ${sidebarMaxPercent}) 1px minmax(0, 1fr)`
      : `minmax(0, 1fr) 1px min(var(--sidebar-track-width), ${sidebarMaxPercent})`

  return (
    <div
      ref={rootRef}
      data-sidebar-split-pane=""
      data-side={side}
      className="grid h-full w-full min-w-0"
      style={
        {
          '--sidebar-track-width': `${open ? preferredWidth : 0}px`,
          gridTemplateAreas:
            side === 'left' ? '"sidebar handle content"' : '"content handle sidebar"',
          gridTemplateColumns,
        } as CSSProperties
      }
    >
      <div
        ref={sidebarRef}
        id="crowbar-sidebar-pane"
        data-sidebar-split-panel="sidebar"
        className="min-h-0 min-w-0 overflow-hidden"
        style={{ gridArea: 'sidebar' }}
      >
        {sidebar}
      </div>

      <div
        ref={handleRef}
        role="separator"
        tabIndex={0}
        aria-controls="crowbar-sidebar-pane"
        aria-label="Resize sidebar"
        aria-orientation="vertical"
        aria-valuemin={0}
        aria-valuemax={maxWidth}
        aria-valuenow={Math.round(open ? preferredWidth : 0)}
        data-slot="sidebar-resize-handle"
        data-testid="sidebar-resize-handle"
        onDoubleClick={() => commitKeyboardWidth(open ? 0 : preferredWidth)}
        onKeyDown={handleKeyDown}
        onPointerDown={handlePointerDown}
        className={cn(
          'relative z-10 h-full w-2 touch-none justify-self-center cursor-col-resize outline-none',
          'after:absolute after:inset-y-0 after:left-1/2 after:w-px after:-translate-x-1/2 after:transition-colors',
          'hover:after:bg-border/60 focus-visible:after:bg-border focus-visible:ring-1 focus-visible:ring-ring',
        )}
        style={{ gridArea: 'handle' }}
      />

      <div
        data-sidebar-split-panel="content"
        className="min-h-0 min-w-0 overflow-visible"
        style={{ gridArea: 'content' }}
      >
        {children}
      </div>
    </div>
  )
}
