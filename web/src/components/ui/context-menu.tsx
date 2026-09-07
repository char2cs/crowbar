import * as React from 'react'
import { Menu as MenuPrimitive } from '@base-ui/react/menu'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ChevronRightIcon } from 'lucide-react'

import { cn } from '@/lib/utils'
import { isTauri, showNativeContextMenu } from '@/lib/crowbar-bridge'

// ── Imperative context-menu API (opened programmatically, not via a trigger) ──

export interface ContextMenuItem {
  id: string
  label: string
  /** Fallback-only: the non-Tauri rendered popup draws this icon. The native OS menu never shows per-item icons, by design, and ignores this entirely. */
  icon?: React.ReactNode
  onClick: () => void
  separator?: boolean
  disabled?: boolean
  /**
   * Displayed verbatim as text in the fallback popup, but on the native path
   * (Tauri) this is passed straight through as the native menu's
   * `accelerator` string, which the underlying `muda` crate PARSES — it must
   * be valid Tauri accelerator syntax (e.g. `"CmdOrCtrl+R"`), not a display
   * string like `"⌘R"`. An invalid value fails the whole native `Menu.new()`
   * call, which falls back to the rendered popup — but the accelerator
   * display is then lost for that click.
   */
  shortcut?: string
  /** Fallback-only: rendered next to the label in the non-Tauri popup. Being a `React.ReactNode` (not a string), this can never become a native `accelerator` — use `shortcut` for that instead. The native OS menu ignores this entirely. */
  keybinding?: React.ReactNode
  /** Fallback-only: applied as a class on the rendered menu item. The native OS menu has no per-item styling and ignores this entirely. */
  className?: string
  items?: ContextMenuItem[]
  /** When false, clicking this item will not auto-close the menu. Defaults to true. Fallback-only: the native OS menu always closes itself when an item is chosen. */
  closeOnClick?: boolean
}

export interface ContextMenuRootProps {
  isOpen: boolean
  position: { x: number; y: number }
  items: ContextMenuItem[]
  onClose: () => void
  className?: string
  /** Optional content rendered below the last menu item (e.g. inline error panel). Fallback-only — there is no native-menu equivalent, so this is ignored entirely when a native popup is shown. */
  footer?: React.ReactNode
}

const menuItemClass =
  "flex min-h-8 cursor-default select-none items-center gap-2 rounded-sm px-2 py-1 text-base text-foreground outline-none data-disabled:pointer-events-none data-highlighted:bg-accent data-highlighted:text-accent-foreground data-disabled:opacity-64 sm:min-h-7 sm:text-sm [&>svg:not([class*='opacity-'])]:opacity-80 [&>svg:not([class*='size-'])]:size-4.5 sm:[&>svg:not([class*='size-'])]:size-4 [&>svg]:pointer-events-none [&>svg]:-mx-0.5 [&>svg]:shrink-0"

const popupClass =
  "relative flex not-[class*='w-']:min-w-[180px] origin-(--transform-origin) rounded-lg border bg-popover not-dark:bg-clip-padding shadow-lg/5 outline-none before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-lg)-1px)] before:shadow-[0_1px_--theme(--color-black/4%)] focus:outline-none dark:before:shadow-[0_-1px_--theme(--color-white/6%)]"

function renderMenuItems(items: ContextMenuItem[], onCloseRef: React.RefObject<() => void>) {
  return items.map((item) => {
    if (item.separator) {
      return <MenuPrimitive.Separator key={item.id} className="mx-2 my-1 h-px bg-border" />
    }
    if (item.items && item.items.length > 0) {
      return (
        <MenuPrimitive.SubmenuRoot key={item.id}>
          <MenuPrimitive.SubmenuTrigger
            disabled={item.disabled}
            className={cn(menuItemClass, item.className)}
          >
            <span className="flex-1">{item.label}</span>
            <ChevronRightIcon className="ml-auto size-4 opacity-60" />
          </MenuPrimitive.SubmenuTrigger>
          <MenuPrimitive.Portal>
            <MenuPrimitive.Positioner
              side="right"
              align="start"
              sideOffset={-4}
              className="z-[10040]"
            >
              <MenuPrimitive.Popup className={popupClass}>
                <div className="max-h-(--available-height) w-full overflow-y-auto p-1">
                  {renderMenuItems(item.items, onCloseRef)}
                </div>
              </MenuPrimitive.Popup>
            </MenuPrimitive.Positioner>
          </MenuPrimitive.Portal>
        </MenuPrimitive.SubmenuRoot>
      )
    }
    return (
      <MenuPrimitive.Item
        key={item.id}
        disabled={item.disabled}
        className={cn(menuItemClass, item.className)}
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
    )
  })
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
          <MenuPrimitive.Popup className={cn(popupClass, className)}>
            <div className="max-h-(--available-height) w-full overflow-y-auto p-1">
              {renderMenuItems(items, onCloseRef)}
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
function ContextMenuHost({
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

  // Set when showNativeContextMenu() itself throws (Menu.new() or
  // popup_native_context_menu rejecting) so the render below falls back to
  // the rendered popup instead of leaving the user with no menu at all.
  const [nativeFailed, setNativeFailed] = useState(false)

  useEffect(() => {
    if (!isTauri() || !isOpen) return
    let cancelled = false
    setNativeFailed(false)
    void (async () => {
      try {
        await showNativeContextMenu(items, position, () => cancelled)
        if (!cancelled) onCloseRef.current()
      } catch (error) {
        console.error('Failed to show native context menu:', error)
        if (!cancelled) setNativeFailed(true)
        // Do NOT call onClose here — the fallback ImperativeContextMenu's own
        // dismissal calls the real onClose when the user actually dismisses
        // it. Closing immediately would flip the caller's `isOpen` to false
        // and unmount the fallback before it ever renders, since every real
        // call site gates its own render on that same `isOpen`.
      }
    })()
    return () => {
      cancelled = true
    }
    // items/position are read once, at the moment isOpen flips true — an
    // already-open native popup can't be updated mid-display anyway.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen])

  if (isTauri() && !nativeFailed) return null

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
