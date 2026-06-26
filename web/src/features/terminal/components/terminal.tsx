import { terminalCreate, terminalResize } from '@/lib/crowbar-bridge'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

// Create a PTY session against the active workspace on the Go daemon.
const createTerminal = async (config: Record<string, unknown>): Promise<string> => {
  const wsId = getActiveWorkspaceId()
  if (!wsId) throw new Error('no active workspace for terminal')
  return terminalCreate(wsId, config.profileId as string | undefined)
}
import type { ISearchOptions } from '@xterm/addon-search'
import { Terminal } from '@xterm/xterm'
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { useSettingsStore } from '@/features/settings/store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { useProjectStore } from '@/features/window/stores/project-store'
import { extractDroppedFilePaths } from '@/features/file-system/utils/file-system-dropped-paths'
import { primitiveConfirm } from '@/components/ui/primitive-dialog-service'
import {
  createTerminalAddons,
  injectLinkStyles,
  loadWebLinksAddon,
  removeLinkStyles,
  type TerminalAddons,
} from '../hooks/use-terminal-addons'
import { useTerminalConnection } from '../hooks/use-terminal-connection'
import { useTerminalTheme } from '../hooks/use-terminal-theme'
import { useTerminalStore } from '../stores/terminal-store'
import { formatDroppedPathsForTerminal } from '../utils/terminal-file-drop'
import { analyzeTerminalPaste } from '../utils/paste-guard'
import { resolveTerminalFont } from '../utils/resolve-font'
import { TerminalSearch, type TerminalSearchOptions } from './terminal-search'
import '@xterm/xterm/css/xterm.css'
import '../styles/terminal.css'

interface XtermTerminalProps {
  sessionId: string
  isActive: boolean
  isVisible?: boolean
  onReady?: () => void
  onTerminalRef?: (ref: { focus: () => void; showSearch: () => void; terminal: Terminal }) => void
  onTerminalExit?: (sessionId: string) => void
  initialCommand?: string
  workingDirectory?: string
  remoteConnectionId?: string
}

