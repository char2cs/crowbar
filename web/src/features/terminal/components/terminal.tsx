import {
  terminalCreate,
  terminalListLive,
  terminalResize,
  onTransportDrop,
} from '@/lib/crowbar-bridge'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceBase } from '@/lib/workspace-scope-url'
import { resolveTerminalConnection } from './resolve-terminal-connection'
import { saveReconnect } from '../lib/terminal-reconnect-map'
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
import { registerTerminalFileLinks, workspaceRelativePath } from '../lib/terminal-file-links'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useProjectDataStore } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { useTerminalTheme } from '../hooks/use-terminal-theme'
import { useTerminalStore } from '../stores/terminal-store'
import { formatDroppedPathsForTerminal } from '../utils/terminal-file-drop'
import { analyzeTerminalPaste } from '../utils/paste-guard'
import { resolveTerminalFont } from '../utils/resolve-font'
import { resolveKeyOverride } from '../utils/terminal-key-overrides'
import { toast } from '@/features/window/stores/toast-store'
import { TerminalSearch, type TerminalSearchOptions } from './terminal-search'
import '@xterm/xterm/css/xterm.css'
import '../styles/terminal.css'

// Resolves the on-disk root of the current view for terminal file links:
// the active workspace's worktree, the repo root for the default workspace,
// or — on the project-home route, whose special workspace is not in the
// sidebar repo list — the project's own path.
function resolveWorkspaceRootPath(): string | undefined {
  const wsId = getActiveWorkspaceId()
  if (wsId) {
    for (const repo of useSidebarStore.getState().repos) {
      const ws = repo.workspaces?.find((w) => w.id === wsId)
      if (ws) return ws.localPath ?? repo.localPath
      // The default (main-worktree) workspace is not in the workspaces array;
      // it maps to the repo root path.
      if (repo.defaultWorkspaceId === wsId) return repo.localPath
    }
  }
  // Project home route (/ide/<projectId>/home): use the project path.
  const projectId = window.location.hash.match(/\/ide\/([^/]+)\/home/)?.[1]
  if (projectId) {
    const projects = dataOf(useProjectDataStore.getState().data) ?? []
    return projects.find((p) => p.id === projectId)?.path
  }
  return undefined
}

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
  // State-backed reuse flag: updated after resolveTerminalConnection returns so
  // post-mount re-attaches (workspace re-entry) correctly suppress the initial
  // command resend and font-settle delay, same as a native remount would.
  const [reuseConnection, setReuseConnection] = useState(Boolean(session?.connectionId))

  // Bumped after a transport-drop reconnect so useTerminalConnection re-registers
  // its terminalListen call on the fresh connection object, even when the
  // connectionId itself has not changed (daemon restarted, session restored).
  const [reconnectKey, setReconnectKey] = useState(0)

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

  // Re-resolve the terminal connection after a transport drop (daemon restart)
  // without recreating the xterm UI. Runs resolveTerminalConnection which
  // handles the reuse/re-attach/create decision, updates the store, and then
  // bumps reconnectKey so useTerminalConnection re-subscribes its listener on
  // the freshly-attached connection object.
  const doReconnect = useCallback(async () => {
    if (isInitializingRef.current) return
    const wsId = getActiveWorkspaceId()
    if (!wsId) return
    const base = `${workspaceBase(wsId)}/terminals`
    const existingSession = getSession(sessionId)
    isInitializingRef.current = true
    try {
      const result = await resolveTerminalConnection({
        workspaceId: wsId,
        tabSessionId: sessionId,
        storeConnectionId: existingSession?.connectionId,
        base,
        listLiveSessions: () => terminalListLive(base),
        createTerminal: () => terminalCreate(wsId, existingSession?.profileId),
      })
      updateSession(sessionId, { connectionId: result.connectionId })
      saveReconnect(wsId, sessionId, result.connectionId)
      // Sync PTY dimensions after re-attach so TUI apps redraw correctly.
      const term = xtermRef.current
      if (term) {
        void terminalResize(result.connectionId, term.rows, term.cols).catch(() => {})
      }
      setReconnectKey((k) => k + 1)
    } catch (err) {
      console.error('[terminal] transport-drop reconnect failed:', err)
      toast.error(
        'Terminal disconnected',
        'Could not reconnect to the terminal session. Try closing and reopening the tab.',
      )
    } finally {
      isInitializingRef.current = false
    }
  }, [getSession, sessionId, updateSession])

  // Subscribe to unexpected transport drops for the current connection. When
  // the daemon restarts while a pane terminal stays mounted, the WS closes
  // without a user-initiated detach — this effect picks that up and triggers
  // doReconnect so the tab re-attaches without a manual workspace switch.
  useEffect(() => {
    if (!connectionId) return
    return onTransportDrop(connectionId, () => {
      void doReconnect()
    })
  }, [connectionId, doReconnect])

  const { currentConnectionIdRef, writeBuffered } = useTerminalConnection({
    connectionId,
    getTerminalTheme,
    initialCommand,
    isInitialized,
    onTerminalExit,
    reconnectKey,
    remoteConnectionId,
    reuseExistingConnection: reuseConnection,
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
    // the blank gap on pane splits/moves visibly long. We read the store here
    // (before resolveTerminalConnection runs) because it reflects the mount-time
    // state; the post-resolve setReuseConnection() updates only affect the next
    // render's useTerminalConnection call, not this init path.
    const hadConnectionAtInitStart = Boolean(getSession(sessionId)?.connectionId)
    if (!hadConnectionAtInitStart) {
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
        // The ONLY manual key override (Shift/Alt+Enter): emit the CSI-u sequence
        // here and return false to SUPPRESS xterm's default CR, so it is sent
        // exactly once. Everything else is left to xterm's built-in keyboard
        // model — re-implementing it (the old onKey shortcuts) double-sent keys.
        const override = resolveKeyOverride(event)
        if (override !== null) {
          event.preventDefault()
          writeBuffered(override)
          return false
        }
        // Ctrl combos (without Cmd) → xterm handles them (Ctrl+U, Ctrl+C, …).
        if (event.ctrlKey && !event.metaKey) return true
        // Cmd combos are app/OS shortcuts (copy, paste, select-all, search) — keep
        // them out of the terminal; everything else goes to xterm.
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
      // File references (foo/bar.ts:12, /abs/path, ./rel) open inside Crowbar.
      // Relative paths resolve against the session cwd when known, else the
      // workspace root; the resolved ABSOLUTE path is then relativized back
      // onto the workspace root because handleFileOpen (the workspace-scoped
      // files API) only accepts worktree-relative paths. The provider is
      // disposed with the terminal instance.
      registerTerminalFileLinks(terminal, {
        getRoot: () => {
          const cwd = getSession(sessionId)?.currentDirectory
          if (cwd?.startsWith('/')) return cwd
          return resolveWorkspaceRootPath()
        },
        openFile: (absolutePath) => {
          const root = resolveWorkspaceRootPath()
          const rel = workspaceRelativePath(absolutePath, root)
          if (!rel) {
            toast.error(
              'Cannot open file',
              `${absolutePath} is outside the current workspace.`,
            )
            return
          }
          const openHandler = useFileSystemStore.getState().handleFileOpen
          if (!openHandler) {
            toast.error('Cannot open file', 'No editor is available in this view.')
            return
          }
          void openHandler(rel).catch(() => {
            toast.error('Could not open file', absolutePath)
          })
        },
        onUnresolved: (candidateText) => {
          toast.error('Cannot open file', `Could not resolve ${candidateText} to a path.`)
        },
      })
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

      // §3/Open-Q2: do NOT fall back to the synthetic rootFolderPath
      // ('/repos/<id>') — it is not a real filesystem path and the daemon
      // already defaults a new PTY's cwd to the workspace worktree. Only pass
      // an explicit working directory when one is actually known.
      const targetDirectory = workingDirectory || existingSession?.currentDirectory
      // parseRemotePath does not expose connectionId in the stub; use only the passed remoteConnectionId
      const effectiveRemoteConnectionId = remoteConnectionId || undefined

      // Derive the workspace base for attach/listLive paths.
      // getActiveWorkspaceId() is safe here (we're inside an async callback,
      // not the render path), and terminalCreate uses the same strategy.
      const wsId = getActiveWorkspaceId()
      if (!wsId) throw new Error('no active workspace for terminal')
      const base = `${workspaceBase(wsId)}/terminals`

      // Resolve: reuse live transport → re-attach detached → create fresh.
      const result = await resolveTerminalConnection({
        workspaceId: wsId,
        tabSessionId: sessionId,
        storeConnectionId: existingSession?.connectionId,
        base,
        listLiveSessions: () => terminalListLive(base),
        createTerminal: () => terminalCreate(wsId, existingSession?.profileId),
      })
      const activeConnectionId = result.connectionId

      // Always sync the store so in-memory connectionId is up to date.
      // Only include remoteConnectionId when it is actually defined — writing
      // undefined would clobber a previously-stored value on a reuse where the
      // prop is absent.
      updateSession(sessionId, {
        connectionId: activeConnectionId,
        currentDirectory: targetDirectory ?? undefined,
        ...(effectiveRemoteConnectionId ? { remoteConnectionId: effectiveRemoteConnectionId } : {}),
      })

      // Persist the tab→connectionId mapping now (not only on workspace switch).
      // Without this, if the user stays on the same workspace and the daemon
      // restarts, loadReconnect() returns null and resolve creates a fresh shell
      // instead of re-attaching the restored session. Idempotent — overwrites
      // with the current (correct) connectionId on every init.
      saveReconnect(wsId, sessionId, activeConnectionId)

      // Thread the resolver's reused decision into useTerminalConnection so
      // initial-command resend and other first-connect side-effects are correctly
      // suppressed when we're reusing or re-attaching an existing PTY.
      setReuseConnection(result.reused)

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

  // Window-level geometry changes: moving the window to another display (a
  // vertical→horizontal monitor, or one with a different scale factor) can
  // resize the window in one native transaction and flip devicePixelRatio.
  // A DPR flip re-measures xterm's cell metrics WITHOUT changing the
  // container box, so the container ResizeObserver above never fires and the
  // grid/PTY stay at the old dimensions (the "TUI keeps a scroll artifact
  // until you switch workspace and back" bug — the workspace re-attach fixed
  // it by force-pushing dims). Mirror that here: on window resize or DPR
  // change, debounce, refit, and ALWAYS push the PTY size (idempotent when
  // unchanged, and terminal.onResize would not fire when only the cell
  // metrics — not cols/rows — went stale).
  useEffect(() => {
    if (!isInitialized) return

    let timer: number | null = null
    const refit = () => {
      if (timer !== null) window.clearTimeout(timer)
      timer = window.setTimeout(() => {
        timer = null
        const container = terminalContainerRef.current
        const addons = addonsRef.current
        const term = xtermRef.current
        if (!container || !addons || !term) return
        const rect = container.getBoundingClientRect()
        if (rect.width <= 0 || rect.height <= 0) return
        addons.fitAddon.fit()
        term.refresh(0, term.rows - 1)
        const connId = currentConnectionIdRef.current
        if (connId) void terminalResize(connId, term.rows, term.cols).catch(() => {})
      }, 150)
    }

    window.addEventListener('resize', refit)

    // devicePixelRatio has no event; the standard pattern is a resolution
    // media query re-registered after each flip (it fires exactly once).
    let mql: MediaQueryList | null = null
    const onDprChange = () => {
      refit()
      listenDpr()
    }
    const listenDpr = () => {
      mql?.removeEventListener('change', onDprChange)
      mql = window.matchMedia(`(resolution: ${window.devicePixelRatio}dppx)`)
      mql.addEventListener('change', onDprChange)
    }
    listenDpr()

    return () => {
      window.removeEventListener('resize', refit)
      mql?.removeEventListener('change', onDprChange)
      if (timer !== null) window.clearTimeout(timer)
    }
  }, [currentConnectionIdRef, isInitialized])

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
