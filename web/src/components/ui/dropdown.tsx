import { cva } from 'class-variance-authority'
import { AnimatePresence, domAnimation, LazyMotion, m, type Transition } from 'framer-motion'
import {
  type CSSProperties,
  type ReactNode,
  type RefObject,
  type WheelEvent as ReactWheelEvent,
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from 'react'
import { createPortal } from 'react-dom'
import { buttonVariants } from '@/components/ui/button-variants'
import { Input } from '@/components/ui/input'
import { cn } from '@/utils/cn'
import { matchesSearchQuery } from '@/utils/search-match'
import { MagnifyingGlass as Search } from '@phosphor-icons/react'

// Trigger styling, with a variant axis rather than one hard-coded look.
//
// `ghost` exists for triggers that sit ON a surface rather than beside content — a
// filled trigger there reads as a stray button someone left lying on the page.
//
// A ghost trigger has NO visible box, so its optical left edge is its ICON, not its
// border. A caller aligning one to a text column must pull back the FULL chrome the
// icon sits behind: px-2 (8px) PLUS the button's 1px transparent border, which renders
// invisible but still takes space. That is 9px, not 8 — get it wrong and the trigger
// looks a pixel out while every measurement insists it is aligned.
//
// Each variant is cn()'d so the trigger's own `px-2` beats buttonVariants' padding.
// Do NOT hoist the shared classes into cva's base string: cva concatenates without
// tailwind-merge, so the base would be emitted BEFORE the button classes and lose to
// them — the trigger would silently render at the button's padding instead of its own.
const TRIGGER_SHARED = 'min-w-0 gap-1 rounded-lg px-2 text-muted-foreground'

const TRANSFORM_ORIGIN_MAP: Record<string, string> = {
  'bottom-start': 'top left',
  'bottom-end': 'top right',
  'top-start': 'bottom left',
  'top-end': 'bottom right',
}

const dropdownTriggerVariants = cva('', {
  variants: {
    variant: {
      default: cn(buttonVariants({ variant: 'default' }), TRIGGER_SHARED),
      ghost: cn(buttonVariants({ variant: 'ghost' }), TRIGGER_SHARED),
    },
  },
  defaultVariants: {
    variant: 'default',
  },
})

export type DropdownTriggerVariant = 'default' | 'ghost'

const dropdownRootVariants = cva(
  'pointer-events-auto fixed z-[10040] min-w-[240px] max-w-[min(480px,calc(100vw-16px))] select-none overflow-y-auto rounded-xl border border-border bg-card/95 p-1 shadow-[0_14px_30px_-24px_rgba(0,0,0,0.45)] backdrop-blur-sm [overscroll-behavior:contain]',
)

const dropdownItemVariants = cva(
  'ui-font ui-text-sm flex w-full items-center justify-between gap-3 whitespace-nowrap rounded-lg px-2.5 py-1.5 text-left text-foreground transition-colors',
  {
    variants: {
      disabled: {
        true: 'cursor-not-allowed opacity-50',
        false: 'cursor-pointer hover:bg-muted',
      },
      focused: {
        true: 'bg-muted',
        false: '',
      },
    },
    defaultVariants: {
      disabled: false,
      focused: false,
    },
  },
)

const dropdownSectionLabelVariants = cva('ui-font ui-text-sm px-2.5 py-1 text-muted-foreground')

const DROPDOWN_ITEM_BASE = dropdownItemVariants()

// react-doctor-disable-next-line only-export-components -- className builder tied to this file's cva variants (dropdownTriggerVariants); extracting it to a satellite module would fragment the dropdown's styling layer for a dev-only Fast-Refresh benefit.
export function dropdownTriggerClassName(
  className?: string,
  variant: DropdownTriggerVariant = 'default',
) {
  return cn(dropdownTriggerVariants({ variant }), className)
}

// react-doctor-disable-next-line only-export-components -- className builder tied to this file's cva variants (DROPDOWN_ITEM_BASE); co-located with the dropdown styling it parametrizes.
export function dropdownItemClassName(className?: string) {
  return cn(DROPDOWN_ITEM_BASE, className)
}

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

export interface MenuItem {
  id: string
  label: string
  icon?: ReactNode
  onClick: () => void
  disabled?: boolean
  separator?: boolean
  keybinding?: ReactNode
  className?: string
}

interface MenuPopoverProps {
  isOpen: boolean
  menuRef: RefObject<HTMLDivElement | null>
  children: ReactNode
  className?: string
  portalContainer?: Element | DocumentFragment | null
  style?: CSSProperties
  animated?: boolean
  initial?: { opacity: number; scale: number; y?: number }
  animate?: { opacity: number; scale: number; y?: number }
  exit?: { opacity: number; scale: number; y?: number }
  transition?: Transition
}

function MenuPopover({
  isOpen,
  menuRef,
  children,
  className,
  portalContainer,
  style,
  animated = true,
  initial = { opacity: 0, scale: 0.95 },
  animate = { opacity: 1, scale: 1 },
  exit = { opacity: 0, scale: 0.95 },
  transition = { duration: 0.12, ease: 'easeOut' as const },
}: MenuPopoverProps) {
  if (typeof document === 'undefined') return null

  const node = isOpen ? (
    <m.div
      ref={menuRef}
      data-prevent-dialog-escape="true"
      onMouseDown={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onWheelCapture={containScrollChain}
      initial={animated ? initial : false}
      animate={animated ? animate : { opacity: 1, scale: 1, y: 0 }}
      exit={animated ? exit : { opacity: 1, scale: 1, y: 0 }}
      transition={animated ? transition : { duration: 0 }}
      className={cn(dropdownRootVariants(), className)}
      style={style}
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

interface MenuItemsListProps {
  items: MenuItem[]
  onItemSelect?: () => void
  className?: string
  itemClassName?: string
  focusIndex?: number
}

function MenuItemsList({
  items,
  onItemSelect,
  className,
  itemClassName,
  focusIndex = -1,
}: MenuItemsListProps) {
  const itemRefs = useRef<(HTMLButtonElement | null)[]>([])

  useEffect(() => {
    if (focusIndex >= 0 && itemRefs.current[focusIndex]) {
      itemRefs.current[focusIndex]?.scrollIntoView({ block: 'nearest' })
    }
  }, [focusIndex])

  let selectableIdx = -1

  return (
    <div className={className}>
      {items.map((item) => {
        if (item.separator) {
          return <div key={item.id} className="my-0.5 border-border/70 border-t" />
        }

        selectableIdx++
        const isFocused = selectableIdx === focusIndex

        return (
          <button
            key={item.id}
            ref={(el) => {
              if (!item.disabled) {
                itemRefs.current[selectableIdx] = el
              }
            }}
            type="button"
            role="menuitem"
            onClick={() => {
              if (item.disabled) return
              item.onClick()
              onItemSelect?.()
            }}
            disabled={item.disabled}
            className={cn(
              dropdownItemVariants({
                disabled: item.disabled,
                focused: isFocused,
              }),
              itemClassName,
              item.className,
            )}
          >
            {item.icon && <span className="size-3 shrink-0">{item.icon}</span>}
            <span className="min-w-0 flex-1 truncate whitespace-nowrap">{item.label}</span>
            {item.keybinding && (
              <span className="ui-text-sm ml-8 shrink-0 whitespace-nowrap text-muted-foreground">
                {item.keybinding}
              </span>
            )}
          </button>
        )
      })}
    </div>
  )
}

export interface DropdownSection {
  id: string
  label?: string
  items: MenuItem[]
}

type AnchorSide = 'top' | 'bottom'
type AnchorAlign = 'start' | 'end'

interface DropdownBaseProps {
  isOpen: boolean
  onClose: () => void
  className?: string
  menuClassName?: string
  style?: CSSProperties
  portalContainer?: Element | DocumentFragment | null
  closeOnSelect?: boolean
  animated?: boolean
}

interface AnchorPositioning {
  anchorRef: RefObject<HTMLElement | null>
  anchorSide?: AnchorSide
  anchorAlign?: AnchorAlign
  point?: never
}

interface PointPositioning {
  point: { x: number; y: number }
  anchorRef?: never
  anchorSide?: never
  anchorAlign?: never
}

type PositioningProps = AnchorPositioning | PointPositioning

interface ItemsContent {
  items: MenuItem[]
  sections?: never
  children?: never
  searchable?: boolean
  searchPlaceholder?: string
}

interface SectionsContent {
  sections: DropdownSection[]
  items?: never
  children?: never
  searchable?: boolean
  searchPlaceholder?: string
}

interface ChildrenContent {
  children: ReactNode
  items?: never
  sections?: never
  searchable?: never
  searchPlaceholder?: never
}

type ContentProps = ItemsContent | SectionsContent | ChildrenContent

export type DropdownProps = DropdownBaseProps & PositioningProps & ContentProps

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

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive primitive — the Dropdown owns keyboard traversal, portal positioning, search filtering and outside-click as one interacting unit; extracting pieces would spread that state across files.
export function Dropdown(props: DropdownProps) {
  const {
    isOpen,
    onClose,
    className,
    menuClassName,
    style,
    searchable,
    searchPlaceholder,
    portalContainer,
    closeOnSelect = true,
    animated = true,
  } = props

  const menuRef = useRef<HTMLDivElement>(null)
  const searchRef = useRef<HTMLInputElement>(null)
  const lockedWidthRef = useRef<number | null>(null)
  const lastMenuSizeRef = useRef<{ width: number; height: number } | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [focusIndex, setFocusIndex] = useState(-1)
  const [resolvedSide, setResolvedSide] = useState<AnchorSide>('bottom')
  const [isPositioned, setIsPositioned] = useState(false)

  const isAnchorMode = 'anchorRef' in props && props.anchorRef != null
  const anchorRef = isAnchorMode ? (props as AnchorPositioning).anchorRef : null
  const anchorSide = isAnchorMode ? ((props as AnchorPositioning).anchorSide ?? 'bottom') : 'bottom'
  const anchorAlign = isAnchorMode ? ((props as AnchorPositioning).anchorAlign ?? 'start') : 'start'
  const point = !isAnchorMode ? (props as PointPositioning).point : null

  // Pull the union members out once. Depending on the whole `props` object
  // rebuilt these callbacks on every render (props is a fresh object each
  // time), which defeated the useCallback entirely; the specific arrays are
  // what the bodies actually read.
  const itemsProp = 'items' in props ? props.items : undefined
  const sectionsProp = 'sections' in props ? props.sections : undefined
  const hasItems = itemsProp != null
  const hasSections = sectionsProp != null
  const hasChildren = 'children' in props && props.children != null

  const getAllItems = useCallback((): MenuItem[] => {
    if (itemsProp != null) return itemsProp
    if (sectionsProp != null) return sectionsProp.flatMap((s) => s.items)
    return []
  }, [itemsProp, sectionsProp])

  const getFilteredItems = useCallback((): MenuItem[] => {
    const all = getAllItems()
    if (!searchQuery.trim()) return all
    return all.filter((item) => !item.separator && matchesSearchQuery(searchQuery, [item.label]))
  }, [getAllItems, searchQuery])

  const getFilteredSections = useCallback((): DropdownSection[] => {
    if (sectionsProp == null) return []
    if (!searchQuery.trim()) return sectionsProp
    const result: DropdownSection[] = []
    for (const section of sectionsProp) {
      const items = section.items.filter(
        (item) => !item.separator && matchesSearchQuery(searchQuery, [item.label]),
      )
      if (items.length > 0) result.push({ ...section, items })
    }
    return result
  }, [sectionsProp, searchQuery])

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
  }, [anchorRef, anchorSide, anchorAlign, point, style?.maxHeight, style?.width])

  useLayoutEffect(() => {
    if (!isOpen) return
    positionMenu()
  }, [isOpen, positionMenu, searchQuery])

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
  }, [isOpen, style?.width])

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

    const handleClickOutside = (e: MouseEvent) => {
      const target = e.target as Node
      if (menuRef.current?.contains(target)) return
      if (anchorRef?.current?.contains(target)) return
      onClose()
    }

    const handleEscape = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.preventDefault()
        e.stopPropagation()
        e.stopImmediatePropagation()
        onClose()
      }
    }

    window.addEventListener('resize', positionMenu)
    window.addEventListener('scroll', positionMenu, true)
    window.visualViewport?.addEventListener('resize', positionMenu)
    window.visualViewport?.addEventListener('scroll', positionMenu)
    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleEscape, true)

    return () => {
      resizeObserver.disconnect()
      window.removeEventListener('resize', positionMenu)
      window.removeEventListener('scroll', positionMenu, true)
      window.visualViewport?.removeEventListener('resize', positionMenu)
      window.visualViewport?.removeEventListener('scroll', positionMenu)
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleEscape, true)
    }
  }, [isOpen, onClose, positionMenu, anchorRef])

  useEffect(() => {
    if (isOpen) {
      setSearchQuery('')
      setFocusIndex(-1)
      if (searchable) {
        requestAnimationFrame(() => searchRef.current?.focus())
      }
    }
  }, [isOpen, searchable])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const items = getFilteredItems().filter((item) => !item.separator && !item.disabled)
      if (items.length === 0) return

      switch (e.key) {
        case 'ArrowDown': {
          e.preventDefault()
          setFocusIndex((prev) => (prev + 1) % items.length)
          break
        }
        case 'ArrowUp': {
          e.preventDefault()
          setFocusIndex((prev) => (prev <= 0 ? items.length - 1 : prev - 1))
          break
        }
        case 'Home': {
          e.preventDefault()
          setFocusIndex(0)
          break
        }
        case 'End': {
          e.preventDefault()
          setFocusIndex(items.length - 1)
          break
        }
        case 'Enter': {
          e.preventDefault()
          if (focusIndex >= 0 && focusIndex < items.length) {
            items[focusIndex].onClick()
            if (closeOnSelect) {
              onClose()
            }
          }
          break
        }
      }
    },
    [closeOnSelect, getFilteredItems, focusIndex, onClose],
  )

  if (typeof document === 'undefined') return null

  const transformOrigin =
    TRANSFORM_ORIGIN_MAP[`${resolvedSide}-${anchorAlign}`] ?? (point ? 'top left' : 'top left')

  return (
    <MenuPopover
      isOpen={isOpen}
      menuRef={menuRef}
      portalContainer={portalContainer}
      className={className}
      style={{ transformOrigin, visibility: isPositioned ? 'visible' : 'hidden', ...style }}
      animated={animated}
      initial={{ opacity: 0, scale: 0.98, y: resolvedSide === 'top' ? 4 : -4 }}
      animate={{ opacity: 1, scale: 1, y: 0 }}
      exit={{ opacity: 0, scale: 0.98, y: resolvedSide === 'top' ? 4 : -4 }}
      transition={{ duration: 0.12, ease: 'easeOut' }}
    >
      <div role="menu" className={menuClassName} onKeyDown={handleKeyDown}>
        {searchable && (
          <div className="border-border/60 border-b px-1.5 pb-1.5 pt-0.5">
            <Input
              ref={searchRef}
              type="text"
              placeholder={searchPlaceholder ?? 'Search...'}
              value={searchQuery}
              onChange={(e) => {
                setSearchQuery(e.target.value)
                setFocusIndex(-1)
              }}
              leftIcon={Search}
              className="w-full"
            />
          </div>
        )}
        {hasChildren && (props as ChildrenContent).children}
        {hasItems && (
          <MenuItemsList
            items={getFilteredItems()}
            focusIndex={focusIndex}
            onItemSelect={closeOnSelect ? onClose : undefined}
          />
        )}
        {hasSections &&
          getFilteredSections().map((section, sectionIdx) => (
            <div key={section.id}>
              {sectionIdx > 0 && <div className="my-0.5 border-border/70 border-t" />}
              {section.label && (
                <div className={dropdownSectionLabelVariants()}>{section.label}</div>
              )}
              <MenuItemsList
                items={section.items}
                onItemSelect={closeOnSelect ? onClose : undefined}
              />
            </div>
          ))}
      </div>
    </MenuPopover>
  )
}
