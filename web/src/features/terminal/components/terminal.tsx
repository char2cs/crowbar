import { useOwningChatId } from '@/lib/workspace-scope'
import { IS_MOBILE } from '@/utils/platform'
import React, {
  useCallback,
  useEffect,
  useImperativeHandle,
  useRef,
  useState,
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
import { TerminalHostNotice } from './terminal-host-notice'
import { TerminalSearch } from './terminal-search'
import '@xterm/xterm/css/xterm.css'
import '../styles/terminal.css'

export interface TerminalFocusHandle {
  focus: () => void
  /** Types `data` into the session as if from the keyboard (the phone's key bar). */
  send: (data: string) => void
}

interface XtermTerminalProps {
  sessionId: string
  /** The workspace this terminal belongs to, never the active one: hidden workspaces keep their
   *  terminals mounted, so a transport drop can land while another is active. */
  workspaceId: string
  /** The chat owning this PTY when known directly (an agent pane's CLI session). Unset for a
   *  shell tab, owned by the chat that owns the workspace's worktree. */
  chatId?: string
  /** The pane this terminal is shown in; a link opened from it lands there. */
  paneId?: string
  isActive: boolean
  isVisible?: boolean
  /** Focus handle for an owner that moves the keyboard here (the agent pane). */
  ref?: Ref<TerminalFocusHandle>
  /** The shell tab's session ended (exit frame, or gone from the daemon); a tab never spawns a
   *  replacement for it. */
  onTerminalExit?: (sessionId: string) => void
  initialCommand?: string
  workingDirectory?: string
  /** A view onto one existing PTY (the agent pane's CLI) that never spawns a shell. Changing
   *  sessionId re-points the same xterm at another PTY. */
  attachOnly?: boolean
  /** Attach-only onTerminalExit. Carries the sessionId it is for, so an owner can tell an
   *  outgoing PTY's death from the incoming one's. */
  onSessionGone?: (goneSessionId: string) => void
  /** Drop the shell tabs' 16px left inset; the agent pane's frame supplies the surface. */
  flush?: boolean
}

export const XtermTerminal: React.FC<XtermTerminalProps> = ({
  sessionId,
  workspaceId,
  chatId,
  paneId,
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

  const owningChatId = useOwningChatId(workspaceId)
  const chatScopeReady = Boolean(chatId) || owningChatId !== null

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
    workspaceId,
    paneId,
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
  useTerminalFileDrop(containerRef, write, focusTerminal)
  const search = useTerminalSearch(terminal, addons)
  useTerminalShortcuts({ isActive, container, search })

  useImperativeHandle(
    ref,
    () => ({ focus: focusTerminal, send: (data: string) => write(data, 'key-bar') }),
    [focusTerminal, write],
  )

  // Focus follows the active pane. The effect runs after React commits the
  // visibility change, so the textarea is focusable: one call, no retries.
  // Not on the phone: focus raises the keyboard there, so only a tap may.
  useEffect(() => {
    if (isActive && isVisible && terminal && !IS_MOBILE) terminal.focus()
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
      {connection === null && <TerminalHostNotice wsId={workspaceId} />}
      <div className={`flex min-h-0 flex-1 flex-col ${flush ? '' : 'pl-[16px]'}`}>
        {/* react-doctor-disable-next-line no-static-element-interactions -- this div is xterm.js's mount point: xterm renders its own canvas + a hidden `.xterm-helper-textarea` inside it, which is the actual focusable/keyboard-operable surface. onMouseDown here only forwards DOM focus onto that textarea for clicks that land on the container's own padding rather than the canvas. */}
        <div
          ref={attachContainer}
          id={`terminal-${sessionId}`}
          data-terminal-drop-target
          data-terminal-session-id={sessionId}
          className="xterm-container flex h-full min-h-0 flex-1 text-foreground"
          onMouseDown={focusTerminal}
        />
      </div>
    </div>
  )
}
