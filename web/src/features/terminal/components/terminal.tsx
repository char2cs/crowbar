import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import type { ISearchOptions } from '@xterm/addon-search'
import type { Terminal } from '@xterm/xterm'
import React, {
  useCallback,
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  useSyncExternalStore,
} from 'react'
import { useSettingsStore } from '@/features/settings/store'
import { extractDroppedFilePaths } from '@/features/file-system/utils/file-system-dropped-paths'
import { useTauriFileDrop } from '@/features/file-system/lib/tauri-file-drop'
import { useTerminalAttachment } from '../hooks/use-terminal-attachment'
import { useTerminalConnection } from '../hooks/use-terminal-connection'
import { usePtySizeSync, fitToContainer } from '../hooks/use-pty-size-sync'
import { useTerminalTheme } from '../hooks/use-terminal-theme'
import { useXtermInstance } from '../hooks/use-xterm-instance'
import { useTerminalStore } from '../stores/terminal-store'
import { formatDroppedPathsForTerminal } from '../utils/terminal-file-drop'
import { TerminalSearch, type TerminalSearchOptions } from './terminal-search'
import '@xterm/xterm/css/xterm.css'
import '../styles/terminal.css'

interface XtermTerminalProps {
  sessionId: string
  /**
   * The workspace this terminal BELONGS to. Attaching must target it, never the
   * currently-active one: workspace keep-alive keeps hidden workspaces'
   * terminals mounted, so a transport drop can land while a DIFFERENT workspace
   * is active. Falls back to the active workspace when unset.
   */
  workspaceId?: string
  /**
   * The chat that OWNS this PTY, when the caller knows it directly — an agent
   * chat pane, whose terminal is that chat's own vendor-CLI session. Unset for a
   * shell tab, whose owner is the chat that owns the workspace's worktree.
   */
  chatId?: string
  isActive: boolean
  isVisible?: boolean
  onReady?: () => void
  onTerminalRef?: (ref: { focus: () => void; showSearch: () => void; terminal: Terminal }) => void
  /**
   * Fires when this shell tab's session has ENDED: the daemon sent its exit
   * frame, or the session it was bound to is no longer on the daemon. A shell
   * tab never spawns a replacement for an ended session (invariant B7).
   */
  onTerminalExit?: (sessionId: string) => void
  initialCommand?: string
  workingDirectory?: string
  /**
   * Attach-only: this terminal is a view onto ONE specific pre-existing PTY and
   * must NEVER spawn a shell. Used by the agent chat pane, whose PTY is a vendor
   * CLI. Changing sessionId re-points the SAME xterm at another PTY.
   */
  attachOnly?: boolean
  /**
   * The attach-only counterpart of onTerminalExit: fires when the viewed session
   * exits, or is found gone. Carries the sessionId it is FOR, so an owner can
   * tell an outgoing PTY's death from the incoming one's.
   */
  onSessionGone?: (goneSessionId: string) => void
  /**
   * Render from the container's very edge, dropping the 16px left inset that
   * shell tabs use — the agent pane's own frame supplies the surface.
   */
  flush?: boolean
}

/**
 * Reactive echo of `getOwningChatId(wsId)`: the sidebar records a workspace's
 * owning chat asynchronously, so a shell tab's terminals base cannot be built
 * until it has. Null (and no subscription) when there is no wsId to watch.
 */
function useOwningChatIdFor(wsId: string | undefined): string | null {
  return useSyncExternalStore(
    (onChange) => (wsId ? subscribeToWorkspaceScope(wsId, onChange) : () => {}),
    () => (wsId ? getOwningChatId(wsId) : null),
  )
}

