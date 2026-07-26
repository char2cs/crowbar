import {
  terminalWrite,
  terminalResize,
  terminalResync,
  terminalSetTheme,
  terminalClose,
  terminalListen,
} from '@/lib/crowbar-bridge'
import type { IDisposable, Terminal as XtermTerminal } from '@xterm/xterm'
import { useEffect, useRef } from 'react'
import { markStart, markEnd } from '@/lib/perf/instrumentation'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { parseOSC7 } from '../utils/osc-parser'
import { sanitizeTerminalTitle } from '../utils/terminal-title'
import { shouldScrollScrollback } from '../utils/wheel-routing'
import { readTerminalThemePayload } from './use-terminal-theme'
import { useTerminalWriteBuffer } from './use-terminal-write-buffer'

interface UseTerminalConnectionOptions {
  connectionId?: string
  getTerminalTheme: () => NonNullable<XtermTerminal['options']['theme']>
  initialCommand?: string
  isInitialized: boolean
  onTerminalExit?: (sessionId: string) => void
  // Bumped by the parent component after a transport drop + re-attach so this
  // hook re-registers its terminalListen call on the fresh connection object
  // even when connectionId itself has not changed.
  reconnectKey?: number
  remoteConnectionId?: string
  reuseExistingConnection?: boolean
  sessionId: string
  terminal: XtermTerminal | null
  updateSession: (
    sessionId: string,
    updates: {
      currentDirectory?: string
      selection?: string
      title?: string
    },
  ) => void
}