export const XtermTerminal: React.FC<XtermTerminalProps> = ({
  sessionId,
  isActive,
  isVisible = true,
  onReady,
  onTerminalRef,
  onTerminalExit,
  initialCommand,
  workingDirectory,
  remoteConnectionId,
}) => {
  const terminalContainerRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<Terminal | null>(null)
  const addonsRef = useRef<TerminalAddons | null>(null)
  const [isInitialized, setIsInitialized] = useState(false)
  const [isSearchVisible, setIsSearchVisible] = useState(false)
  const [searchResults, setSearchResults] = useState({ current: 0, total: 0 })
  const isInitializingRef = useRef(false)
  const pasteGuardAttachedRef = useRef(false)

  const updateSession = useTerminalStore((s) => s.updateSession)
  const getSession = useTerminalStore((s) => s.getSession)
  const session = getSession(sessionId)
  const connectionId = session?.connectionId
  const hadExistingConnectionOnMountRef = useRef(Boolean(session?.connectionId))

  const {
    theme: terminalThemeId,
    terminalFontFamily,
    terminalFontSize,
    terminalLineHeight,
    terminalLetterSpacing,
    terminalScrollback,
    terminalCursorStyle,
    terminalCursorBlink,
    terminalCursorWidth,
  } = useSettingsStore((state) => state.settings)
  const zoomLevel = useZoomStore.use.terminalZoomLevel()
  const rootFolderPath = useProjectStore((s) => s.rootFolderPath)
  const { getTerminalTheme } = useTerminalTheme()
  const effectiveTerminalFontSize = Math.round(terminalFontSize * zoomLevel * 10) / 10
  const effectiveTerminalLetterSpacing = terminalLetterSpacing * zoomLevel
  const effectiveTerminalCursorWidth = Math.max(1, Math.round(terminalCursorWidth * zoomLevel))

  const fitTerminal = useCallback((attempts = 1) => {
    let attempt = 0
    let rafId: number | null = null

    const runFit = () => {
      const container = terminalContainerRef.current
      const addons = addonsRef.current
      if (!container || !addons) return

      const rect = container.getBoundingClientRect()
      // Use rect dimensions only — offsetParent is null inside position:fixed
      // ancestors (popup windows, overlays) even when the element is visible.
      if (rect.width <= 0 || rect.height <= 0) {
        if (attempt < attempts - 1) {
          attempt += 1
          rafId = requestAnimationFrame(runFit)
        }
        return
      }

      const term = xtermRef.current
      const prevRows = term?.rows ?? 0
      const prevCols = term?.cols ?? 0
      addons.fitAddon.fit()

      if (attempt < attempts - 1) {
        attempt += 1
        rafId = requestAnimationFrame(runFit)
      } else if (term && (term.rows !== prevRows || term.cols !== prevCols)) {
        // Final pass: only force a full WebGL repaint when dimensions actually
        // changed. An unconditional refresh(0, rows-1) on every fit call causes
        // expensive full-canvas repaints that compound with WKWebView's CA layer
        // re-rasterization, making the app feel sluggish after terminal updates.
        term.refresh(0, term.rows - 1)
      }
    }

    rafId = requestAnimationFrame(runFit)

    return () => {
      if (rafId !== null) cancelAnimationFrame(rafId)
    }
  }, [])

  const { currentConnectionIdRef, writeBuffered } = useTerminalConnection({
    connectionId,
    getTerminalTheme,
    initialCommand,
    isInitialized,
    onTerminalExit,
    remoteConnectionId,
    reuseExistingConnection: hadExistingConnectionOnMountRef.current,
    sessionId,
    terminal: xtermRef.current,
    updateSession,
  })

  const handleTerminalFileDrop = useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      const paths = extractDroppedFilePaths(event.dataTransfer)
      const text = formatDroppedPathsForTerminal(paths)
      if (!text) return

      event.preventDefault()
      event.stopPropagation()
      writeBuffered(text)
      requestAnimationFrame(() => xtermRef.current?.focus())
    },
    [writeBuffered],
  )

  const handleTerminalDragOver = useCallback((event: React.DragEvent<HTMLDivElement>) => {
    if (!Array.from(event.dataTransfer.types).includes('Files')) return
    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'copy'
  }, [])

  const initializeTerminal = useCallback(async () => {
    const container = terminalContainerRef.current
    if (!container || isInitialized || isInitializingRef.current) return

    const rect = container.getBoundingClientRect()
    if (rect.width <= 0 || rect.height <= 0) return

    isInitializingRef.current = true
    const resolved = await resolveTerminalFont(terminalFontFamily, effectiveTerminalFontSize)
    // Skip the font-settle delay when reconnecting to an existing PTY — font is
    // already loaded and rasterized, so the wait is pure dead time that makes
    // the blank gap on pane splits/moves visibly long.
    if (!hadExistingConnectionOnMountRef.current) {
      await new Promise((resolve) => setTimeout(resolve, 100))
    }

    if (!terminalContainerRef.current) {
      isInitializingRef.current = false
      return
    }

    try {
      const terminal = new Terminal({
        fontFamily: resolved.fontFamily,
        fontSize: effectiveTerminalFontSize,
        lineHeight: terminalLineHeight,
        letterSpacing: effectiveTerminalLetterSpacing,
        cursorBlink: terminalCursorBlink,
        cursorStyle: terminalCursorStyle,
        cursorWidth: effectiveTerminalCursorWidth,
        allowProposedApi: true,
        allowTransparency: true,
        theme: getTerminalTheme(),
        scrollback: terminalScrollback,
        convertEol: false,
        macOptionIsMeta: true,
        rightClickSelectsWord: false,
      })

      const addons = createTerminalAddons(terminal, {
        skipWebGL: resolved.skipWebGL,
      })

      terminal.open(terminalContainerRef.current)
      terminal.attachCustomKeyEventHandler((event) => {
        if (event.ctrlKey && !event.metaKey) return true
        if (
          event.metaKey &&
          ['Backspace', 'k', 'a', 'e', 'f', 'ArrowLeft', 'ArrowRight'].includes(event.key)
        ) {
          return true
        }
        return !event.metaKey
      })

      if (terminal.textarea) {
        terminal.textarea.spellcheck = false
        terminal.textarea.addEventListener('beforeinput', (event) => {
          if (event.inputType === 'insertReplacementText' || event.inputType === 'insertFromDrop') {
            const text = event.dataTransfer?.getData('text/plain') ?? event.data
            if (!text || !currentConnectionIdRef.current) return

            event.preventDefault()
            writeBuffered(text)
          }
        })
      }

      // Paste interception MUST be on the container in the capture phase.
      // xterm registers its own textarea paste listener inside terminal.open()
      // (before ours), and same-element listeners fire in registration order
      // regardless of the capture flag — a textarea-level listener runs after
      // xterm has already written the paste to the PTY (BUG-016). Capturing on
      // the ancestor runs first; stopPropagation keeps xterm's handler out.
      // The ref guard makes an init retry (e.g. PTY create failure) not stack
      // a second listener.
      if (!pasteGuardAttachedRef.current) {
        pasteGuardAttachedRef.current = true
        terminalContainerRef.current.addEventListener(
          'paste',
          (event) => {
            const text = event.clipboardData?.getData('text/plain')
            if (!text || !currentConnectionIdRef.current) return

            event.preventDefault()
            event.stopPropagation()

            const { normalizedText, lineCount, requiresConfirmation } = analyzeTerminalPaste(text)

            if (requiresConfirmation) {
              void primitiveConfirm(
                `Paste ${lineCount} lines into the terminal? This may execute multiple commands.`,
                { title: 'Paste Into Terminal', confirmLabel: 'Paste' },
              ).then((confirmed) => {
                if (confirmed) writeBuffered(normalizedText)
              })
              return
            }

            writeBuffered(normalizedText)
          },
          true,
        )
      }

      loadWebLinksAddon(terminal)
      terminal.unicode.activeVersion = '11'
      injectLinkStyles(sessionId, terminalContainerRef.current.id || `terminal-${sessionId}`)

      xtermRef.current = terminal
      addonsRef.current = addons

      // FitAddon.proposeDimensions() returns undefined until the renderer has computed
      // cell metrics (css.cell.width/height > 0). With WebGL this takes at least one
      // rAF after open(). Poll here so the PTY is created with the correct dimensions
      // instead of xterm's default 80×24.
      const maxWaitFrames = 20
      for (let waitFrame = 0; waitFrame < maxWaitFrames; waitFrame++) {
        if (addons.fitAddon.proposeDimensions()) break
        await new Promise<void>((resolve) => {
          requestAnimationFrame(() => resolve())
        })
        if (!terminalContainerRef.current) {
          isInitializingRef.current = false
          return
        }
      }
      addons.fitAddon.fit()

      const existingSession = getSession(sessionId)

      // If the session already has a live PTY connection (e.g., component
      // remounted after a pane split or tab move), reuse the existing
      // connection instead of killing the running process.
      let activeConnectionId: string
      if (existingSession?.connectionId) {
        activeConnectionId = existingSession.connectionId
      } else {
        // §3/Open-Q2: do NOT fall back to the synthetic rootFolderPath
        // ('/repos/<id>') — it is not a real filesystem path and the daemon
        // already defaults a new PTY's cwd to the workspace worktree. Only pass
        // an explicit working directory when one is actually known.
        const targetDirectory = workingDirectory || existingSession?.currentDirectory
        // parseRemotePath does not expose connectionId in the stub; use only the passed remoteConnectionId
        const effectiveRemoteConnectionId = remoteConnectionId || undefined

        activeConnectionId = await createTerminal({
          working_directory: targetDirectory || undefined,
          shell: existingSession?.shell || undefined,
          rows: terminal.rows,
          cols: terminal.cols,
        })

        updateSession(sessionId, {
          connectionId: activeConnectionId,
          currentDirectory: targetDirectory ?? undefined,
          remoteConnectionId: effectiveRemoteConnectionId,
        })
      }

      // Force-sync the PTY to xterm's freshly-fitted size for BOTH paths:
      // - New session: the backend spawns the PTY at its default 80×24 and does
      //   not honor the create-time rows/cols, so without this full-screen TUIs
      //   (cmatrix, vim, htop) only use 24 rows.
      // - Remount (pane split / tab move): the PTY keeps its old size, and the
      //   post-create fit() is a no-op for xterm's dimensions, so no onResize
      //   fires. Pushing the size here makes the running process redraw.
      void terminalResize(activeConnectionId, terminal.rows, terminal.cols).catch(() => {})

      // No snapshot replay: xterm is portaled and never remounts mid-session,
      // so the live PTY redrawing via SIGWINCH is the source of truth.

      setIsInitialized(true)
      isInitializingRef.current = false

      // Re-fit after connection is established so onResize can notify the PTY
      fitTerminal(3)

      window.dispatchEvent(
        new CustomEvent('terminal-ready', {
          detail: { terminalId: sessionId, connectionId: activeConnectionId },
        }),
      )

      onTerminalRef?.({
        focus: () => terminal.focus(),
        showSearch: () => setIsSearchVisible(true),
        terminal,
      })
      onReady?.()
    } catch (error) {
      console.error('Failed to initialize terminal:', error)
      isInitializingRef.current = false
    }
  }, [
    currentConnectionIdRef,
    fitTerminal,
    getSession,
    getTerminalTheme,
    isInitialized,
    onReady,
    onTerminalRef,
    rootFolderPath,
    remoteConnectionId,
    sessionId,
    terminalCursorBlink,
    terminalCursorStyle,
    terminalCursorWidth,
    terminalFontFamily,
    effectiveTerminalCursorWidth,
    effectiveTerminalFontSize,
    effectiveTerminalLetterSpacing,
    terminalLineHeight,
    terminalScrollback,
    updateSession,
    workingDirectory,
    writeBuffered,
  ])

  useEffect(() => {
    if (!xtermRef.current) return
    xtermRef.current.options.theme = getTerminalTheme()
    const timer = setTimeout(() => {
      xtermRef.current?.refresh(0, xtermRef.current.rows - 1)
      fitTerminal(4)
    }, 10)
    return () => clearTimeout(timer)
  }, [terminalThemeId, getTerminalTheme, fitTerminal])

  useEffect(() => {
    if (!xtermRef.current || !addonsRef.current) return

    let cancelled = false

    const applyFontChange = async () => {
      const resolved = await resolveTerminalFont(terminalFontFamily, effectiveTerminalFontSize)
      if (cancelled || !xtermRef.current || !addonsRef.current) return

      if (resolved.skipWebGL) {
        addonsRef.current.webglAddon?.dispose()
      }

      xtermRef.current.options.fontFamily = resolved.fontFamily
      xtermRef.current.options.fontSize = effectiveTerminalFontSize
      xtermRef.current.options.lineHeight = terminalLineHeight
      xtermRef.current.options.letterSpacing = effectiveTerminalLetterSpacing
      xtermRef.current.options.scrollback = terminalScrollback
      xtermRef.current.options.cursorBlink = terminalCursorBlink
      xtermRef.current.options.cursorStyle = terminalCursorStyle
      xtermRef.current.options.cursorWidth = effectiveTerminalCursorWidth

      fitTerminal(4)
      xtermRef.current.refresh(0, xtermRef.current.rows - 1)
    }

    void applyFontChange()

    return () => {
      cancelled = true
    }
  }, [
    terminalFontFamily,
    effectiveTerminalCursorWidth,
    effectiveTerminalFontSize,
    effectiveTerminalLetterSpacing,
    terminalLineHeight,
    terminalScrollback,
    terminalCursorBlink,
    terminalCursorStyle,
    fitTerminal,
  ])

  useEffect(() => {
    if (!isVisible) return

    let mounted = true
    const initTimer = setTimeout(() => {
      if (mounted && !isInitialized && !isInitializingRef.current) {
        void initializeTerminal()
      }
    }, 200)

    return () => {
      mounted = false
      clearTimeout(initTimer)
      removeLinkStyles(sessionId)
    }
  }, [initializeTerminal, isInitialized, isVisible, sessionId])

  useEffect(() => {
    if (isInitialized || !isVisible || !terminalContainerRef.current) return

    let rafId: number | null = null
    const container = terminalContainerRef.current

    const attemptInitialize = () => {
      if (isInitialized || isInitializingRef.current) return

      const rect = container.getBoundingClientRect()
      if (rect.width <= 0 || rect.height <= 0) {
        rafId = requestAnimationFrame(attemptInitialize)
        return
      }

      void initializeTerminal()
    }

    rafId = requestAnimationFrame(attemptInitialize)

    return () => {
      if (rafId !== null) cancelAnimationFrame(rafId)
    }
  }, [initializeTerminal, isInitialized, isVisible])

  // Dispose only the xterm UI on unmount. The PTY process is owned by the
  // buffer store and killed in closeBuffer (via killTerminalSession) when the
  // user actually closes the tab — NOT here. This prevents pane splits, tab
  // moves, and other layout changes from killing running terminal processes.
  useEffect(() => {
    return () => {
      if (xtermRef.current) {
        xtermRef.current.dispose()
        xtermRef.current = null
        addonsRef.current = null
      }
      pasteGuardAttachedRef.current = false
    }
  }, [])

  // XtermTerminal stays mounted while slots move between panes. When a new
  // slot owner provides a fresh ref callback, hand the live terminal handle to
  // it even though initialization does not re-run.
  useEffect(() => {
    const terminal = xtermRef.current
    if (!isInitialized || !terminal || !onTerminalRef) return

    onTerminalRef({
      focus: () => terminal.focus(),
      showSearch: () => setIsSearchVisible(true),
      terminal,
    })
  }, [isInitialized, onTerminalRef])

  // Listen for portal-target changes from TerminalHost; force a fit + repaint
  // so PTY/xterm dims match the new slot before any TUI relies on them.
  useEffect(() => {
    if (!isInitialized) return
    const handler = (event: Event) => {
      const detail = (event as CustomEvent<{ sessionId: string }>).detail
      if (!detail || detail.sessionId !== sessionId) return
      fitTerminal(4)
      const term = xtermRef.current
      if (term) term.refresh(0, term.rows - 1)
    }
    window.addEventListener('athas-terminal-refit', handler)
    return () => window.removeEventListener('athas-terminal-refit', handler)
  }, [fitTerminal, isInitialized, sessionId])

  useEffect(() => {
    if (!addonsRef.current || !terminalContainerRef.current || !isInitialized) return

    // Mirror Monaco's pattern: suppress fitting during pane/sidebar drags
    // (data-pane-resizing attribute) and do one final fit on pane-resize-end.
    // Without this, fitAddon.fit() + terminalResize() IPC fires every frame
    // during drag — far heavier than editor.layout() and causes canvas glitches.
    let rafId: number | null = null
    let needsFitAfterResize = false

    const resizeObserver = new ResizeObserver(() => {
      if (document.documentElement.hasAttribute('data-pane-resizing')) {
        needsFitAfterResize = true
        return
      }
      if (rafId) cancelAnimationFrame(rafId)
      rafId = requestAnimationFrame(() => {
        rafId = null
        const container = terminalContainerRef.current
        if (!addonsRef.current || !container) return

        const rect = container.getBoundingClientRect()
        if (rect.width > 0 && rect.height > 0) {
          fitTerminal(3)
        }
      })
    })

    const handlePaneResizeEnd = () => {
      if (!needsFitAfterResize) return
      needsFitAfterResize = false
      if (rafId !== null) cancelAnimationFrame(rafId)
      rafId = requestAnimationFrame(() => {
        rafId = null
        const container = terminalContainerRef.current
        if (!addonsRef.current || !container) return
        const rect = container.getBoundingClientRect()
        if (rect.width > 0 && rect.height > 0) {
          fitTerminal(3)
        }
      })
    }
    window.addEventListener('pane-resize-end', handlePaneResizeEnd)

    resizeObserver.observe(terminalContainerRef.current)
    const cleanupFit = fitTerminal(3)

    return () => {
      resizeObserver.disconnect()
      if (rafId) cancelAnimationFrame(rafId)
      window.removeEventListener('pane-resize-end', handlePaneResizeEnd)
      cleanupFit?.()
    }
  }, [fitTerminal, isInitialized])

  useEffect(() => {
    if (!isActive || !isVisible || !xtermRef.current || !isInitialized) return

    let cancelled = false

    // Fit the terminal first to recalculate dimensions after display:none → display:flex
    const cleanupFit = fitTerminal(6)

    // Focus with verified retry — wait for layout to fully settle after tab switch
    const ensureFocus = (attempt: number) => {
      if (cancelled || !xtermRef.current || attempt >= 8) return

      xtermRef.current.focus()

      // Verify the textarea actually received focus
      requestAnimationFrame(() => {
        if (cancelled || !xtermRef.current) return
        const textarea = xtermRef.current.textarea
        if (textarea && document.activeElement !== textarea) {
          ensureFocus(attempt + 1)
        }
      })
    }

    // Wait 2 frames for DOM layout to settle after display change, then focus
    requestAnimationFrame(() => {
      requestAnimationFrame(() => {
        if (!cancelled) ensureFocus(0)
      })
    })

    return () => {
      cancelled = true
      cleanupFit?.()
    }
  }, [isActive, isInitialized, isVisible, fitTerminal])

  useEffect(() => {
    if (!isInitialized || !addonsRef.current) return

    const disposable = addonsRef.current.searchAddon.onDidChangeResults(
      ({ resultIndex, resultCount }) => {
        setSearchResults({
          current: resultCount > 0 && resultIndex >= 0 ? resultIndex + 1 : 0,
          total: resultCount,
        })
      },
    )

    return () => disposable.dispose()
  }, [isInitialized])

  const handleZoom = useCallback(
    (delta: number) => {
      const newSize = Math.min(Math.max(terminalFontSize + delta, 8), 32)
      useSettingsStore.getState().updateSetting('terminalFontSize', newSize)
      if (xtermRef.current) {
        xtermRef.current.options.fontSize = newSize
        fitTerminal(4)
      }
    },
    [fitTerminal, terminalFontSize],
  )

  const handleZoomReset = useCallback(() => {
    useSettingsStore.getState().updateSetting('terminalFontSize', 14)
    if (xtermRef.current) {
      xtermRef.current.options.fontSize = 14
      fitTerminal(4)
    }
  }, [fitTerminal])

  const getSearchOptions = useCallback((options: TerminalSearchOptions): ISearchOptions => {
    const rootStyles = getComputedStyle(document.documentElement)
    const selected = rootStyles.getPropertyValue('--color-selected').trim() || '#3b82f6'
    const accent = rootStyles.getPropertyValue('--color-accent').trim() || '#60a5fa'
    const border = rootStyles.getPropertyValue('--color-border').trim() || '#4b5563'

    return {
      caseSensitive: options.caseSensitive,
      wholeWord: options.wholeWord,
      regex: options.regex,
      decorations: {
        matchBackground: selected,
        matchBorder: border,
        matchOverviewRuler: selected,
        activeMatchBackground: accent,
        activeMatchBorder: border,
        activeMatchColorOverviewRuler: accent,
      },
    }
  }, [])

  const clearSearch = useCallback(() => {
    addonsRef.current?.searchAddon.clearDecorations()
    xtermRef.current?.clearSelection()
    setSearchResults({ current: 0, total: 0 })
  }, [])

  useEffect(() => {
    if (!isActive) return

    const handleKeyDown = (event: KeyboardEvent) => {
      const isTerminalFocused =
        terminalContainerRef.current?.contains(event.target as Node) ||
        terminalContainerRef.current?.contains(document.activeElement)
      const key = event.key.toLowerCase()

      if (
        (event.ctrlKey || event.metaKey) &&
        key === 'f' &&
        (isTerminalFocused || isSearchVisible)
      ) {
        event.preventDefault()
        event.stopPropagation()
        setIsSearchVisible(true)
      }

      if (event.key === 'Escape' && isSearchVisible) {
        event.preventDefault()
        setIsSearchVisible(false)
        clearSearch()
        xtermRef.current?.focus()
      }

      if (isTerminalFocused && (event.ctrlKey || event.metaKey)) {
        if (event.key === '+' || event.key === '=') {
          event.preventDefault()
          handleZoom(2)
        } else if (event.key === '-') {
          event.preventDefault()
          handleZoom(-2)
        } else if (event.key === '0') {
          event.preventDefault()
          handleZoomReset()
        }
      }
    }

    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [clearSearch, handleZoom, handleZoomReset, isActive, isSearchVisible])

  const handleSearch = useCallback(
    (term: string, options: TerminalSearchOptions) => {
      if (!term || !addonsRef.current) {
        clearSearch()
        return
      }

      const found = addonsRef.current.searchAddon.findNext(term, {
        ...getSearchOptions(options),
        incremental: true,
      })

      if (!found) {
        setSearchResults({ current: 0, total: 0 })
      }
    },
    [clearSearch, getSearchOptions],
  )

  const handleSearchNext = useCallback(
    (term: string, options: TerminalSearchOptions) => {
      if (!term || !addonsRef.current) return
      addonsRef.current.searchAddon.findNext(term, getSearchOptions(options))
    },
    [getSearchOptions],
  )

  const handleSearchPrevious = useCallback(
    (term: string, options: TerminalSearchOptions) => {
      if (!term || !addonsRef.current) return
      addonsRef.current.searchAddon.findPrevious(term, getSearchOptions(options))
    },
    [getSearchOptions],
  )

  const handleSearchClose = useCallback(() => {
    setIsSearchVisible(false)
    clearSearch()
    xtermRef.current?.focus()
  }, [clearSearch])

  React.useImperativeHandle(
    getSession(sessionId)?.ref,
    () => ({
      terminal: xtermRef.current,
      searchAddon: addonsRef.current?.searchAddon,
      focus: () => xtermRef.current?.focus(),
      showSearch: () => setIsSearchVisible(true),
      blur: () => xtermRef.current?.blur(),
      clear: () => xtermRef.current?.clear(),
      selectAll: () => xtermRef.current?.selectAll(),
      clearSelection: () => xtermRef.current?.clearSelection(),
      getSelection: () => xtermRef.current?.getSelection() || '',
      paste: (text: string) => xtermRef.current?.paste(text),
      scrollToTop: () => xtermRef.current?.scrollToTop(),
      scrollToBottom: () => xtermRef.current?.scrollToBottom(),
      findNext: (term: string) => addonsRef.current?.searchAddon.findNext(term),
      findPrevious: (term: string) => addonsRef.current?.searchAddon.findPrevious(term),
      serialize: () => (xtermRef.current ? addonsRef.current?.serializeAddon.serialize() : ''),
      resize: () => fitTerminal(4),
    }),
    [fitTerminal, getSession, isInitialized, sessionId],
  )

  return (
    <div className="relative flex h-full w-full flex-col overflow-hidden bg-transparent">
      <TerminalSearch
        isVisible={isSearchVisible}
        onSearch={handleSearch}
        onNext={handleSearchNext}
        onPrevious={handleSearchPrevious}
        onClose={handleSearchClose}
        currentMatch={searchResults.current}
        totalMatches={searchResults.total}
      />
      <div className="flex min-h-0 flex-1 flex-col pl-[16px]">
        <div
          ref={terminalContainerRef}
          id={`terminal-${sessionId}`}
          data-terminal-drop-target
          data-terminal-session-id={sessionId}
          className={`xterm-container flex h-full min-h-0 flex-1 text-foreground ${!isActive ? 'opacity-60' : ''}`}
          onDragOver={handleTerminalDragOver}
          onDrop={handleTerminalFileDrop}
          onMouseDown={() => {
            requestAnimationFrame(() => xtermRef.current?.focus())
          }}
        />
      </div>
    </div>
  )
}
