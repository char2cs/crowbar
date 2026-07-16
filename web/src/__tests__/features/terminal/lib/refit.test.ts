import { describe, it, expect, vi } from 'vitest'
import {
  refitAndSyncPty,
  pollUntilResizeSettles,
  type LastSyncedPtyDims,
} from '@/features/terminal/lib/refit'

const CONN = 'conn-1'
const COLS = 200

/** Container fake — the test controls the reported box (jsdom returns 0×0). */
function makeContainer(width: number, height: number) {
  return { getBoundingClientRect: () => ({ width, height }) }
}

/** A fit addon whose fit() mutates the shared terminal to `target` dims. */
function makeFit(terminal: { rows: number; cols: number }, target: { rows: number; cols: number }) {
  return {
    fit: vi.fn(() => {
      terminal.rows = target.rows
      terminal.cols = target.cols
    }),
  }
}

describe('refitAndSyncPty', () => {
  it('pushes the new size to the daemon when a fit shrinks xterm (56 → 28)', () => {
    const terminal = { rows: 56, cols: COLS }
    const fitAddon = makeFit(terminal, { rows: 28, cols: COLS })
    const resize = vi.fn()
    // Daemon last knew 56; the pane halved and this fit shrinks xterm to 28.
    const lastSyncedRef: { current: LastSyncedPtyDims | null } = {
      current: { connId: CONN, rows: 56, cols: COLS },
    }

    refitAndSyncPty({
      container: makeContainer(800, 300),
      fitAddon,
      terminal,
      getConnectionId: () => CONN,
      resize,
      lastSyncedRef,
    })

    expect(fitAddon.fit).toHaveBeenCalledTimes(1)
    expect(resize).toHaveBeenCalledWith(CONN, 28, COLS)
    expect(lastSyncedRef.current).toEqual({ connId: CONN, rows: 28, cols: COLS })
  })

  it('THE REGRESSION: a no-op fit still pushes when the daemon PTY is stale', () => {
    // xterm was already re-fitted to 28 by the remount, so fit() changes
    // nothing. But the daemon was last told 56. A naive "only push when the fit
    // changed xterm" would skip this and leave a full-screen TUI drawing at the
    // bottom of its 56-row model (blank top half). refitAndSyncPty gates on the
    // last size SYNCED TO THE DAEMON, so it pushes 28.
    const terminal = { rows: 28, cols: COLS }
    const fitAddon = makeFit(terminal, { rows: 28, cols: COLS }) // no-op fit
    const resize = vi.fn()
    const lastSyncedRef: { current: LastSyncedPtyDims | null } = {
      current: { connId: CONN, rows: 56, cols: COLS }, // daemon still at 56
    }

    refitAndSyncPty({
      container: makeContainer(800, 300),
      fitAddon,
      terminal,
      getConnectionId: () => CONN,
      resize,
      lastSyncedRef,
    })

    expect(resize).toHaveBeenCalledWith(CONN, 28, COLS)
    expect(lastSyncedRef.current).toEqual({ connId: CONN, rows: 28, cols: COLS })
  })

  it('does not re-push (no IPC spam) when xterm already matches the last synced size', () => {
    const terminal = { rows: 28, cols: COLS }
    const fitAddon = makeFit(terminal, { rows: 28, cols: COLS })
    const resize = vi.fn()
    const lastSyncedRef: { current: LastSyncedPtyDims | null } = {
      current: { connId: CONN, rows: 28, cols: COLS }, // already reconciled
    }

    refitAndSyncPty({
      container: makeContainer(800, 300),
      fitAddon,
      terminal,
      getConnectionId: () => CONN,
      resize,
      lastSyncedRef,
    })

    expect(fitAddon.fit).toHaveBeenCalledTimes(1) // fit still runs (WebGL/layout)
    expect(resize).not.toHaveBeenCalled() // but no redundant resize IPC
  })

  it('re-syncs when the connection id changed even if the dims are unchanged', () => {
    const terminal = { rows: 28, cols: COLS }
    const fitAddon = makeFit(terminal, { rows: 28, cols: COLS })
    const resize = vi.fn()
    // Same dims, but this is a different daemon connection (reconnect / swap):
    // the new PTY has never been told anything, so it must be reconciled.
    const lastSyncedRef: { current: LastSyncedPtyDims | null } = {
      current: { connId: 'conn-old', rows: 28, cols: COLS },
    }

    refitAndSyncPty({
      container: makeContainer(800, 300),
      fitAddon,
      terminal,
      getConnectionId: () => CONN,
      resize,
      lastSyncedRef,
    })

    expect(resize).toHaveBeenCalledWith(CONN, 28, COLS)
    expect(lastSyncedRef.current).toEqual({ connId: CONN, rows: 28, cols: COLS })
  })

  it('does not push when there is no connection id', () => {
    const terminal = { rows: 56, cols: COLS }
    const fitAddon = makeFit(terminal, { rows: 28, cols: COLS })
    const resize = vi.fn()
    const lastSyncedRef: { current: LastSyncedPtyDims | null } = { current: null }

    refitAndSyncPty({
      container: makeContainer(800, 300),
      fitAddon,
      terminal,
      getConnectionId: () => null, // not attached yet
      resize,
      lastSyncedRef,
    })

    expect(resize).not.toHaveBeenCalled()
    expect(lastSyncedRef.current).toBeNull()
  })

  it('does not fit or push when the container is 0×0 (hidden / unlaid-out)', () => {
    const terminal = { rows: 56, cols: COLS }
    const fitAddon = makeFit(terminal, { rows: 28, cols: COLS })
    const resize = vi.fn()
    const lastSyncedRef: { current: LastSyncedPtyDims | null } = { current: null }

    refitAndSyncPty({
      container: makeContainer(0, 0),
      fitAddon,
      terminal,
      getConnectionId: () => CONN,
      resize,
      lastSyncedRef,
    })

    expect(fitAddon.fit).not.toHaveBeenCalled()
    expect(resize).not.toHaveBeenCalled()
    expect(terminal.rows).toBe(56) // untouched
  })
})

