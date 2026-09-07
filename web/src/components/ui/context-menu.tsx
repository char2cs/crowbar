import * as React from 'react'
import { Menu as MenuPrimitive } from '@base-ui/react/menu'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'

import { cn } from '@/lib/utils'
import { Menu } from '@tauri-apps/api/menu'
import type { MenuItemOptions, SubmenuOptions, PredefinedMenuItemOptions } from '@tauri-apps/api/menu'
import { LogicalPosition } from '@tauri-apps/api/dpi'
import { isTauri } from '@/lib/crowbar-bridge'

// ── Imperative context-menu API (opened programmatically, not via a trigger) ──

export interface ContextMenuItem {
  id: string
  label: string
  icon?: React.ReactNode
  onClick: () => void
  separator?: boolean
  disabled?: boolean
  shortcut?: string
  keybinding?: React.ReactNode
  className?: string
  items?: ContextMenuItem[]
  /** When false, clicking this item will not auto-close the menu. Defaults to true. */
  closeOnClick?: boolean
}

export interface ContextMenuRootProps {
  isOpen: boolean
  position: { x: number; y: number }
  items: ContextMenuItem[]
  onClose: () => void
  className?: string
  /** Optional content rendered below the last menu item (e.g. inline error panel). */
  footer?: React.ReactNode
}

// ── Native menu (Tauri) ──────────────────────────────────────────────────────

type NativeMenuEntry = MenuItemOptions | SubmenuOptions | PredefinedMenuItemOptions

function toNativeMenuEntries(items: ContextMenuItem[]): NativeMenuEntry[] {
  return items.map((item): NativeMenuEntry => {
    if (item.separator) {
      return { item: 'Separator' }
    }
    if (item.items && item.items.length > 0) {
      return {
        text: item.label,
        enabled: !item.disabled,
        items: toNativeMenuEntries(item.items),
      }
    }
    return {
      id: item.id,
      text: item.label,
      enabled: !item.disabled,
      accelerator: item.shortcut,
      action: () => item.onClick(),
    }
  })
}

/** Pops up the OS's own context menu. Always closes the underlying native
 * resource handle when the popup dismisses, whether an item was picked or
 * the popup was closed with no selection. */
export async function showNativeContextMenu(
  items: ContextMenuItem[],
  position: { x: number; y: number },
): Promise<void> {
  const menu = await Menu.new({ items: toNativeMenuEntries(items) })
  try {
    await menu.popup(new LogicalPosition(position.x, position.y))
  } finally {
    await menu.close()
  }
}