export function useTerminalConnection({
  connectionId,
  getTerminalTheme,
  initialCommand,
  isInitialized,
  onTerminalExit,
  reconnectKey = 0,
  remoteConnectionId,
  reuseExistingConnection = false,
  sessionId,
  terminal,
  updateSession,
}: UseTerminalConnectionOptions) {
  const currentConnectionIdRef = useRef<string | null>(null)
  const currentInputLineRef = useRef('')
  const initialCommandSentForConnectionRef = useRef<string | null>(null)
  const onTerminalExitRef = useRef(onTerminalExit)
  const explicitExitRequestedRef = useRef(false)
  const lastExitInfoRef = useRef<{ exitCode?: number | null; signal?: string | null } | null>(null)
  const outputBufferRef = useRef('')
  // One-shot flag: armed on every (re)attach, consumed by the first output
  // flush (the daemon's bulk scrollback replay) to force a viewport repaint.
  const pendingAttachFinalizeRef = useRef(false)
  // Latched while a snapshot's reset+redraw is sequenced through xterm's async
  // write queue (see the snapshot branch below). Incremental frames that arrive
  // while this is set must NOT be flushed — xterm parses writes in enqueue
  // order, so a frame flushed between the barrier write and its callback would
  // parse AFTER reset() but BEFORE the redraw, landing on a blank buffer and
  // then getting silently overwritten by the redraw that follows it. Cleared
  // only inside the barrier callback, immediately after the redraw write is
  // enqueued, so anything still buffered is flushed strictly after the redraw.
  const snapshotPendingRef = useRef(false)
  // Guards the barrier callback above against two races the boolean latch alone
  // cannot express: (a) a second snapshot arriving before the first snapshot's
  // barrier callback has fired — both barriers would otherwise fire and both
  // would reset+redraw, replaying scrollback twice — and (b) this effect
  // tearing down (reconnectKey bump mid-burst) while a barrier is still
  // in-flight — the stale callback would otherwise fire later against a
  // NEW connection's terminal state. Bumped each time a snapshot latches (the
  // barrier closes over its own generation) and again on effect cleanup; a
  // barrier whose captured generation no longer matches the live counter is
  // inert — no reset, no redraw write, no unlatch, no flush reschedule.
  const snapshotGenRef = useRef(0)
  const { write, flush } = useTerminalWriteBuffer({
    getConnectionId: () => currentConnectionIdRef.current,
    writeChunk: async (activeConnectionId, data) => {
      await terminalWrite(activeConnectionId, data)
    },
  })

  useEffect(() => {
    onTerminalExitRef.current = onTerminalExit
  }, [onTerminalExit])

  useEffect(() => {
    currentConnectionIdRef.current = connectionId ?? null
  }, [connectionId])

  useEffect(() => {
    explicitExitRequestedRef.current = false
    lastExitInfoRef.current = null
  }, [connectionId])

  // Arm the one-shot viewport finalize for the first output flush after every
  // (re)attach. The first post-attach frame is the daemon's bulk scrollback
  // replay; writeFrame repaints + scrolls to bottom once it lands so the
  // re-attached xterm is never left blank in WKWebView. Keyed only on the
  // connection identity (not the main effect's callback deps) so ordinary
  // mid-session re-renders don't re-arm it and yank a scrolled-up user to the
  // bottom. reconnectKey bumps on a transport-drop re-attach (same connectionId).
  useEffect(() => {
    pendingAttachFinalizeRef.current = true
  }, [connectionId, reconnectKey])

  useEffect(() => {
    if (!terminal || !isInitialized || !connectionId) return

    const disposables: IDisposable[] = []

    // Writes a frame straight to xterm on arrival — no discretionary
    // requestAnimationFrame delay between the daemon's bytes landing and
    // terminal.write(). That rAF used to add up to a full frame of latency to
    // every keystroke echo, stacked on top of xterm's own render rAF.
    const writeFrame = (data: string) => {
      // Correctness gate: while a snapshot's reset+redraw is pending in
      // xterm's write queue, hold frames in the buffer instead — the barrier
      // callback below drains it once the redraw has been enqueued. See
      // snapshotPendingRef above for why writing here would race the redraw.
      if (snapshotPendingRef.current) {
        outputBufferRef.current += data
        return
      }

      // The first write after a (re)attach is the daemon's bulk ring-buffer
      // replay, written into a freshly-opened xterm. xterm schedules its own
      // render after write(), but in WKWebView that render can no-op — the
      // WebGL canvas is not repainted until something invalidates it, leaving
      // the viewport BLANK until a manual scroll forces refresh() (the
      // "terminal empty until I scroll" bug after a workspace switch). Force it
      // once, in write()'s parse-complete callback: pin to the latest output
      // and repaint every visible row. Gated to this first post-attach write so
      // live streaming keeps xterm's cheap incremental rendering — an
      // unconditional refresh per frame is expensive and compounds with
      // WKWebView CA-layer re-rasterization.
      const finalizeViewport = pendingAttachFinalizeRef.current
      if (finalizeViewport) pendingAttachFinalizeRef.current = false

      // One callback always runs (rather than only on the finalize path) so the
      // terminal.echo span opened in onData below gets closed here regardless
      // of which branch this write takes. markEnd is a no-op without a prior
      // markStart, so output-only frames (no keystroke) don't fabricate a
      // span, and several keystrokes whose writes complete before xterm's
      // parser catches up coalesce into one span — acceptable slop for a p95
      // latency metric.
      terminal.write(data, () => {
        markEnd('terminal.echo')
        if (finalizeViewport) {
          terminal.scrollToBottom()
          terminal.refresh(0, terminal.rows - 1)
        }
      })

      const newDirectory = parseOSC7(data)
      if (newDirectory) updateSession(sessionId, { currentDirectory: newDirectory })
    }

    disposables.push(
      terminal.onData((data) => {
        // xterm emits a focus report (\x1b[I / \x1b[O) whenever its helper
        // textarea gains/loses DOM focus. Inside an IDE that fires on EVERY pane
        // switch — clicking the sidebar, the editor, another tab — so with focus
        // reporting (DECSET ?1004) enabled it floods the PTY with focus events. A
        // shell sitting at its prompt echoes those as visible "^[[O"/"^[[I"
        // garbage. Drop the pane-level events here; real WINDOW focus is reported
        // via the window focus/blur listeners below, which is what focus-aware
        // TUIs (Claude Code, vim, tmux) actually want to track.
        if (data === '\x1b[I' || data === '\x1b[O') return

        // M1 terminal.echo span: opened here (keystroke in), closed in
        // writeFrame's terminal.write callback (echo painted). See the
        // markEnd call site for the coalescing/no-prior-mark caveats.
        markStart('terminal.echo')

        const activeConnectionId = currentConnectionIdRef.current || connectionId
        const hasNewline = data.includes('\n') || data.includes('\r')

        if (hasNewline) {
          currentInputLineRef.current += data
          if (/^\s*exit\s*$/i.test(currentInputLineRef.current.trim())) {
            explicitExitRequestedRef.current = true
            currentInputLineRef.current = ''
            write(data, 'onData:exit')
            window.setTimeout(() => {
              void terminalClose(activeConnectionId).catch(() => {})
            }, 100)
            return
          }
          currentInputLineRef.current = ''
        } else {
          currentInputLineRef.current += data
          if (currentInputLineRef.current.length > 1000) {
            currentInputLineRef.current = currentInputLineRef.current.slice(-100)
          }
        }

        write(data, 'onData')
      }),
    )

    // NOTE: there is intentionally no terminal.onKey handler. xterm's built-in
    // keyboard model already emits the correct sequence for every standard key
    // (Ctrl+U, Shift+Tab, arrows, Backspace, Option word-nav via macOptionIsMeta,
    // ...) via onData. A second onKey that re-wrote those sequences sent every
    // such key TWICE -- the double-fire bug. The only sequence xterm 5.5.0 cannot
    // produce (Shift/Alt+Enter disambiguation, kitty protocol) is handled once in
    // the attachCustomKeyEventHandler in terminal.tsx (see resolveKeyOverride).

    // Post-resize convergence: after the grid dimensions settle (debounced past
    // the last onResize of a gesture — sash drag, window/monitor resize, sidebar
    // toggle), ask the daemon to re-emit its model snapshot. xterm's client-side
    // reflow deposits stale copies of a repainting TUI into LOCAL scrollback on
    // every resize; the daemon model never reflows, so replacing the local
    // buffer with the snapshot removes the junk. The daemon no-ops at an idle
    // shell prompt, where xterm's native reflow is already correct and a resync
    // would only cost the scroll position.
    let resyncTimer: number | null = null
    const scheduleResync = () => {
      if (resyncTimer !== null) window.clearTimeout(resyncTimer)
      resyncTimer = window.setTimeout(() => {
        resyncTimer = null
        void terminalResync(connectionId).catch(() => {})
      }, 300)
    }

    disposables.push(
      terminal.onResize(({ cols, rows }) => {
        void terminalResize(connectionId, rows, cols).catch(() => {})
        scheduleResync()
      }),
    )

    // NOTE: do NOT mirror the live selection into the terminal store here.
    // onSelectionChange fires on every mousemove during a drag-select, and
    // updateSession() allocates a brand-new sessions Map each call — which
    // re-renders every tab label (useBufferDisplayName subscribes to the whole
    // Map). Nothing reads session.selection: consumers that need the selected
    // text read it live from xterm via the session ref's getSelection(). Mirroring
    // it was pure churn that made selecting text in the terminal jankily slow.

    disposables.push(
      terminal.onTitleChange((rawTitle) => {
        // Always write the sanitized result. An empty/rejected title — e.g. a
        // shell that embeds its colored prompt in the OSC title — clears any
        // prior (possibly garbled) value so the tab falls back to the
        // directory/command label instead of showing stale garbage.
        updateSession(sessionId, { title: sanitizeTerminalTitle(rawTitle) })
      }),
    )

    // Propagate the host light/dark theme to the daemon so a foreground app's automatic
    // theme (Claude Code's `auto`) follows Crowbar. Push once on (re)attach — so a freshly
    // started app queries the correct background — and again on every theme switch — so an
    // ALREADY-running, DEC-2031-subscribed app is notified and re-queries live.
    const pushTheme = () => {
      void terminalSetTheme(connectionId, readTerminalThemePayload()).catch(() => {})
    }
    pushTheme()

    const unlistenThemeChange = themeRegistry.onThemeChange(() => {
      terminal.options.theme = getTerminalTheme()
      pushTheme()
    })

    // Use terminalListen from crowbar-bridge for PTY output. Snapshot frames
    // (the attach redraw, the post-resize resync) are self-contained
    // ground-state redraws serialized from the daemon's screen model: apply
    // them onto a RESET buffer — dropping any buffered pre-snapshot output the
    // redraw supersedes — so client-side reflow junk (stale TUI copies pushed
    // into local scrollback by xterm's resize semantics) is replaced by truth.
    const unlistenOutput = terminalListen(connectionId, (frame) => {
      if (frame.snapshot) {
        outputBufferRef.current = ''
        pendingAttachFinalizeRef.current = false
        // Latch BEFORE the barrier write so any incremental frame that arrives
        // before the barrier callback fires is held in outputBufferRef instead
        // of being written — see snapshotPendingRef and writeFrame above.
        snapshotPendingRef.current = true
        // Bump the generation and let this barrier's callback close over its
        // own value. If a second snapshot latches before this barrier's
        // callback runs, that second snapshot bumps the counter again — this
        // barrier's captured `gen` then no longer matches
        // snapshotGenRef.current, so its callback becomes a dead no-op below
        // (only the LAST snapshot's barrier ever resets+redraws — see
        // snapshotGenRef above).
        const gen = ++snapshotGenRef.current
        // Sequence the reset + redraw THROUGH xterm's async write queue. xterm
        // parses write() data asynchronously, so any live bytes handed to
        // terminal.write() before this snapshot arrived are still sitting in that
        // queue. A synchronous terminal.reset() here would run BEFORE those queued
        // bytes are parsed — they would then apply onto the freshly reset buffer,
        // injecting stale content into the snapshot. Deferring the reset+redraw to
        // an empty write's parse-complete callback runs them only AFTER everything
        // already queued ahead has been parsed, so the snapshot lands on a truly
        // clean buffer.
        terminal.write('', () => {
          // Stale barrier: a newer snapshot latched (or this effect tore down)
          // since this write was enqueued. Do nothing — no reset, no redraw
          // write, no unlatch, no flush reschedule — the current generation's
          // barrier (or the fresh effect's teardown state) owns all of that.
          if (snapshotGenRef.current !== gen) return
          terminal.reset()
          terminal.write(frame.data, () => {
            terminal.scrollToBottom()
            terminal.refresh(0, terminal.rows - 1)
          })
          // Clear the latch immediately after the redraw write is ENQUEUED
          // (not after its parse-complete callback runs). xterm parses writes
          // in enqueue order, so anything scheduled here is guaranteed to
          // parse strictly after the redraw above — never between reset() and
          // the redraw.
          snapshotPendingRef.current = false
          if (outputBufferRef.current) {
            const held = outputBufferRef.current
            outputBufferRef.current = ''
            writeFrame(held)
          }
        })
        return
      }
      writeFrame(frame.data)
    })

    // Route the wheel by the app's DECLARED mouse-tracking intent, not by which
    // buffer is active (see shouldScrollScrollback for the full rationale):
    //
    //  - No mouse tracking → the terminal owns the wheel; return early and let
    //    xterm scroll its own viewport natively (unchanged normal-shell behavior).
    //  - Mouse tracking ON → the APP owns the wheel on BOTH buffers; return early
    //    and let xterm forward the wheel to the app. This is what makes scrolling
    //    inside a full-screen TUI (Claude Code) reach the app — even in the
    //    (mouseTrackingMode === 'any', buffer.active.type === 'normal') state the
    //    daemon serializes when an app tracks the mouse on the primary buffer.
    //  - The one escape hatch: Shift+wheel on the primary buffer — the universal
    //    "scroll the terminal, not the app" modifier — is the ONLY case we
    //    intercept and scroll our own scrollback. scrollLines(): negative = up
    //    (older history), positive = down; wheel deltaY is already negative on
    //    scroll-up, so signs align.
    const wheelContainer = terminal.element?.parentElement
    const handleWheel = (event: WheelEvent) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const mode = (terminal as any).modes?.mouseTrackingMode as string | undefined
      if (!shouldScrollScrollback(event, mode, terminal.buffer.active.type)) return
      event.preventDefault()
      event.stopPropagation()
      const lines = Math.ceil(Math.abs(event.deltaY) / 40) * (event.deltaY < 0 ? -1 : 1)
      terminal.scrollLines(lines * 3)
    }
    wheelContainer?.addEventListener('wheel', handleWheel, { capture: true, passive: false })

    // Report focus to the PTY on real WINDOW focus changes instead of the
    // pane-level textarea focus xterm reports by default (suppressed in onData
    // above). Only emit when the app has actually enabled focus reporting
    // (DECSET ?1004); otherwise this is a no-op. This makes "is the Crowbar
    // window focused" the signal apps see — accurate, and free of the per-click
    // focus-event spam that a shell would echo as garbage.
    const emitFocusReport = (seq: string) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      if ((terminal as any).modes?.sendFocusMode) write(seq, 'focus-report')
    }
    const handleWindowFocus = () => emitFocusReport('\x1b[I')
    const handleWindowBlur = () => emitFocusReport('\x1b[O')
    window.addEventListener('focus', handleWindowFocus)
    window.addEventListener('blur', handleWindowBlur)

    return () => {
      window.removeEventListener('focus', handleWindowFocus)
      window.removeEventListener('blur', handleWindowBlur)
      wheelContainer?.removeEventListener('wheel', handleWheel, true)
      if (resyncTimer !== null) window.clearTimeout(resyncTimer)
      // Never leave a reconnect starting latched: a fresh (re)attach effect
      // must be free to write frames from its first frame. Also bump the
      // generation so any barrier still in flight from THIS effect instance
      // (enqueued but not yet parsed — e.g. a reconnectKey bump mid-burst)
      // becomes inert instead of firing later against the next connection's
      // terminal state — see snapshotGenRef above. Note: outputBufferRef may
      // still hold unflushed bytes buffered while latched; that's benign,
      // since the next attach's snapshot resets outputBufferRef to '' before
      // anything is written. There is no scheduled-but-not-yet-run write to
      // flush here: writeFrame writes synchronously on arrival whenever the
      // latch is clear, so nothing is ever left pending except while latched.
      snapshotGenRef.current += 1
      snapshotPendingRef.current = false
      void flush()
      for (const disposable of disposables) {
        disposable.dispose()
      }
      unlistenThemeChange()
      unlistenOutput()
    }
  }, [
    connectionId,
    flush,
    getTerminalTheme,
    isInitialized,
    reconnectKey,
    sessionId,
    terminal,
    updateSession,
    write,
    remoteConnectionId,
  ])

  useEffect(() => {
    if (!initialCommand || !connectionId || reuseExistingConnection) return
    if (initialCommandSentForConnectionRef.current === connectionId) return

    initialCommandSentForConnectionRef.current = connectionId
    const timeoutId = window.setTimeout(() => {
      write(`${initialCommand}\n`, 'initial-command')
    }, 300)

    return () => window.clearTimeout(timeoutId)
  }, [connectionId, initialCommand, reuseExistingConnection, write])

  return {
    currentConnectionIdRef,
    writeBuffered: write,
  }
}
