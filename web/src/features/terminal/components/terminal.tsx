import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import React, {
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
  useSyncExternalStore,
  type Ref,
} from 'react'
import { useTerminalAttachment } from '../hooks/use-terminal-attachment'
import { useTerminalConnection } from '../hooks/use-terminal-connection'
import { useTerminalFileDrop } from '../hooks/use-terminal-file-drop'
import { usePtySizeSync, fitToContainer } from '../hooks/use-pty-size-sync'
import { useTerminalSearch } from '../hooks/use-terminal-search'
import { useTerminalShortcuts } from '../hooks/use-terminal-shortcuts'
import { useTerminalTheme } from '../hooks/use-terminal-theme'
import { useXtermInstance } from '../hooks/use-xterm-instance'
import { useTerminalStore } from '../stores/terminal-store'
import { TerminalSearch } from './terminal-search'
import '@xterm/xterm/css/xterm.css'
import '../styles/terminal.css'

export interface TerminalFocusHandle {
  focus: () => void
}

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
  /** Focus handle for an owner that moves the keyboard here (the agent pane). */
  ref?: Ref<TerminalFocusHandle>
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
  ref,
  onTerminalExit,
  initialCommand,
  workingDirectory,
  attachOnly = false,
  onSessionGone,
  flush = false,
}) => {
  // The container as state (hooks rebuild on it) and as a ref (Tauri's drop
  // hit-test reads it), both set by one stable callback ref.
  const [container, setContainer] = useState<HTMLDivElement | null>(null)
  const containerRef = useRef<HTMLDivElement | null>(null)
  const attachContainer = useCallback((node: HTMLDivElement | null) => {
    containerRef.current = node
    setContainer(node)
  }, [])

  const updateSession = useTerminalStore((s) => s.updateSession)
  const getSession = useTerminalStore((s) => s.getSession)
  const { getTerminalTheme } = useTerminalTheme()

  // A never-shown tab builds nothing and attaches nothing until it is looked at.
  // Latched during render, so the first visible render already builds.
  const [activated, setActivated] = useState(isVisible)
  if (isVisible && !activated) setActivated(true)

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
  const fileDrop = useTerminalFileDrop(containerRef, write, focusTerminal)
  const search = useTerminalSearch(terminal, addons)
  useTerminalShortcuts({ isActive, container, search })

  useImperativeHandle(ref, () => ({ focus: focusTerminal }), [focusTerminal])

  // Focus follows the active pane. The effect runs after React commits the
  // visibility change, so the textarea is focusable: one call, no retries.
  useEffect(() => {
    if (isActive && isVisible && terminal) terminal.focus()
  }, [isActive, isVisible, terminal])

  useImperativeHandle(
    getSession(sessionId)?.ref,
    () => ({
      terminal,
      searchAddon: addons?.searchAddon,
      focus: () => terminal?.focus(),
      showSearch: search.open,
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
    [addons, refit, search.open, terminal],
  )

  return (
    <div className="relative flex h-full w-full flex-col overflow-hidden bg-transparent">
      <TerminalSearch {...search.barProps} />
      <div className={`flex min-h-0 flex-1 flex-col ${flush ? '' : 'pl-[16px]'}`}>
        {/* react-doctor-disable-next-line no-static-element-interactions -- this div is xterm.js's mount point: xterm renders its own canvas + a hidden `.xterm-helper-textarea` inside it, which is the actual focusable/keyboard-operable surface. onMouseDown here only forwards DOM focus onto that textarea for clicks that land on the container's own padding rather than the canvas. */}
        <div
          ref={attachContainer}
          id={`terminal-${sessionId}`}
          data-terminal-drop-target
          data-terminal-session-id={sessionId}
          className="xterm-container flex h-full min-h-0 flex-1 text-foreground"
          onDragOver={fileDrop.onDragOver}
          onDrop={fileDrop.onDrop}
          onMouseDown={focusTerminal}
        />
      </div>
    </div>
  )
}
