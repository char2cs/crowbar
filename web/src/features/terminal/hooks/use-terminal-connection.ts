import { terminalWrite, terminalResize, terminalClose, terminalListen } from '@/lib/crowbar-bridge'
import type { IDisposable, Terminal as XtermTerminal } from '@xterm/xterm'
import { useEffect, useRef } from 'react'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { parseOSC7 } from '../utils/osc-parser'
import { sanitizeTerminalTitle } from '../utils/terminal-title'
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
  const outputFlushFrameRef = useRef<number | null>(null)
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

  useEffect(() => {
    return () => {
      if (outputFlushFrameRef.current !== null) {
        cancelAnimationFrame(outputFlushFrameRef.current)
      }
      outputBufferRef.current = ''
    }
  }, [])

  useEffect(() => {
    if (!terminal || !isInitialized || !connectionId) return

    const disposables: IDisposable[] = []

    const flushOutputBuffer = () => {
      outputFlushFrameRef.current = null
      const pendingOutput = outputBufferRef.current
      if (!pendingOutput) return

      outputBufferRef.current = ''
      terminal.write(pendingOutput)

      const newDirectory = parseOSC7(pendingOutput)
      if (newDirectory) updateSession(sessionId, { currentDirectory: newDirectory })
    }

    const scheduleOutputFlush = () => {
      if (outputFlushFrameRef.current !== null) return
      outputFlushFrameRef.current = window.requestAnimationFrame(flushOutputBuffer)
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

        const activeConnectionId = currentConnectionIdRef.current || connectionId
        const hasNewline = data.includes('\n') || data.includes('\r')

        if (hasNewline) {
          currentInputLineRef.current += data
          if (/^\s*exit\s*$/i.test(currentInputLineRef.current.trim())) {
            explicitExitRequestedRef.current = true
            currentInputLineRef.current = ''
            write(data)
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

        write(data)
      }),
    )

    disposables.push(
      terminal.onKey(({ domEvent: event }) => {
        // NOTE: do NOT add 'alt+Backspace' here. The terminal is created with
        // macOptionIsMeta:true, so xterm natively emits ESC+DEL (backward-kill-
        // word) for opt+backspace via onData. A shortcut entry here would fire a
        // SECOND delete-word sequence (Ctrl-W), deleting two words per keystroke.
        const shortcuts: Record<string, string> = {
          'meta+Backspace': '\u0015',
          'ctrl+u': '\u0015',
          'meta+k': '\u000c',
          'meta+a': '\u0001',
          'meta+e': '\u0005',
        }

        const key = `${event.metaKey ? 'meta+' : ''}${event.ctrlKey ? 'ctrl+' : ''}${event.altKey ? 'alt+' : ''}${event.key}`
        if (shortcuts[key]) {
          event.preventDefault()
          write(shortcuts[key])
          return
        }

        // Modifier+Enter: send CSI u sequences so TUI apps can distinguish them
        if (event.key === 'Enter') {
          if (event.shiftKey) {
            event.preventDefault()
            write('\x1b[13;2u') // Shift+Enter
            return
          }
          if (event.altKey) {
            event.preventDefault()
            write('\x1b[13;3u') // Alt+Enter
            return
          }
        }

        // Shift+Tab: send reverse-tab escape sequence
        if (event.key === 'Tab' && event.shiftKey) {
          event.preventDefault()
          write('\x1b[Z')
          return
        }

        if (event.metaKey && (event.key === 'ArrowLeft' || event.key === 'ArrowRight')) {
          event.preventDefault()
          write(event.key === 'ArrowLeft' ? '\u0001' : '\u0005')
          return
        }

        if (event.altKey && (event.key === 'ArrowLeft' || event.key === 'ArrowRight')) {
          event.preventDefault()
          write(event.key === 'ArrowLeft' ? '\u001bb' : '\u001bf')
        }
      }),
    )

    disposables.push(
      terminal.onResize(({ cols, rows }) => {
        void terminalResize(connectionId, rows, cols).catch(() => {})
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

    const unlistenThemeChange = themeRegistry.onThemeChange(() => {
      terminal.options.theme = getTerminalTheme()
    })

    // Use terminalListen from crowbar-bridge for PTY output
    const unlistenOutput = terminalListen(connectionId, (data: string) => {
      outputBufferRef.current += data
      scheduleOutputFlush()
    })

    // Wheel handling depends on what the app is doing:
    //
    //  - No mouse tracking → xterm scrolls its own viewport natively. Leave it.
    //  - Mouse tracking + ALTERNATE screen (full-screen TUIs: Claude Code, vim,
    //    less) → the app owns its history/scrolling and there is NO scrollback to
    //    scroll. Let xterm forward the wheel to the app (its default) so the app
    //    scrolls. (Intercepting here and calling scrollLines() on the empty alt
    //    buffer is a no-op — it makes the wheel appear completely dead, which is
    //    exactly the "can't scroll Claude" bug.)
    //  - Mouse tracking + NORMAL buffer → xterm would forward the wheel to the
    //    app, but the user expects to scroll our scrollback, so intercept and do
    //    it ourselves. scrollLines(): negative = up (older history), positive =
    //    down; wheel deltaY is already negative on scroll-up, so signs align.
    const wheelContainer = terminal.element?.parentElement
    const handleWheel = (event: WheelEvent) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const mode = (terminal as any).modes?.mouseTrackingMode as string | undefined
      if (!mode || mode === 'none') return
      if (terminal.buffer.active.type === 'alternate') return
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
      if ((terminal as any).modes?.sendFocusMode) write(seq)
    }
    const handleWindowFocus = () => emitFocusReport('\x1b[I')
    const handleWindowBlur = () => emitFocusReport('\x1b[O')
    window.addEventListener('focus', handleWindowFocus)
    window.addEventListener('blur', handleWindowBlur)

    return () => {
      window.removeEventListener('focus', handleWindowFocus)
      window.removeEventListener('blur', handleWindowBlur)
      wheelContainer?.removeEventListener('wheel', handleWheel, true)
      if (outputFlushFrameRef.current !== null) {
        cancelAnimationFrame(outputFlushFrameRef.current)
        flushOutputBuffer()
      }
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
      write(`${initialCommand}\n`)
    }, 300)

    return () => window.clearTimeout(timeoutId)
  }, [connectionId, initialCommand, reuseExistingConnection, write])

  return {
    currentConnectionIdRef,
    writeBuffered: write,
  }
}
