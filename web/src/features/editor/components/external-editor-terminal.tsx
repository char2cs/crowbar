// Crowbar stubs — FUTURE: replace with Go API / WebSocket calls
const invoke = async (_cmd: string, _args?: unknown): Promise<void> => {}
function listen<T = unknown>(
  _event: string,
  _handler: (e: { payload: T }) => void,
): Promise<() => void> {
  return Promise.resolve(() => {})
}
import { ClipboardAddon } from '@xterm/addon-clipboard'
import { FitAddon } from '@xterm/addon-fit'
import { Unicode11Addon } from '@xterm/addon-unicode11'
import { WebLinksAddon } from '@xterm/addon-web-links'
import { Terminal } from '@xterm/xterm'
import { useCallback, useEffect, useRef } from 'react'
import { useEditorSettingsStore } from '@/features/editor/stores/settings-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useSettingsStore } from '@/features/settings/store'
import { useTerminalTheme } from '@/features/terminal/hooks/use-terminal-theme'
import { sanitizeTerminalTitle } from '@/features/terminal/utils/terminal-title'
import { useProjectStore } from '@/features/window/stores/project-store'
import { cn } from '@/utils/cn'
import '@xterm/xterm/css/xterm.css'
import '@/features/terminal/styles/terminal.css'

interface ExternalEditorTerminalProps {
  filePath: string
  fileName: string
  terminalConnectionId: string
  onEditorExit?: () => void
}

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive terminal host — a single external-editor PTY surface with intertwined spawn/resize/teardown effects.
export const ExternalEditorTerminal = ({
  filePath,
  fileName,
  terminalConnectionId,
  onEditorExit,
}: ExternalEditorTerminalProps) => {
  const terminalRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const isInitializingRef = useRef(false)
  const hasExecutedCommandRef = useRef(false)
  const initFitTimeoutRef = useRef<NodeJS.Timeout | null>(null)
  const themeRefreshTimeoutRef = useRef<NodeJS.Timeout | null>(null)
  const resizeRafRef = useRef<number | null>(null)

  const editorFontSize = useEditorSettingsStore((s) => s.fontSize)
  const editorFontFamily = useEditorSettingsStore((s) => s.fontFamily)
  const rootFolderPath = useProjectStore((s) => s.rootFolderPath)
  const settings = useSettingsStore((s) => s.settings)
  const { getTerminalTheme } = useTerminalTheme()

  const updateExternalEditorBufferTitle = useCallback(
    (title: string) => {
      const trimmed = title.trim()
      if (!trimmed || trimmed === 'Default Terminal') return

      const buffers = windowPaneStore.getState().buffers
      const buffer = buffers.find(
        (item) =>
          item.type === 'externalEditor' && item.terminalConnectionId === terminalConnectionId,
      )

      if (!buffer || buffer.name === trimmed) return

      windowPaneStore.setState((state) => ({
        buffers: state.buffers.map((b) =>
          b.type === 'externalEditor' && b.terminalConnectionId === terminalConnectionId
            ? { ...b, name: trimmed }
            : b,
        ),
      }))
    },
    [terminalConnectionId],
  )

  const getEditorCommand = useCallback(
    (path: string): string => {
      const relativePath = rootFolderPath ? path.replace(rootFolderPath, '.') : path

      switch (settings.editorEngine) {
        case 'nvim':
          return `nvim "${relativePath}"`
        case 'helix':
          return `hx "${relativePath}"`
        case 'vim':
          return `vim "${relativePath}"`
        case 'custom':
          return settings.customEditorCommand.replace('$FILE', `"${relativePath}"`)
        default:
          return `nvim "${relativePath}"`
      }
    },
    [settings.editorEngine, settings.customEditorCommand, rootFolderPath],
  )

  const initializeTerminal = useCallback(() => {
    if (!terminalRef.current || xtermRef.current || isInitializingRef.current) {
      return
    }

    isInitializingRef.current = true

    const terminal = new Terminal({
      cursorBlink: true,
      fontSize: editorFontSize,
      fontFamily: `${editorFontFamily}, Menlo, Monaco, "Courier New", monospace`,
      theme: getTerminalTheme(),
      allowProposedApi: true,
      smoothScrollDuration: 100,
      scrollback: 10000,
      convertEol: true,
    })

    const fitAddon = new FitAddon()
    const webLinksAddon = new WebLinksAddon()
    const unicodeAddon = new Unicode11Addon()
    const clipboardAddon = new ClipboardAddon()

    terminal.loadAddon(fitAddon)
    terminal.loadAddon(webLinksAddon)
    terminal.loadAddon(unicodeAddon)
    terminal.loadAddon(clipboardAddon)

    terminal.unicode.activeVersion = '11'

    terminal.open(terminalRef.current)

    if (initFitTimeoutRef.current) {
      clearTimeout(initFitTimeoutRef.current)
    }
    initFitTimeoutRef.current = setTimeout(() => {
      if (fitAddon && terminalRef.current) {
        fitAddon.fit()
      }
      initFitTimeoutRef.current = null
    }, 150)

    xtermRef.current = terminal
    fitAddonRef.current = fitAddon

    terminal.onData((data) => {
      invoke('terminal_write', { id: terminalConnectionId, data }).catch((e) => {
        console.error('Failed to write to external editor terminal:', e)
      })
    })

    terminal.onTitleChange((rawTitle) => {
      const title = sanitizeTerminalTitle(rawTitle)

      if (!title || title === fileName) return
      updateExternalEditorBufferTitle(title)
    })

    terminal.onKey(({ domEvent }) => {
      const e = domEvent

      if ((e.metaKey && e.key === 'Backspace') || (e.ctrlKey && e.key === 'u')) {
        e.preventDefault()
        invoke('terminal_write', { id: terminalConnectionId, data: '\u0015' }).catch((e) => {
          console.error('Failed to write to terminal:', e)
        })
        return
      }

      if (e.metaKey && e.key === 'k') {
        e.preventDefault()
        invoke('terminal_write', { id: terminalConnectionId, data: '\u000c' }).catch((e) => {
          console.error('Failed to write to terminal:', e)
        })
        return
      }

      if (e.altKey && e.key === 'Backspace') {
        e.preventDefault()
        invoke('terminal_write', { id: terminalConnectionId, data: '\u0017' }).catch((e) => {
          console.error('Failed to write to terminal:', e)
        })
        return
      }
    })

    terminal.attachCustomKeyEventHandler((e) => {
      if (e.metaKey && ['Backspace', 'k', 'a', 'e', 'ArrowLeft', 'ArrowRight'].includes(e.key)) {
        return true
      }
      return true
    })

    // Set up event listeners asynchronously
    const setupListeners = async () => {
      try {
        const outputEventName = `pty-output-${terminalConnectionId}`
        const closedEventName = `pty-closed-${terminalConnectionId}`
        const errorEventName = `pty-error-${terminalConnectionId}`

        // Each subscription is an independent event name, so register them
        // concurrently instead of one at a time.
        const [unlisten, closedUnlisten, errorUnlisten] = await Promise.all([
          listen<{ data: string }>(outputEventName, (event) => {
            terminal.write(event.payload.data)
          }),
          listen(closedEventName, () => {
            if (onEditorExit) {
              onEditorExit()
            }
          }),
          listen<{ error: string }>(errorEventName, (event) => {
            console.error('Terminal error:', event.payload.error)
          }),
        ])

        // Store cleanup functions
        ;(terminal as unknown as { _cleanupListeners: () => void })._cleanupListeners = () => {
          unlisten()
          closedUnlisten()
          errorUnlisten()
        }
      } catch (error) {
        console.error('Failed to set up terminal event listeners:', error)
      }
    }

    setupListeners()

    isInitializingRef.current = false

    terminal.focus()

    if (!hasExecutedCommandRef.current) {
      hasExecutedCommandRef.current = true
      const command = getEditorCommand(filePath)
      setTimeout(() => {
        invoke('terminal_write', { id: terminalConnectionId, data: `${command}\n` }).catch((e) => {
          console.error('Failed to execute editor command:', e)
        })
      }, 200)
    }
  }, [
    editorFontSize,
    editorFontFamily,
    getTerminalTheme,
    terminalConnectionId,
    filePath,
    fileName,
    getEditorCommand,
    onEditorExit,
    updateExternalEditorBufferTitle,
  ])

  useEffect(() => {
    initializeTerminal()

    return () => {
      if (initFitTimeoutRef.current) {
        clearTimeout(initFitTimeoutRef.current)
      }
      if (themeRefreshTimeoutRef.current) {
        clearTimeout(themeRefreshTimeoutRef.current)
      }
      if (resizeRafRef.current) {
        cancelAnimationFrame(resizeRafRef.current)
      }

      if (xtermRef.current) {
        // Call cleanup listeners if available
        const cleanup = (xtermRef.current as unknown as { _cleanupListeners?: () => void })
          ._cleanupListeners
        if (cleanup) {
          cleanup()
        }
        xtermRef.current.dispose()
        xtermRef.current = null
      }
    }
  }, [initializeTerminal])

  useEffect(() => {
    if (xtermRef.current) {
      xtermRef.current.options.fontSize = editorFontSize
      if (fitAddonRef.current) {
        fitAddonRef.current.fit()
      }
    }
  }, [editorFontSize])

  useEffect(() => {
    if (xtermRef.current) {
      xtermRef.current.options.fontFamily = `${editorFontFamily}, Menlo, Monaco, "Courier New", monospace`
      if (fitAddonRef.current) {
        fitAddonRef.current.fit()
      }
    }
  }, [editorFontFamily])

  useEffect(() => {
    if (xtermRef.current) {
      if (themeRefreshTimeoutRef.current) {
        clearTimeout(themeRefreshTimeoutRef.current)
      }

      themeRefreshTimeoutRef.current = setTimeout(() => {
        if (xtermRef.current) {
          xtermRef.current.options.theme = getTerminalTheme()
        }
        themeRefreshTimeoutRef.current = null
      }, 50)
    }
    return () => {
      if (themeRefreshTimeoutRef.current) {
        clearTimeout(themeRefreshTimeoutRef.current)
        themeRefreshTimeoutRef.current = null
      }
    }
  }, [settings.theme, getTerminalTheme])

  useEffect(() => {
    const handleResize = () => {
      if (resizeRafRef.current) {
        cancelAnimationFrame(resizeRafRef.current)
      }

      resizeRafRef.current = requestAnimationFrame(() => {
        if (fitAddonRef.current && terminalRef.current) {
          fitAddonRef.current.fit()

          if (xtermRef.current) {
            const rows = xtermRef.current.rows
            const cols = xtermRef.current.cols

            invoke('terminal_resize', {
              id: terminalConnectionId,
              rows,
              cols,
            }).catch((e) => {
              console.error('Failed to resize terminal:', e)
            })
          }
        }
        resizeRafRef.current = null
      })
    }

    const resizeObserver = new ResizeObserver(handleResize)
    if (terminalRef.current) {
      resizeObserver.observe(terminalRef.current)
    }

    window.addEventListener('resize', handleResize)

    return () => {
      resizeObserver.disconnect()
      window.removeEventListener('resize', handleResize)
      if (resizeRafRef.current) {
        cancelAnimationFrame(resizeRafRef.current)
      }
    }
  }, [terminalConnectionId])

  return (
    <div className="flex size-full flex-col bg-transparent">
      <div
        ref={terminalRef}
        className={cn('xterm-container size-full flex-1', 'focus:outline-none')}
        style={{ padding: '8px' }}
      />
    </div>
  )
}