// The single-PTY-size-owner contract (Task 4). A hidden attach-only view fits
// xterm to its box LOCALLY but must not own the daemon PTY size — XtermTerminal
// routes its resize into a no-op and its lastSyncedRef into a throwaway record so
// the SHARED record is untouched. Modelled here at the refit seam because jsdom
// gives the terminal a 0×0 box, so the component fit path never runs live.
describe('refitAndSyncPty — hidden attach-only view is not the PTY-size owner', () => {
  it('a no-op resize + throwaway ref fits xterm locally but pushes nothing and leaves the shared record untouched', () => {
    // The pane was resized while this chat tab was hidden: xterm shrinks 56 → 28
    // locally, but the daemon (and the shared last-synced record, still 56) must
    // NOT be advanced — the visible co-view owns the PTY size.
    const terminal = { rows: 56, cols: COLS }
    const fitAddon = makeFit(terminal, { rows: 28, cols: COLS })
    const realResize = vi.fn()
    const sharedLastSynced: { current: LastSyncedPtyDims | null } = {
      current: { connId: CONN, rows: 56, cols: COLS },
    }

    refitAndSyncPty({
      container: makeContainer(800, 300),
      fitAddon,
      terminal,
      getConnectionId: () => CONN,
      // XtermTerminal's hidden-attachOnly branch: resize is a no-op...
      resize: () => {},
      // ...and lastSyncedRef is a throwaway, so the shared record is not advanced.
      lastSyncedRef: { current: null },
    })

    expect(fitAddon.fit).toHaveBeenCalledTimes(1) // xterm still fitted locally
    expect(terminal.rows).toBe(28) // grid is correct the instant it is shown
    expect(realResize).not.toHaveBeenCalled() // nothing pushed to the daemon
    // The shared record the VISIBLE view relies on is untouched, still 56.
    expect(sharedLastSynced.current).toEqual({ connId: CONN, rows: 56, cols: COLS })
  })

  it('the SAME view once visible pushes the corrected size (real resize + shared ref)', () => {
    // On becoming visible the active-catch-up fit runs the real path against the
    // shared record — still the pre-hide 56 while xterm is now 28 — so it pushes 28
    // exactly once and reconciles the daemon.
    const terminal = { rows: 28, cols: COLS } // already fitted locally while hidden
    const fitAddon = makeFit(terminal, { rows: 28, cols: COLS })
    const realResize = vi.fn()
    const sharedLastSynced: { current: LastSyncedPtyDims | null } = {
      current: { connId: CONN, rows: 56, cols: COLS },
    }

    refitAndSyncPty({
      container: makeContainer(800, 300),
      fitAddon,
      terminal,
      getConnectionId: () => CONN,
      resize: (connId, rows, cols) => realResize(connId, rows, cols),
      lastSyncedRef: sharedLastSynced,
    })

    expect(realResize).toHaveBeenCalledWith(CONN, 28, COLS)
    expect(sharedLastSynced.current).toEqual({ connId: CONN, rows: 28, cols: COLS })
  })
})

