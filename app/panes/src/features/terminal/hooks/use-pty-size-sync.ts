import { useEffect, useRef } from 'react'
import type { Terminal } from '@xterm/xterm'
import type { FitAddon } from '@xterm/addon-fit'
import type { TerminalConnection } from '@/lib/crowbar-bridge'
import { pollUntilResizeSettles, watchDevicePixelRatio } from '../lib/refit'

interface UsePtySizeSyncOptions {
  terminal: Terminal | null
  fitAddon: FitAddon | null
  container: HTMLElement | null
  connection: TerminalConnection | null
  isVisible: boolean
}

export function fitToContainer(fitAddon: FitAddon | null, container: HTMLElement | null): void {
  if (!fitAddon || !container) return
  const rect = container.getBoundingClientRect()
  if (rect.width <= 0 || rect.height <= 0) return
  fitAddon.fit()
}

export function usePtySizeSync({
  terminal,
  fitAddon,
  container,
  connection,
  isVisible,
}: UsePtySizeSyncOptions): void {
  const lastSyncedRef = useRef<{ connection: TerminalConnection; rows: number; cols: number }>(null)
  const syncRef = useRef<() => void>(() => {})

  useEffect(() => {
    syncRef.current = () => {
      if (!terminal || !connection || !connection.alive || !isVisible) return
      const { rows, cols } = terminal
      const last = lastSyncedRef.current
      if (last && last.connection === connection && last.rows === rows && last.cols === cols) {
        return
      }
      connection.resize(rows, cols)
      lastSyncedRef.current = { connection, rows, cols }
    }
    fitToContainer(fitAddon, container)
    syncRef.current()
  }, [terminal, fitAddon, container, connection, isVisible])

  useEffect(() => {
    if (!terminal) return
    const disposable = terminal.onResize(() => syncRef.current())
    return () => disposable.dispose()
  }, [terminal])

  useEffect(() => {
    if (!terminal || !fitAddon || !container || typeof ResizeObserver === 'undefined') return
    let frame: number | null = null
    let cancelSettle: (() => void) | null = null
    const dragging = () => document.documentElement.hasAttribute('data-pane-resizing')
    const fitNow = () => {
      fitToContainer(fitAddon, container)
      syncRef.current()
    }
    const schedule = () => {
      if (dragging()) {
        cancelSettle ??= pollUntilResizeSettles({
          isResizing: dragging,
          onSettled: () => {
            cancelSettle = null
            fitNow()
          },
          requestFrame: (cb) => requestAnimationFrame(cb),
          cancelFrame: (id) => cancelAnimationFrame(id),
        })
        return
      }
      if (frame !== null) return
      frame = requestAnimationFrame(() => {
        frame = null
        fitNow()
      })
    }
    const observer = new ResizeObserver(schedule)
    observer.observe(container)
    const stopWatchingDpr = watchDevicePixelRatio(schedule)
    return () => {
      observer.disconnect()
      stopWatchingDpr()
      if (frame !== null) cancelAnimationFrame(frame)
      cancelSettle?.()
    }
  }, [terminal, fitAddon, container])
}
