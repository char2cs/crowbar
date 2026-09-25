import { create } from 'zustand'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'
import type { Position, Range } from '@/features/editor/types/editor'
import { createSelectors } from '@/utils/zustand-selectors'

/**
 * The ACTIVE editor's cursor, mirrored out of Monaco for readers outside it
 * (status-bar line:col, the jump-list recorder), plus a small per-view cache
 * of where the cursor was in each buffer. Monaco itself owns the editor
 * state; this is a read-only projection written by the editor surface.
 */
export interface EditorViewState {
  cursor: Position
  selection?: Range
  scrollTop: number
  scrollLeft: number
}

const viewStateCache = new Map<string, EditorViewState>()

function cacheViewState(key: string, state: EditorViewState): void {
  if (!viewStateCache.has(key) && viewStateCache.size >= EDITOR_CONSTANTS.MAX_POSITION_CACHE_SIZE) {
    const oldest = viewStateCache.keys().next().value
    if (oldest !== undefined) viewStateCache.delete(oldest)
  }
  viewStateCache.set(key, state)
}

/** An exact view key, else a `${paneId}:${bufferId}` entry for the bare buffer id. */
function cachedViewState(key: string): EditorViewState | null {
  const exact = viewStateCache.get(key)
  if (exact) return exact
  const separator = key.lastIndexOf(':')
  if (separator >= 0) return viewStateCache.get(key.slice(separator + 1)) ?? null
  for (const [cachedKey, state] of viewStateCache) {
    if (cachedKey.endsWith(`:${key}`)) return state
  }
  return null
}

function positionsEqual(a: Position, b: Position): boolean {
  return a.line === b.line && a.column === b.column && a.offset === b.offset
}

function rangesEqual(a?: Range, b?: Range): boolean {
  if (a === b) return true
  if (!a || !b) return false
  return positionsEqual(a.start, b.start) && positionsEqual(a.end, b.end)
}

interface EditorState {
  cursorPosition: Position
  selection?: Range
  /** `${paneId}:${bufferId}` of the active editor surface. */
  activeEditorViewKey: string | null
  actions: {
    /** One batched write for a burst of cursor moves (rAF-coalesced by the caller). */
    setCursorAndSelection: (position: Position, selection?: Range) => void
    setActiveEditorViewKey: (viewKey: string | null) => void
    getCachedPosition: (viewKey: string) => Position | null
    getCachedViewState: (viewKey: string) => EditorViewState | null
    cacheViewStateForBuffer: (viewKey: string, state: EditorViewState) => void
    clearPositionCache: (viewKey?: string) => void
  }
}

export const useEditorStateStore = createSelectors(
  create<EditorState>()((set, get) => ({
    cursorPosition: { line: 0, column: 0, offset: 0 },
    selection: undefined,
    activeEditorViewKey: null,
    actions: {
      setCursorAndSelection: (position, selection) => {
        const current = get()
        if (current.activeEditorViewKey) {
          const cached = viewStateCache.get(current.activeEditorViewKey)
          cacheViewState(current.activeEditorViewKey, {
            cursor: position,
            selection,
            scrollTop: cached?.scrollTop ?? 0,
            scrollLeft: cached?.scrollLeft ?? 0,
          })
        }
        const cursorChanged = !positionsEqual(current.cursorPosition, position)
        const selectionChanged = !rangesEqual(current.selection, selection)
        if (cursorChanged || selectionChanged) set({ cursorPosition: position, selection })
      },
      setActiveEditorViewKey: (activeEditorViewKey) => {
        if (get().activeEditorViewKey !== activeEditorViewKey) set({ activeEditorViewKey })
      },
      getCachedPosition: (viewKey) => {
        const cached = cachedViewState(viewKey)
        return cached ? { ...cached.cursor } : null
      },
      getCachedViewState: (viewKey) => cachedViewState(viewKey),
      cacheViewStateForBuffer: (viewKey, state) => cacheViewState(viewKey, state),
      clearPositionCache: (viewKey) => {
        if (viewKey) viewStateCache.delete(viewKey)
        else viewStateCache.clear()
      },
    },
  })),
)