/**
 * Deterministic fake requestAnimationFrame: callbacks queue by id and only run
 * when the test flushes. A frame scheduled DURING a flush runs on the NEXT
 * flush, mirroring real rAF — no real timers, no polling in the test.
 */
function makeFakeRaf() {
  let nextId = 1
  const scheduled = new Map<number, () => void>()
  return {
    requestFrame: (cb: () => void) => {
      const id = nextId++
      scheduled.set(id, cb)
      return id
    },
    cancelFrame: (id: number) => {
      scheduled.delete(id)
    },
    flush() {
      const batch = [...scheduled.values()]
      scheduled.clear()
      for (const cb of batch) cb()
    },
    pending: () => scheduled.size,
  }
}

describe('pollUntilResizeSettles', () => {
  it('defers while resizing, then fires onSettled exactly once when the drag ends', () => {
    const raf = makeFakeRaf()
    let resizing = true
    const onSettled = vi.fn()

    pollUntilResizeSettles({
      isResizing: () => resizing,
      onSettled,
      requestFrame: raf.requestFrame,
      cancelFrame: raf.cancelFrame,
    })

    // Frames tick while the drag is live: keep polling, never settle.
    raf.flush()
    raf.flush()
    expect(onSettled).not.toHaveBeenCalled()
    expect(raf.pending()).toBe(1) // still polling

    // Drag ends → the next frame settles once and stops polling.
    resizing = false
    raf.flush()
    expect(onSettled).toHaveBeenCalledTimes(1)
    expect(raf.pending()).toBe(0)

    // No further frames scheduled; extra flushes are inert.
    raf.flush()
    expect(onSettled).toHaveBeenCalledTimes(1)
  })

  it('settles on the first frame when the drag has already ended', () => {
    const raf = makeFakeRaf()
    const onSettled = vi.fn()

    pollUntilResizeSettles({
      isResizing: () => false,
      onSettled,
      requestFrame: raf.requestFrame,
      cancelFrame: raf.cancelFrame,
    })

    raf.flush()
    expect(onSettled).toHaveBeenCalledTimes(1)
  })

  it('cancel stops the poll and guarantees onSettled never fires', () => {
    // Models effect cleanup / an earlier pane-resize-end arriving mid-drag: even
    // after the drag later ends, the cancelled poll must not fit a torn-down or
    // superseded terminal.
    const raf = makeFakeRaf()
    let resizing = true
    const onSettled = vi.fn()

    const cancel = pollUntilResizeSettles({
      isResizing: () => resizing,
      onSettled,
      requestFrame: raf.requestFrame,
      cancelFrame: raf.cancelFrame,
    })

    raf.flush() // still resizing → rescheduled
    expect(raf.pending()).toBe(1)

    cancel()
    expect(raf.pending()).toBe(0) // pending frame cancelled

    resizing = false
    raf.flush()
    expect(onSettled).not.toHaveBeenCalled()
  })
})
