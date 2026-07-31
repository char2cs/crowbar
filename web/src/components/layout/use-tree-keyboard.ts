import {
  useCallback,
  useEffect,
  useRef,
  type FocusEvent,
  type KeyboardEvent,
  type RefObject,
} from 'react'
import { useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { readDropRow } from './drop-target-dom'
import { visibleRows } from './row-selection'
import { isTypeaheadKey, resolveTreeKey } from './tree-keyboard'
import type { DropRow } from './drop-target-dom'

/**
 * Keyboard navigation for the sidebar tree, driven entirely through the DOM.
 *
 * Moving between rows costs ZERO React renders. Focus and the single tab stop
 * are both element state, and routing an arrow key through a store would
 * re-render every row that reads it to move a focus ring — on the interaction
 * most likely to be held down.
 *
 * The rows themselves are all `tabIndex={-1}`; exactly one is promoted to 0 here
 * so the whole tree is one stop in the tab order rather than one stop per row.
 */

/** How long one word of type-to-jump stays open before the next key starts a new one. */
const TYPEAHEAD_WINDOW_MS = 700

const ROW_SELECTOR = '[role="treeitem"]'

export interface TreeKeyboardHandlers {
  /** Collapse or expand a row, whichever it is currently doing. */
  onToggle: (row: DropRow) => void
  /** Delete: send the selection (or the focused row) to the removal tray. */
  onRemove: (focused: DropRow | null) => void
}

export interface TreeKeyboard {
  treeRef: RefObject<HTMLDivElement | null>
  onKeyDown: (e: KeyboardEvent<HTMLElement>) => void
  /** Follow a row that took focus by any other route — a click, or a Tab in. */
  onFocus: (e: FocusEvent<HTMLElement>) => void
}

function rowElements(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(ROW_SELECTOR))
}

function idOf(el: HTMLElement): string | null {
  return readDropRow(el)?.id ?? null
}

/**
 * Leave exactly one row tabbable.
 *
 * Written imperatively and only where it differs: React never rewrites the rows'
 * `tabIndex` because the prop it renders never changes, so the promotion sticks
 * across re-renders and costs nothing to maintain.
 */
function setTabStop(root: HTMLElement, preferId?: string | null): void {
  const rows = rowElements(root)
  if (rows.length === 0) return
  const chosen =
    (preferId ? rows.find((el) => idOf(el) === preferId) : undefined) ??
    rows.find((el) => el.tabIndex === 0) ??
    rows[0]
  for (const el of rows) {
    const wanted = el === chosen ? 0 : -1
    if (el.tabIndex !== wanted) el.tabIndex = wanted
  }
}

/** Whether the key press came from something that is spelling, not navigating. */
function isTextEntry(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  return target.closest('input, textarea, [contenteditable="true"]') !== null
}

export function useTreeKeyboard({ onToggle, onRemove }: TreeKeyboardHandlers): TreeKeyboard {
  const treeRef = useRef<HTMLDivElement | null>(null)
  const typed = useRef({ prefix: '', at: 0 })

  // Runs after every render of the tree, which is when rows can have appeared or
  // gone. The steady state is one querySelectorAll and no writes.
  useEffect(() => {
    if (treeRef.current) setTabStop(treeRef.current)
  })

  const onFocus = useCallback((e: FocusEvent<HTMLElement>) => {
    const root = treeRef.current
    if (!root || !(e.target instanceof HTMLElement)) return
    const row = e.target.closest<HTMLElement>(ROW_SELECTOR)
    if (row) setTabStop(root, idOf(row))
  }, [])

  const onKeyDown = useCallback(
    (e: KeyboardEvent<HTMLElement>) => {
      const root = treeRef.current
      if (!root) return
      // A row's own Enter/Space handler runs first and marks the event handled;
      // acting on it again here would open the same workspace twice.
      if (e.defaultPrevented || isTextEntry(e.target)) return

      const modified = e.metaKey || e.ctrlKey || e.altKey
      const focusedRow =
        e.target instanceof HTMLElement ? e.target.closest<HTMLElement>(ROW_SELECTOR) : null
      const focusedId = focusedRow ? idOf(focusedRow) : null

      const now = Date.now()
      const extending =
        isTypeaheadKey(e.key, modified) && now - typed.current.at < TYPEAHEAD_WINDOW_MS
      const prefix = isTypeaheadKey(e.key, modified)
        ? (extending ? typed.current.prefix : '') + e.key
        : ''
      if (prefix) typed.current = { prefix, at: now }

      const rows = visibleRows(root)
      const action = resolveTreeKey({
        key: e.key,
        modified,
        rows,
        focusedId,
        prefix,
        // A repeated letter walks the rows that share it; a growing word must
        // stay on the row it is narrowing towards.
        extendingPrefix: extending && prefix.length > 1,
      })
      if (!action) return
      // Enter belongs to whatever it was pressed on. `treeitem` permits
      // interactive descendants, and the row's own controls are real buttons —
      // swallowing their Enter here would stop them activating at all.
      if (action.kind === 'activate' && e.target !== focusedRow) return

      e.preventDefault()

      switch (action.kind) {
        case 'move': {
          const target = rowElements(root).find((el) => idOf(el) === action.id)
          if (!target) return
          setTabStop(root, action.id)
          target.focus()
          return
        }
        case 'collapse':
        case 'expand': {
          const row = rows.find((r) => r.id === action.id)
          if (row) onToggle(row)
          return
        }
        case 'activate':
          focusedRow?.click()
          return
        case 'clear-selection':
          useSidebarSelectionStore.getState().clearSelected()
          return
        case 'remove':
          onRemove(rows.find((r) => r.id === focusedId) ?? null)
      }
    },
    [onToggle, onRemove],
  )

  return { treeRef, onKeyDown, onFocus }
}
