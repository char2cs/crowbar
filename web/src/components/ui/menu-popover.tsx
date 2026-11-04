import { AnimatePresence, domAnimation, LazyMotion, m } from 'framer-motion'
import {
  type CSSProperties,
  type ReactNode,
  type RefObject,
  type WheelEvent as ReactWheelEvent,
} from 'react'
import { createPortal } from 'react-dom'
import { dropdownRootVariants } from '@/components/ui/dropdown-styles'
import type { AnchorAlign, AnchorSide } from '@/components/ui/use-dropdown-position'
import { cn } from '@/utils/cn'

function containScrollChain(event: ReactWheelEvent<HTMLDivElement>) {
  const root = event.currentTarget
  const deltaY = event.deltaY

  if (deltaY === 0) return

  let node = event.target instanceof HTMLElement ? event.target : null

  while (node) {
    const style = window.getComputedStyle(node)
    const canScrollY =
      (style.overflowY === 'auto' || style.overflowY === 'scroll') &&
      node.scrollHeight > node.clientHeight

    if (canScrollY) {
      const maxScrollTop = node.scrollHeight - node.clientHeight
      if ((deltaY < 0 && node.scrollTop > 0) || (deltaY > 0 && node.scrollTop < maxScrollTop)) {
        return
      }
    }

    if (node === root) break
    node = node.parentElement
  }

  event.preventDefault()
  event.stopPropagation()
}

const TRANSFORM_ORIGIN: Record<`${AnchorSide}-${AnchorAlign}`, string> = {
  'bottom-start': 'top left',
  'bottom-end': 'top right',
  'top-start': 'bottom left',
  'top-end': 'bottom right',
}

/** The dropdown's portalled surface, growing out of its anchor; wheel scrolling stays inside it. */
export function MenuPopover({
  isOpen,
  menuRef,
  children,
  className,
  portalContainer,
  style,
  animated,
  side,
  align,
}: {
  isOpen: boolean
  menuRef: RefObject<HTMLDivElement | null>
  children: ReactNode
  className?: string
  portalContainer?: Element | DocumentFragment | null
  style?: CSSProperties
  animated: boolean
  side: AnchorSide
  align: AnchorAlign
}) {
  if (typeof document === 'undefined') return null

  const node = isOpen ? (
    <m.div
      ref={menuRef}
      data-prevent-dialog-escape="true"
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onWheelCapture={containScrollChain}
      initial={animated ? { opacity: 0, scale: 0.98, y: side === 'top' ? 4 : -4 } : false}
      animate={{ opacity: 1, scale: 1, y: 0 }}
      exit={animated ? { opacity: 0, scale: 0.98, y: side === 'top' ? 4 : -4 } : { opacity: 1 }}
      transition={animated ? { duration: 0.12, ease: 'easeOut' } : { duration: 0 }}
      className={cn(dropdownRootVariants(), className)}
      style={{ transformOrigin: TRANSFORM_ORIGIN[`${side}-${align}`], ...style }}
    >
      {children}
    </m.div>
  ) : null

  return createPortal(
    <LazyMotion features={domAnimation}>
      <AnimatePresence>{node}</AnimatePresence>
    </LazyMotion>,
    portalContainer ?? document.body,
  )
}
