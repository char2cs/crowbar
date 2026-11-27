import type { TerminalConnection } from '@/lib/crowbar-bridge'
import type { Terminal as XtermTerminal } from '@xterm/xterm'
import { useCallback, useEffect, useRef } from 'react'
import { markStart, markEnd } from '@/lib/perf/instrumentation'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { sanitizeTerminalTitle } from '../utils/terminal-title'
import { shouldScrollScrollback } from '../utils/wheel-routing'
import { recordInputTape } from '../utils/input-tape'
import { readTerminalThemePayload } from './use-terminal-theme'

const RIS = new Uint8Array([0x1b, 0x63])

function withReset(data: Uint8Array): Uint8Array {
  const chunk = new Uint8Array(RIS.length + data.length)
  chunk.set(RIS)
  chunk.set(data, RIS.length)
  return chunk
}

interface UseTerminalConnectionOptions {
  connection: TerminalConnection | null
  created: boolean
  getTerminalTheme: () => NonNullable<XtermTerminal['options']['theme']>
  initialCommand?: string
  onTerminalExit?: (sessionId: string) => void
  sessionId: string
  terminal: XtermTerminal | null
  updateSession: (sessionId: string, updates: { title?: string }) => void
}

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
  const pendingInputRef = useRef('')
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
        initialCommandPending = false
        initialCommandSentToRef.current = connection
        connection.write(`${initialCommand}\n`)
      }
      if (!frame.snapshot) {
        writeFrame(frame.data)
        return
      }
      finalizePending = false
      terminal.write(withReset(frame.data), () => {
        terminal.scrollToBottom()
        terminal.refresh(0, terminal.rows - 1)
      })
    })

    const disposables = [
      terminal.onData((data) => {
        if (data === '\x1b[I' || data === '\x1b[O') return
        markStart('terminal.echo')
        write(data, 'onData')
      }),
      terminal.onTitleChange((rawTitle) => {
        updateSession(sessionId, { title: sanitizeTerminalTitle(rawTitle) })
      }),
    ]

    connection.setTheme(readTerminalThemePayload())
    const unlistenTheme = themeRegistry.onThemeChange(() => {
      terminal.options.theme = getTerminalTheme()
      connection.setTheme(readTerminalThemePayload())
    })

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
