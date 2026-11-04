import { type ReactNode, useEffect, useRef } from 'react'
import { dropdownItemVariants } from '@/components/ui/dropdown-styles'
import { cn } from '@/utils/cn'

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

interface MenuItemsListProps {
  items: MenuItem[]
  onItemSelect?: () => void
  className?: string
  itemClassName?: string
  focusIndex?: number
}

/** A dropdown's buttons; `focusIndex` counts selectable items only and is kept in view. */
export function MenuItemsList({
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
