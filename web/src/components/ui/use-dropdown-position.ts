import {
  type CSSProperties,
  type RefObject,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'

export type AnchorSide = 'top' | 'bottom'
export type AnchorAlign = 'start' | 'end'

const VIEWPORT_PADDING = 8
const RESIZE_REPOSITION_THRESHOLD = 2

function getNumericMaxHeight(value: CSSProperties['maxHeight']) {
  if (typeof value === 'number' && Number.isFinite(value)) {
    return value
  }
  if (typeof value === 'string') {
    const match = value.trim().match(/^(\d+(?:\.\d+)?)px$/)
    if (match) {
      return Number.parseFloat(match[1])
    }
  }
  return null
}

function getViewportBounds() {
  const vv = window.visualViewport
  if (!vv || !Number.isFinite(vv.width) || !Number.isFinite(vv.height)) {
    return { left: 0, top: 0, width: window.innerWidth, height: window.innerHeight }
  }
  return {
    left: Number.isFinite(vv.offsetLeft) ? vv.offsetLeft : 0,
    top: Number.isFinite(vv.offsetTop) ? vv.offsetTop : 0,
    width: vv.width,
    height: vv.height,
  }
}

/**
 * Places an open menu against its anchor (or a point), flipping to the side with more room
 * and clamping to the visual viewport; it follows resizes, scrolls and its own size changes.
 * The menu's width is locked at its first measure so filtering does not make it jump.
 */
export function useDropdownPosition({
  isOpen,
  menuRef,
  anchorRef,
  anchorSide,
  anchorAlign,
  point,
  style,
  contentKey,
}: {
  isOpen: boolean
  menuRef: RefObject<HTMLDivElement | null>
  anchorRef: RefObject<HTMLElement | null> | null
  anchorSide: AnchorSide
  anchorAlign: AnchorAlign
  point: { x: number; y: number } | null
  style?: CSSProperties
  /** Repositions before paint whenever it changes: the content it measures did. */
  contentKey: unknown
}) {
  const lockedWidthRef = useRef<number | null>(null)
  const lastMenuSizeRef = useRef<{ width: number; height: number } | null>(null)
  const [resolvedSide, setResolvedSide] = useState<AnchorSide>('bottom')
  const [isPositioned, setIsPositioned] = useState(false)

  const positionMenu = useCallback(() => {
    const menu = menuRef.current
    if (!menu) return

    const vp = getViewportBounds()
    const userMaxHeight = getNumericMaxHeight(style?.maxHeight)
    const hasExplicitWidth = style?.width != null

    const applyMaxHeight = (height: number) => {
      const nextHeight = userMaxHeight == null ? height : Math.min(height, userMaxHeight)
      menu.style.maxHeight = `${nextHeight}px`
    }

    const applyLockedWidth = () => {
      if (hasExplicitWidth) return

      if (lockedWidthRef.current == null) {
        lockedWidthRef.current = menu.getBoundingClientRect().width
      }

      if (lockedWidthRef.current != null) {
        menu.style.width = `${lockedWidthRef.current}px`
      }
    }

    let x: number
    let y: number
    let finalSide: AnchorSide = 'bottom'

    if (anchorRef?.current) {
      const anchorRect = anchorRef.current.getBoundingClientRect()
      const viewportMaxHeight = Math.max(120, vp.height - VIEWPORT_PADDING * 2)
      const spaceBelow = vp.top + vp.height - anchorRect.bottom - VIEWPORT_PADDING
      const spaceAbove = anchorRect.top - vp.top - VIEWPORT_PADDING

      if (anchorSide === 'bottom') {
        finalSide = spaceBelow >= spaceAbove ? 'bottom' : 'top'
      } else {
        finalSide = spaceAbove >= spaceBelow ? 'top' : 'bottom'
      }

      const availableHeight = finalSide === 'bottom' ? spaceBelow : spaceAbove
      applyMaxHeight(Math.max(120, Math.min(viewportMaxHeight, availableHeight)))
      applyLockedWidth()

      const menuRect = menu.getBoundingClientRect()

      if (anchorAlign === 'end') {
        x = anchorRect.right - menuRect.width
      } else {
        x = anchorRect.left
      }

      if (finalSide === 'bottom') {
        if (menuRect.height <= spaceBelow || spaceBelow >= spaceAbove) {
          y = anchorRect.bottom + 6
          finalSide = 'bottom'
        } else {
          y = anchorRect.top - menuRect.height - 6
          finalSide = 'top'
        }
      } else {
        if (menuRect.height <= spaceAbove || spaceAbove >= spaceBelow) {
          y = anchorRect.top - menuRect.height - 6
          finalSide = 'top'
        } else {
          y = anchorRect.bottom + 6
          finalSide = 'bottom'
        }
      }
    } else if (point) {
      const maxH = Math.max(120, vp.height - VIEWPORT_PADDING * 2)
      applyMaxHeight(maxH)
      applyLockedWidth()

      const menuRect = menu.getBoundingClientRect()
      x = point.x
      y = point.y

      if (x + menuRect.width > vp.left + vp.width - VIEWPORT_PADDING) {
        x = point.x - menuRect.width
      }
      if (y + menuRect.height > vp.top + vp.height - VIEWPORT_PADDING) {
        y = point.y - menuRect.height
      }
    } else {
      return
    }

    const menuRect = menu.getBoundingClientRect()

    const minX = vp.left + VIEWPORT_PADDING
    const maxX = vp.left + vp.width - menuRect.width - VIEWPORT_PADDING
    const minY = vp.top + VIEWPORT_PADDING
    const maxY = vp.top + vp.height - menuRect.height - VIEWPORT_PADDING

    x = Math.max(minX, Math.min(x, maxX))
    y = Math.max(minY, Math.min(y, maxY))

    menu.style.left = `${Math.round(x)}px`
    menu.style.top = `${Math.round(y)}px`
    setResolvedSide(finalSide)
    setIsPositioned(true)
  }, [anchorRef, anchorSide, anchorAlign, point, style?.maxHeight, style?.width, menuRef])

  useLayoutEffect(() => {
    if (!isOpen) return
    positionMenu()
  }, [isOpen, positionMenu, contentKey])

  // The open menu's cleanup, not a reaction to `isOpen` going false: the
  // measurements below belong to one opening of the menu, so releasing them
  // when that opening ends is the same trigger expressed as ownership — and it
  // now also covers unmounting while open.
  useEffect(() => {
    if (!isOpen) return
    const menu = menuRef.current
    return () => {
      lockedWidthRef.current = null
      lastMenuSizeRef.current = null
      setIsPositioned(false)
      if (menu && style?.width == null) {
        menu.style.width = ''
      }
    }
  }, [isOpen, style?.width, menuRef])

  useEffect(() => {
    if (!isOpen) return

    const resizeObserver = new ResizeObserver((entries) => {
      const entry = entries[0]
      if (!entry) return

      const { width, height } = entry.contentRect
      const previousSize = lastMenuSizeRef.current
      lastMenuSizeRef.current = { width, height }

      if (!previousSize) {
        positionMenu()
        return
      }

      const widthDelta = Math.abs(width - previousSize.width)
      const heightDelta = Math.abs(height - previousSize.height)

      if (widthDelta < RESIZE_REPOSITION_THRESHOLD && heightDelta < RESIZE_REPOSITION_THRESHOLD) {
        return
      }

      positionMenu()
    })
    if (menuRef.current) resizeObserver.observe(menuRef.current)
    window.addEventListener('resize', positionMenu)
    window.addEventListener('scroll', positionMenu, true)
    window.visualViewport?.addEventListener('resize', positionMenu)
    window.visualViewport?.addEventListener('scroll', positionMenu)

    return () => {
      resizeObserver.disconnect()
      window.removeEventListener('resize', positionMenu)
      window.removeEventListener('scroll', positionMenu, true)
      window.visualViewport?.removeEventListener('resize', positionMenu)
      window.visualViewport?.removeEventListener('scroll', positionMenu)
    }
  }, [isOpen, positionMenu, menuRef])

  return { resolvedSide, isPositioned }
}
