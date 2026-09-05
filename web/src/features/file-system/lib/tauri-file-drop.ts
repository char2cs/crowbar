import { useEffect } from 'react'
import type { RefObject } from 'react'
import { isTauri } from '@/lib/crowbar-bridge'

export interface TauriDropPosition {
  x: number
  y: number
}

/** Pure — is `position` (Tauri's CSS-pixel, window-relative drop coordinate)
 *  inside `rect`. Inclusive of the boundary. */
export function pointInRect(position: TauriDropPosition, rect: DOMRect): boolean {
  return (
    position.x >= rect.left &&
    position.x <= rect.right &&
    position.y >= rect.top &&
    position.y <= rect.bottom
  )
}

/**
 * Real OS file paths dropped on `containerRef`'s element — Tauri desktop only.
 *
 * Tauri's webview intercepts a native OS file drag before it becomes a DOM
 * `drop` event at all: `dragDropEnabled` defaults to true and nothing in
 * `desktop/src-tauri/tauri.conf.json` overrides it, so `DataTransfer` never
 * carries files for a real drag and no DOM `drop` handler ever fires for
 * one. `extractDroppedFilePaths` (file-system-dropped-paths.ts) covers only
 * the other, unavoidably path-less case — a plain browser tab, where a
 * `DataTransfer` exists but no browser exposes a real host path on a `File`
 * either way. This hook is what actually answers "a real file got dropped
 * here, and here is its path" on the desktop build.
 *
 * Every mounted consumer receives the SAME window-wide event — Tauri does
 * not scope `onDragDropEvent` to an element — and filters it against its own
 * `getBoundingClientRect()`. Cheap, and avoids inventing a second routing
 * contract when the DOM already gives every consumer its own rect.
 */
export function useTauriFileDrop(
  containerRef: RefObject<HTMLElement | null>,
  onDrop: (paths: string[]) => void,
): void {
  useEffect(() => {
    if (!isTauri()) return
    let disposed = false
    let unlisten: (() => void) | undefined

    void (async () => {
      const { getCurrentWebview } = await import('@tauri-apps/api/webview')
      const stop = await getCurrentWebview().onDragDropEvent((event) => {
        if (event.payload.type !== 'drop') return
        const el = containerRef.current
        if (!el) return
        if (pointInRect(event.payload.position, el.getBoundingClientRect())) {
          onDrop(event.payload.paths)
        }
      })
      if (disposed) stop()
      else unlisten = stop
    })()

    return () => {
      disposed = true
      unlisten?.()
    }
  }, [containerRef, onDrop])
}
