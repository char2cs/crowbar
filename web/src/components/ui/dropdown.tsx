import {
  type CSSProperties,
  type ReactNode,
  type RefObject,
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react'
import { dropdownSectionLabelVariants } from '@/components/ui/dropdown-styles'
import { Input } from '@/components/ui/input'
import { MenuItemsList, type MenuItem } from '@/components/ui/menu-items-list'
import { MenuPopover } from '@/components/ui/menu-popover'
import {
  type AnchorAlign,
  type AnchorSide,
  useDropdownPosition,
} from '@/components/ui/use-dropdown-position'
import { matchesSearchQuery } from '@/utils/search-match'
import { MagnifyingGlass as Search } from '@phosphor-icons/react'

export interface DropdownSection {
  id: string
  label?: string
  items: MenuItem[]
}

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

const matches = (query: string, item: MenuItem) =>
  !item.separator && matchesSearchQuery(query, [item.label])

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
  const [searchQuery, setSearchQuery] = useState('')
  const [focusIndex, setFocusIndex] = useState(-1)

  const isAnchorMode = 'anchorRef' in props && props.anchorRef != null
  const anchorRef = isAnchorMode ? (props as AnchorPositioning).anchorRef : null
  const anchorSide = isAnchorMode ? ((props as AnchorPositioning).anchorSide ?? 'bottom') : 'bottom'
  const anchorAlign = isAnchorMode ? ((props as AnchorPositioning).anchorAlign ?? 'start') : 'start'
  const point = !isAnchorMode ? (props as PointPositioning).point : null

  // Depend on the specific union arrays, not `props` (a fresh object each render), so the
  // callbacks below actually memoize.
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
    return searchQuery.trim() ? all.filter((item) => matches(searchQuery, item)) : all
  }, [getAllItems, searchQuery])

  const getFilteredSections = useCallback((): DropdownSection[] => {
    if (sectionsProp == null || !searchQuery.trim()) return sectionsProp ?? []
    const result: DropdownSection[] = []
    for (const section of sectionsProp) {
      const items = section.items.filter((item) => matches(searchQuery, item))
      if (items.length > 0) result.push({ ...section, items })
    }
    return result
  }, [sectionsProp, searchQuery])

  const { resolvedSide, isPositioned } = useDropdownPosition({
    isOpen,
    menuRef,
    anchorRef,
    anchorSide,
    anchorAlign,
    point,
    style,
    contentKey: searchQuery,
  })

  useEffect(() => {
    if (!isOpen) return

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
    document.addEventListener('mousedown', handleClickOutside)
    document.addEventListener('keydown', handleEscape, true)

    return () => {
      document.removeEventListener('mousedown', handleClickOutside)
      document.removeEventListener('keydown', handleEscape, true)
    }
  }, [isOpen, onClose, anchorRef])

  // A search belongs to one opening of the menu: closing it lets the query go.
  useEffect(() => {
    if (!isOpen) return
    if (searchable) requestAnimationFrame(() => searchRef.current?.focus())
    return () => {
      setSearchQuery('')
      setFocusIndex(-1)
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

  return (
    <MenuPopover
      isOpen={isOpen}
      menuRef={menuRef}
      portalContainer={portalContainer}
      className={className}
      style={{ visibility: isPositioned ? 'visible' : 'hidden', ...style }}
      animated={animated}
      side={resolvedSide}
      align={anchorAlign}
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
