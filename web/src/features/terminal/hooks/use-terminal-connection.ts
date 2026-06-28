import { terminalWrite, terminalResize, terminalClose, terminalListen } from '@/lib/crowbar-bridge'
import type { IDisposable, Terminal as XtermTerminal } from '@xterm/xterm'
import { useEffect, useRef } from 'react'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { parseOSC7 } from '../utils/osc-parser'
import { useTerminalWriteBuffer } from './use-terminal-write-buffer'

const ESCAPE_CODE = 27
const BEL_CODE = 7
const DELETE_CODE = 127
const C1_ESCAPE_CODE = 155

const isAsciiLetter = (charCode: number) =>
  (charCode >= 65 && charCode <= 90) || (charCode >= 97 && charCode <= 122)

const stripTerminalControlSequences = (rawTitle: string) => {
  let title = ''

  for (let index = 0; index < rawTitle.length; index += 1) {
    const charCode = rawTitle.charCodeAt(index)

    if (charCode === ESCAPE_CODE) {
      const nextChar = rawTitle[index + 1]

      if (nextChar === '[') {
        index += 2
        while (index < rawTitle.length && !isAsciiLetter(rawTitle.charCodeAt(index))) {
          index += 1
        }
        continue
      }

      if (nextChar === ']') {
        index += 2
        while (index < rawTitle.length && rawTitle.charCodeAt(index) !== BEL_CODE) {
          index += 1
        }
        continue
      }

      continue
    }

    if (charCode <= 31 || charCode === DELETE_CODE || charCode === C1_ESCAPE_CODE) {
      continue
    }

    title += rawTitle[index]
  }

  return title.trim()
}

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
        const shortcuts: Record<string, string> = {
          'meta+Backspace': '\u0015',
          'ctrl+u': '\u0015',
          'meta+k': '\u000c',
          'alt+Backspace': '\u0017',
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

    disposables.push(
      terminal.onSelectionChange(() => {
        const selection = terminal.getSelection()
        if (selection) updateSession(sessionId, { selection })
      }),
    )

    disposables.push(
      terminal.onTitleChange((rawTitle) => {
        const title = stripTerminalControlSequences(rawTitle)
        if (title) {
          updateSession(sessionId, { title })
        }
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

    // When a TUI app like CC enables mouse tracking, xterm forwards wheel events
    // to the PTY as SGR sequences instead of scrolling its viewport. Intercept in
    // capture phase so we can scroll the viewport ourselves while the app is active,
    // without breaking apps that don't use mouse tracking.
    const wheelContainer = terminal.element?.parentElement
    const handleWheel = (event: WheelEvent) => {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      const mode = (terminal as any).modes?.mouseTrackingMode as string | undefined
      if (!mode || mode === 'none') return
      event.preventDefault()
      event.stopPropagation()
      // xterm sign convention: positive = scroll toward older content (up).
      // Wheel deltaY is opposite: negative = user scrolled up.
      const lines = Math.ceil(Math.abs(event.deltaY) / 40) * (event.deltaY < 0 ? 1 : -1)
      terminal.scrollLines(lines * 3)
    }
    wheelContainer?.addEventListener('wheel', handleWheel, { capture: true, passive: false })

    return () => {
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
