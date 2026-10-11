import { CaretRightIcon } from '@phosphor-icons/react'
import type React from 'react'
import { TreeRow } from '@/components/ui/tree-row'
import { cn } from '@/lib/utils'

/** A visible row as the keyboard sees it. `expanded` is undefined for a leaf. */
interface TreeNavItem {
  key: string
  /** 1 for a top-level row. */
  level: number
  expanded?: boolean
}

type TreeProps = Omit<React.ComponentProps<'div'>, 'onSelect' | 'role'> & {
  /** Every visible row, top to bottom. */
  items: readonly TreeNavItem[]
  /** The row the keyboard is on, announced through `aria-activedescendant`. */
  activeKey: string | null
  onActiveKeyChange: (key: string) => void
  onExpandedChange: (key: string, expanded: boolean) => void
  onSelect: (key: string) => void
  /** The DOM id of `key`'s row. */
  itemId: (key: string) => string
}

function parentIndex(items: readonly TreeNavItem[], index: number): number {
  const level = items[index].level
  for (let i = index - 1; i >= 0; i--) if (items[i].level < level) return i
  return -1
}

/**
 * A single-select tree (WAI-ARIA tree pattern): focus stays on the tree, which moves an
 * active row through `aria-activedescendant`. Arrows move and expand/collapse, Home/End
 * jump, Enter and Space select.
 */
export function Tree({
  items,
  activeKey,
  onActiveKeyChange,
  onExpandedChange,
  onSelect,
  itemId,
  className,
  onFocus,
  onKeyDown,
  ...props
}: TreeProps) {
  const activeIndex = items.findIndex((item) => item.key === activeKey)

  function handleKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    onKeyDown?.(event)
    if (event.defaultPrevented || items.length === 0) return
    const index = Math.max(0, activeIndex)
    const current = items[index]
    const moveTo = (to: number) =>
      onActiveKeyChange(items[Math.min(items.length - 1, Math.max(0, to))].key)
    switch (event.key) {
      case 'ArrowDown':
        moveTo(activeIndex + 1)
        break
      case 'ArrowUp':
        moveTo(index - 1)
        break
      case 'Home':
        moveTo(0)
        break
      case 'End':
        moveTo(items.length - 1)
        break
      case 'ArrowRight':
        if (current.expanded === false) onExpandedChange(current.key, true)
        else if (current.expanded && items[index + 1]?.level > current.level) moveTo(index + 1)
        break
      case 'ArrowLeft':
        if (current.expanded) onExpandedChange(current.key, false)
        else if (parentIndex(items, index) >= 0) moveTo(parentIndex(items, index))
        break
      case 'Enter':
      case ' ':
        onSelect(current.key)
        break
      default:
        return
    }
    event.preventDefault()
  }

  return (
    <div
      role="tree"
      tabIndex={0}
      aria-activedescendant={activeIndex >= 0 ? itemId(items[activeIndex].key) : undefined}
      className={cn('group/tree flex flex-col outline-none', className)}
      onFocus={(event) => {
        onFocus?.(event)
        if (activeIndex < 0 && items.length > 0) onActiveKeyChange(items[0].key)
      }}
      onKeyDown={handleKeyDown}
      {...props}
    />
  )
}

type TreeItemProps = Omit<React.ComponentProps<'button'>, 'role'> & {
  level: number
  /** Undefined for a leaf: it draws no chevron and has no `aria-expanded`. */
  expanded?: boolean
  selected?: boolean
  /** The row the keyboard is on. */
  active?: boolean
  /** Shown but inert: never selected, never expanded. */
  inert?: boolean
  /** The chevron toggles without selecting. */
  onToggle?: () => void
  icon?: React.ReactNode
}

/** One row of a {@link Tree}: a chevron, an icon and its label, indented by level. */
export function TreeItem({
  level,
  expanded,
  selected = false,
  active = false,
  inert = false,
  onToggle,
  icon,
  className,
  children,
  ...props
}: TreeItemProps) {
  return (
    <TreeRow
      role="treeitem"
      tabIndex={-1}
      aria-level={level}
      aria-expanded={expanded}
      aria-selected={inert ? undefined : selected}
      aria-disabled={inert || undefined}
      data-active={active || undefined}
      active={selected}
      depth={level - 1}
      baseIndent={6}
      className={cn(
        'group-focus-within/tree:data-active:ring-1 group-focus-within/tree:data-active:ring-ring h-7 gap-1.5 px-1.5',
        inert && 'cursor-default text-muted-foreground opacity-60 hover:bg-transparent',
        className,
      )}
      {...props}
    >
      <span
        aria-hidden="true"
        data-slot="tree-item-chevron"
        className="flex size-3.5 shrink-0 items-center justify-center"
        onClick={(event) => {
          if (expanded === undefined || !onToggle) return
          event.stopPropagation()
          onToggle()
        }}
      >
        {expanded !== undefined && (
          <CaretRightIcon
            weight="bold"
            className={cn(
              'size-2.5 text-muted-foreground transition-transform duration-150',
              expanded && 'rotate-90',
            )}
          />
        )}
      </span>
      {icon}
      <span className="min-w-0 truncate">{children}</span>
    </TreeRow>
  )
}
