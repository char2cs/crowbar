import type { TerminalConnection } from '@/lib/crowbar-bridge'
import type { Terminal as XtermTerminal } from '@xterm/xterm'
import { useCallback, useEffect, useRef } from 'react'
import { markStart, markEnd } from '@/lib/perf/instrumentation'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { sanitizeTerminalTitle } from '../utils/terminal-title'
import { shouldScrollScrollback } from '../utils/wheel-routing'
import { recordInputTape } from '../utils/input-tape'
import { readTerminalThemePayload } from './use-terminal-theme'

// RIS, xterm's in-band reset. A snapshot goes out as ONE chunk that begins with it: xterm parses
// a chunk within one task, so no frame can show the reset buffer before the redraw.
const RIS = new Uint8Array([0x1b, 0x63])

function withReset(data: Uint8Array): Uint8Array {
  const chunk = new Uint8Array(RIS.length + data.length)
  chunk.set(RIS)
  chunk.set(data, RIS.length)
  return chunk
}

interface UseTerminalConnectionOptions {
  /** This view's live stream to its PTY (null while (re)attaching). */
  connection: TerminalConnection | null
  /** True when the PTY was spawned by this attach — gates the initial command. */
  created: boolean
  getTerminalTheme: () => NonNullable<XtermTerminal['options']['theme']>
  /** Run once, after the fresh shell's first output (its prompt). */
  initialCommand?: string
  // Fired once when the daemon reports the session's process exited (its exit
  // frame). The only signal that ends a terminal — see TerminalFrame.
  onTerminalExit?: (sessionId: string) => void
  sessionId: string
  terminal: XtermTerminal | null
  updateSession: (sessionId: string, updates: { title?: string }) => void
}

/**
 * The terminal's I/O: PTY frames into xterm, xterm's input to the PTY, and the
 * host theme to the daemon. Bound to ONE connection object at a time — a
 * re-attach hands in a new one, and everything re-binds to it.
 */
export function useTerminalConnection({
  connection,
  created,
  getTerminalTheme,
  initialCommand,
  onTerminalExit,
  sessionId,
  terminal,
  updateSession,
}: UseTerminalConnectionOptions) {
  const connectionRef = useRef(connection)
  const onTerminalExitRef = useRef(onTerminalExit)
  // Input typed while no connection exists (between a drop and the re-attach)
  // waits here, in order, and goes out the moment one arrives.
  const pendingInputRef = useRef('')
  // The connection the initial command already went to: a re-render that re-binds
  // the same connection must not run it twice.
  const initialCommandSentToRef = useRef<TerminalConnection | null>(null)
  useEffect(() => {
    onTerminalExitRef.current = onTerminalExit
  }, [onTerminalExit])

  const write = useCallback((data: string, origin = 'unknown') => {
    if (!data) return
    recordInputTape('write', origin, data)
    const conn = connectionRef.current
    if (conn?.alive) conn.write(data)
    else pendingInputRef.current += data
  }, [])

  useEffect(() => {
    connectionRef.current = connection
    if (connection?.alive && pendingInputRef.current) {
      connection.write(pendingInputRef.current)
      pendingInputRef.current = ''
    }
  }, [connection])

  useEffect(() => {
    if (!terminal || !connection) return

    // The first frame after an attach is the daemon's redraw of the whole screen:
    // pin to it and repaint once — WKWebView can leave a freshly written WebGL
    // canvas blank until something invalidates it.
    let finalizePending = true
    let initialCommandPending =
      Boolean(initialCommand) && created && initialCommandSentToRef.current !== connection

    const writeFrame = (data: Uint8Array) => {
      const finalize = finalizePending
      finalizePending = false
      terminal.write(data, () => {
        markEnd('terminal.echo')
        if (finalize) {
          terminal.scrollToBottom()
          terminal.refresh(0, terminal.rows - 1)
        }
      })
    }

    const unlisten = connection.listen((frame) => {
      if (frame.exit) {
        onTerminalExitRef.current?.(sessionId)
        return
      }
      if (initialCommandPending) {
        // The fresh shell is attached and has drawn (its first frame): whatever it
        // is doing, the PTY holds this input until it reads it.
        initialCommandPending = false
        initialCommandSentToRef.current = connection
        connection.write(`${initialCommand}\n`)
      }
      if (!frame.snapshot) {
        writeFrame(frame.data)
        return
      }
      // A snapshot replaces the screen with the daemon's ground truth. Its reset rides in band,
      // so the bytes queued before it parse first and the frames after it land on it.
      finalizePending = false
      terminal.write(withReset(frame.data), () => {
        terminal.scrollToBottom()
        terminal.refresh(0, terminal.rows - 1)
      })
    })

    const disposables = [
      terminal.onData((data) => {
        // xterm reports its helper textarea's focus (\x1b[I / \x1b[O) on every pane
        // switch; a shell at its prompt would echo them as garbage. Real WINDOW
        // focus is reported below instead.
        if (data === '\x1b[I' || data === '\x1b[O') return
        markStart('terminal.echo')
        // Keystrokes are forwarded verbatim and never interpreted: whether a typed
        // "exit" ends anything is up to the program reading it (a shell, ssh, a
        // REPL, an agent TUI). The daemon's exit frame reports the outcome.
        write(data, 'onData')
      }),
      terminal.onTitleChange((rawTitle) => {
        // Always write the sanitized result: an empty/rejected title clears any
        // prior one so the tab falls back to its directory/command label.
        updateSession(sessionId, { title: sanitizeTerminalTitle(rawTitle) })
      }),
    ]

    // The host light/dark theme reaches the daemon on (re)attach — so a freshly
    // started app queries the right background — and on every switch — so a
    // DEC-2031-subscribed app re-queries live.
    connection.setTheme(readTerminalThemePayload())
    const unlistenTheme = themeRegistry.onThemeChange(() => {
      terminal.options.theme = getTerminalTheme()
      connection.setTheme(readTerminalThemePayload())
    })

    // Route the wheel by the app's DECLARED mouse-tracking intent (see
    // shouldScrollScrollback): only Shift+wheel on the primary buffer scrolls our
    // own scrollback; everything else is xterm's (native scroll, or the app's).
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

    // Report WINDOW focus to an app that enabled focus reporting (DECSET ?1004).
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
      for (const disposable of disposables) disposable.dispose()
      unlistenTheme()
      unlisten()
    }
  }, [
    connection,
    created,
    getTerminalTheme,
    initialCommand,
    sessionId,
    terminal,
    updateSession,
    write,
  ])

  return { write }
}