export const XtermTerminal: React.FC<XtermTerminalProps> = ({
  sessionId,
  workspaceId,
  chatId,
  isActive,
  isVisible = true,
  onReady,
  onTerminalRef,
  onTerminalExit,
  initialCommand,
  workingDirectory,
  attachOnly = false,
  onSessionGone,
  flush = false,
}) => {
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const [isSearchVisible, setIsSearchVisible] = useState(false)
  const [searchResults, setSearchResults] = useState({ current: 0, total: 0 })

  const updateSession = useTerminalStore((s) => s.updateSession)
  const getSession = useTerminalStore((s) => s.getSession)
  const terminalFontSize = useSettingsStore((s) => s.settings.terminalFontSize)
  const { getTerminalTheme } = useTerminalTheme()

  // A never-shown tab builds nothing and attaches nothing until it is looked at.
  const [activated, setActivated] = useState(isVisible)
  useEffect(() => {
    if (isVisible) setActivated(true)
  }, [isVisible])

  const owningChatId = useOwningChatIdFor(chatId ? undefined : workspaceId)
  const chatScopeReady = Boolean(chatId) || !workspaceId || owningChatId !== null

  // Where a new shell starts, recorded for the tab label and link resolution.
  useEffect(() => {
    if (workingDirectory) updateSession(sessionId, { currentDirectory: workingDirectory })
  }, [sessionId, updateSession, workingDirectory])

  const onSessionGoneRef = useRef(onSessionGone)
  const onTerminalExitRef = useRef(onTerminalExit)
  useEffect(() => {
    onSessionGoneRef.current = onSessionGone
    onTerminalExitRef.current = onTerminalExit
  }, [onSessionGone, onTerminalExit])
  // The one place a session's end is routed to its owner: the agent view's
  // "ended" state, or the shell tab's close.
  const handleSessionEnded = useCallback(
    (endedSessionId: string) => {
      if (attachOnly) onSessionGoneRef.current?.(endedSessionId)
      else onTerminalExitRef.current?.(endedSessionId)
    },
    [attachOnly],
  )

  const { connection, created } = useTerminalAttachment({
    sessionId,
    workspaceId,
    chatId,
    attachOnly,
    enabled: activated && chatScopeReady,
    onEnded: handleSessionEnded,
  })

  // Input the xterm instance itself produces goes through the I/O hook's write,
  // which exists only after the hooks below are called: bridge it with a ref.
  const writeRef = useRef<(data: string, origin: string) => void>(() => {})
  const writeFromTerminal = useCallback(
    (data: string, origin: string) => writeRef.current(data, origin),
    [],
  )
  const { terminal, addons } = useXtermInstance({
    sessionId,
    container,
    enabled: activated,
    write: writeFromTerminal,
  })

  const { write } = useTerminalConnection({
    connection,
    created,
    getTerminalTheme,
    initialCommand,
    onTerminalExit: handleSessionEnded,
    sessionId,
    terminal,
    updateSession,
  })
  useEffect(() => {
    writeRef.current = write
  }, [write])

  usePtySizeSync({
    terminal,
    fitAddon: addons?.fitAddon ?? null,
    container,
    connection,
    isVisible,
  })

  const refit = useCallback(() => {
    fitToContainer(addons?.fitAddon ?? null, container)
  }, [addons, container])

  const focusTerminal = useCallback(() => terminal?.focus(), [terminal])

  const handleTerminalFileDrop = useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      const text = formatDroppedPathsForTerminal(extractDroppedFilePaths(event.dataTransfer))
      if (!text) return
      event.preventDefault()
      event.stopPropagation()
      write(text, 'file-drop')
      focusTerminal()
    },
    [focusTerminal, write],
  )
  const handleTauriTerminalDrop = useCallback(
    (paths: string[]) => {
      const text = formatDroppedPathsForTerminal(paths)
      if (!text) return
      write(text, 'file-drop')
      focusTerminal()
    },
    [focusTerminal, write],
  )
  const containerRef = useRef<HTMLDivElement | null>(null)
  useEffect(() => {
    containerRef.current = container
  }, [container])
  useTauriFileDrop(containerRef, handleTauriTerminalDrop)

  const handleTerminalDragOver = useCallback((event: React.DragEvent<HTMLDivElement>) => {
    if (!Array.from(event.dataTransfer.types).includes('Files')) return
    event.preventDefault()
    event.stopPropagation()
    event.dataTransfer.dropEffect = 'copy'
  }, [])

  // Hand the live terminal to whoever renders us, once it exists.
  useEffect(() => {
    if (!terminal) return
    onTerminalRef?.({
      focus: () => terminal.focus(),
      showSearch: () => setIsSearchVisible(true),
      terminal,
    })
  }, [terminal, onTerminalRef])
  const onReadyEvent = useEffectEvent(() => onReady?.())
  useEffect(() => {
    if (terminal) onReadyEvent()
  }, [terminal])

  // Focus follows the active pane. The effect runs after React commits the
  // visibility change, so the textarea is focusable: one call, no retries.
  useEffect(() => {
    if (isActive && isVisible && terminal) terminal.focus()
  }, [isActive, isVisible, terminal])

  useEffect(() => {
    if (!addons) return
    const disposable = addons.searchAddon.onDidChangeResults(({ resultIndex, resultCount }) => {
      setSearchResults({
        current: resultCount > 0 && resultIndex >= 0 ? resultIndex + 1 : 0,
        total: resultCount,
      })
    })
    return () => disposable.dispose()
  }, [addons])

  const handleZoom = useCallback(
    (delta: number) => {
      const newSize = Math.min(Math.max(terminalFontSize + delta, 8), 32)
      useSettingsStore.getState().updateSetting('terminalFontSize', newSize)
    },
    [terminalFontSize],
  )
  const handleZoomReset = useCallback(() => {
    useSettingsStore.getState().updateSetting('terminalFontSize', 14)
  }, [])

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
    addons?.searchAddon.clearDecorations()
    terminal?.clearSelection()
    setSearchResults({ current: 0, total: 0 })
  }, [addons, terminal])

  // Read the latest search/zoom state + handlers via an Effect Event so the
  // global keydown listener subscribes once per active session.
  const onWindowKeyDown = useEffectEvent((event: KeyboardEvent) => {
    const isTerminalFocused =
      container?.contains(event.target as Node) || container?.contains(document.activeElement)
    const key = event.key.toLowerCase()

    if ((event.ctrlKey || event.metaKey) && key === 'f' && (isTerminalFocused || isSearchVisible)) {
      event.preventDefault()
      event.stopPropagation()
      setIsSearchVisible(true)
    }

    if (event.key === 'Escape' && isSearchVisible) {
      event.preventDefault()
      setIsSearchVisible(false)
      clearSearch()
      terminal?.focus()
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
  })
  useEffect(() => {
    if (!isActive) return
    const handleKeyDown = (event: KeyboardEvent) => onWindowKeyDown(event)
    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [isActive])

  const handleSearch = useCallback(
    (term: string, options: TerminalSearchOptions) => {
      if (!term || !addons) {
        clearSearch()
        return
      }
      const found = addons.searchAddon.findNext(term, {
        ...getSearchOptions(options),
        incremental: true,
      })
      if (!found) setSearchResults({ current: 0, total: 0 })
    },
    [addons, clearSearch, getSearchOptions],
  )
  const handleSearchNext = useCallback(
    (term: string, options: TerminalSearchOptions) => {
      if (!term || !addons) return
      addons.searchAddon.findNext(term, getSearchOptions(options))
    },
    [addons, getSearchOptions],
  )
  const handleSearchPrevious = useCallback(
    (term: string, options: TerminalSearchOptions) => {
      if (!term || !addons) return
      addons.searchAddon.findPrevious(term, getSearchOptions(options))
    },
    [addons, getSearchOptions],
  )
  const handleSearchClose = useCallback(() => {
    setIsSearchVisible(false)
    clearSearch()
    terminal?.focus()
  }, [clearSearch, terminal])

  React.useImperativeHandle(
    getSession(sessionId)?.ref,
    () => ({
      terminal,
      searchAddon: addons?.searchAddon,
      focus: () => terminal?.focus(),
      showSearch: () => setIsSearchVisible(true),
      blur: () => terminal?.blur(),
      clear: () => terminal?.clear(),
      selectAll: () => terminal?.selectAll(),
      clearSelection: () => terminal?.clearSelection(),
      getSelection: () => terminal?.getSelection() || '',
      paste: (text: string) => terminal?.paste(text),
      scrollToTop: () => terminal?.scrollToTop(),
      scrollToBottom: () => terminal?.scrollToBottom(),
      findNext: (term: string) => addons?.searchAddon.findNext(term),
      findPrevious: (term: string) => addons?.searchAddon.findPrevious(term),
      serialize: () => (terminal ? addons?.serializeAddon.serialize() : ''),
      resize: refit,
    }),
    [addons, refit, terminal],
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
      <div className={`flex min-h-0 flex-1 flex-col ${flush ? '' : 'pl-[16px]'}`}>
        {/* react-doctor-disable-next-line no-static-element-interactions -- this div is xterm.js's mount point: xterm renders its own canvas + a hidden `.xterm-helper-textarea` inside it, which is the actual focusable/keyboard-operable surface. onMouseDown here only forwards DOM focus onto that textarea for clicks that land on the container's own padding rather than the canvas. */}
        <div
          ref={setContainer}
          id={`terminal-${sessionId}`}
          data-terminal-drop-target
          data-terminal-session-id={sessionId}
          className="xterm-container flex h-full min-h-0 flex-1 text-foreground"
          onDragOver={handleTerminalDragOver}
          onDrop={handleTerminalFileDrop}
          onMouseDown={focusTerminal}
        />
      </div>
    </div>
  )
}