function ImperativeContextMenu({
  isOpen,
  position,
  items,
  onClose,
  className,
  footer,
}: ContextMenuRootProps) {
  const onCloseRef = useRef(onClose)
  useEffect(() => {
    onCloseRef.current = onClose
  }, [onClose])

  const virtualAnchor = useMemo(
    () => ({
      getBoundingClientRect: (): DOMRect =>
        ({
          x: position.x,
          y: position.y,
          top: position.y,
          left: position.x,
          right: position.x,
          bottom: position.y,
          width: 0,
          height: 0,
          toJSON() {
            return this
          },
        }) as DOMRect,
    }),
    [position.x, position.y],
  )

  return (
    <MenuPrimitive.Root
      modal={false}
      open={isOpen}
      onOpenChange={(open) => {
        if (!open) onCloseRef.current()
      }}
    >
      <MenuPrimitive.Portal>
        <MenuPrimitive.Positioner
          anchor={virtualAnchor}
          side="bottom"
          align="start"
          sideOffset={0}
          className="z-[10040]"
        >
          <MenuPrimitive.Popup
            className={cn(
              "relative flex not-[class*='w-']:min-w-[180px] origin-(--transform-origin) rounded-lg border bg-popover not-dark:bg-clip-padding shadow-lg/5 outline-none before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-lg)-1px)] before:shadow-[0_1px_--theme(--color-black/4%)] focus:outline-none dark:before:shadow-[0_-1px_--theme(--color-white/6%)]",
              className,
            )}
          >
            <div className="max-h-(--available-height) w-full overflow-y-auto p-1">
              {items.map((item) =>
                item.separator ? (
                  <MenuPrimitive.Separator key={item.id} className="mx-2 my-1 h-px bg-border" />
                ) : (
                  <MenuPrimitive.Item
                    key={item.id}
                    disabled={item.disabled}
                    className={cn(
                      "flex min-h-8 cursor-default select-none items-center gap-2 rounded-sm px-2 py-1 text-base text-foreground outline-none data-disabled:pointer-events-none data-highlighted:bg-accent data-highlighted:text-accent-foreground data-disabled:opacity-64 sm:min-h-7 sm:text-sm [&>svg:not([class*='opacity-'])]:opacity-80 [&>svg:not([class*='size-'])]:size-4.5 sm:[&>svg:not([class*='size-'])]:size-4 [&>svg]:pointer-events-none [&>svg]:-mx-0.5 [&>svg]:shrink-0",
                      item.className,
                    )}
                    onClick={() => {
                      item.onClick()
                      if (item.closeOnClick !== false) onCloseRef.current()
                    }}
                  >
                    {item.icon}
                    <span className="flex-1">{item.label}</span>
                    {item.shortcut && (
                      <kbd className="ms-auto font-medium font-sans text-muted-foreground/72 text-xs tracking-widest">
                        {item.shortcut}
                      </kbd>
                    )}
                  </MenuPrimitive.Item>
                ),
              )}
              {footer}
            </div>
          </MenuPrimitive.Popup>
        </MenuPrimitive.Positioner>
      </MenuPrimitive.Portal>
    </MenuPrimitive.Root>
  )
}

interface ContextMenuState<T = unknown> {
  isOpen: boolean
  position: { x: number; y: number }
  data: T | null
}

export function useContextMenu<T = unknown>() {
  const [state, setState] = useState<ContextMenuState<T>>({
    isOpen: false,
    position: { x: 0, y: 0 },
    data: null,
  })

  const open = useCallback((e: React.MouseEvent, data?: T) => {
    e.preventDefault()
    e.stopPropagation()
    setState({ isOpen: true, position: { x: e.clientX, y: e.clientY }, data: data ?? null })
  }, [])

  const openAt = useCallback((position: { x: number; y: number }, data?: T) => {
    setState({ isOpen: true, position, data: data ?? null })
  }, [])

  const close = useCallback(() => {
    setState({ isOpen: false, position: { x: 0, y: 0 }, data: null })
  }, [])

  return { ...state, open, openAt, close }
}

// `ContextMenu` is native-first: in Tauri it pops the OS's own menu and
// renders nothing; outside Tauri (plain-browser `bun run dev`) it falls back
// to ImperativeContextMenu, the rendered popup this app used everywhere
// before native menus existed.
function ContextMenuHost({ isOpen, position, items, onClose, className, footer }: ContextMenuRootProps) {
  const onCloseRef = useRef(onClose)
  useEffect(() => {
    onCloseRef.current = onClose
  }, [onClose])

  useEffect(() => {
    if (!isTauri() || !isOpen) return
    let cancelled = false
    void (async () => {
      try {
        await showNativeContextMenu(items, position)
      } catch (error) {
        console.error('Failed to show native context menu:', error)
      } finally {
        if (!cancelled) onCloseRef.current()
      }
    })()
    return () => {
      cancelled = true
    }
    // items/position are read once, at the moment isOpen flips true — an
    // already-open native popup can't be updated mid-display anyway.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen])

  if (isTauri()) return null

  return (
    <ImperativeContextMenu
      isOpen={isOpen}
      position={position}
      items={items}
      onClose={onClose}
      className={className}
      footer={footer}
    />
  )
}

export { ContextMenuHost as ContextMenu }
