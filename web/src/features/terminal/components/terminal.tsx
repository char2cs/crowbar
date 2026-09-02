import {
  terminalCreate,
  terminalDetach,
  terminalListLive,
  terminalResize,
  onTransportDrop,
} from '@/lib/crowbar-bridge'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { chatBase, terminalsBaseForWorkspace } from '@/lib/workspace-scope-url'
import { resolveTerminalConnection } from './resolve-terminal-connection'
import { saveReconnect } from '../lib/terminal-reconnect-map'
import { retain as retainAttach, release as releaseAttach } from '../lib/attach-refcount'
import type { ISearchOptions } from '@xterm/addon-search'
import { Terminal } from '@xterm/xterm'
import React, { useCallback, useEffect, useEffectEvent, useRef, useState } from 'react'
import { useSettingsStore } from '@/features/settings/store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { extractDroppedFilePaths } from '@/features/file-system/utils/file-system-dropped-paths'
import {
  createTerminalAddons,
  injectLinkStyles,
  loadWebLinksAddon,
  removeLinkStyles,
  type TerminalAddons,
} from '../hooks/use-terminal-addons'
import { useTerminalConnection } from '../hooks/use-terminal-connection'
import { registerTerminalFileLinks, workspaceRelativePath } from '../lib/terminal-file-links'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { resolveWorkspaceRootPath } from '@/lib/workspace/resolve-root-path'
import { useTerminalTheme } from '../hooks/use-terminal-theme'
import { useTerminalStore } from '../stores/terminal-store'
import { refitAndSyncPty, pollUntilResizeSettles, type LastSyncedPtyDims } from '../lib/refit'
import { formatDroppedPathsForTerminal } from '../utils/terminal-file-drop'
import { resolveTerminalFont } from '../utils/resolve-font'
import { selectionTextPreservingWraps } from '../utils/selection-text'
import { resolveKeyOverride } from '../utils/terminal-key-overrides'
import { installInputTapeGlobal, observeInputEvents } from '../utils/input-tape'
import { toast } from '@/features/window/stores/toast-store'
import { TerminalSearch, type TerminalSearchOptions } from './terminal-search'
import '@xterm/xterm/css/xterm.css'
import '../styles/terminal.css'

interface XtermTerminalProps {
  sessionId: string
  /**
   * The workspace this terminal BELONGS to. Connection resolution (attach /
   * listLive / create / saveReconnect) must target this workspace, never the
   * currently-active one: workspace keep-alive keeps hidden workspaces'
   * terminals mounted and attached, so a transport drop (daemon restart) can
   * land while a DIFFERENT workspace is active — resolving against the active
   * workspace would spawn the replacement shell in the wrong worktree and
   * persist the wrong mapping. Falls back to the active workspace when unset
   * (callers that only ever render into the active workspace).
   */
  workspaceId?: string
  /**
   * The chat that OWNS this PTY, when the caller knows it directly — an agent
   * chat pane, whose terminal is that chat's own vendor-CLI session.
   *
   * Terminal routes are chat-scoped (`/v0/chats/:chatId/terminals`) because a
   * session belongs to the chat that opened it, never to the worktree it runs
   * in: sibling chats share worktrees and must not share shells. When unset
   * (a plain workspace shell tab, which no single chat opened) the owner is the
   * chat that owns the workspace's worktree — see terminalsBaseForWorkspace.
   */
  chatId?: string
  isActive: boolean
  isVisible?: boolean
  onReady?: () => void
  onTerminalRef?: (ref: { focus: () => void; showSearch: () => void; terminal: Terminal }) => void
  onTerminalExit?: (sessionId: string) => void
  initialCommand?: string
  workingDirectory?: string
  remoteConnectionId?: string
  /**
   * Attach-only: this terminal is a view onto ONE specific pre-existing PTY and
   * must NEVER spawn a shell. When that PTY is not live on the daemon — on mount
   * OR on a transport-drop reconnect — nothing is attached and `onSessionGone`
   * fires instead. Used by the agent chat pane, whose PTY is a vendor CLI: a
   * spawned replacement there is a bare shell wearing the agent's frame, not a
   * usable terminal. Ordinary shell tabs leave this unset and keep spawning.
   */
  attachOnly?: boolean
  /**
   * Fires when attachOnly resolution finds the session gone (mount or reconnect).
   * Carries the sessionId the resolution was FOR, not whichever session the owner
   * wants now: a displaced PTY can report its death after its replacement has
   * already been attached, and an owner that cannot tell the two apart would read
   * the outgoing death as the incoming one dying.
   */
  onSessionGone?: (goneSessionId: string) => void
  /**
   * Render from the container's very edge, dropping the 16px left inset that
   * shell tabs use for breathing room against a bare pane. The agent chat pane
   * sets this because its terminal already sits inside a Frame panel that
   * supplies the surface — there, the inset reads as a stray gap, and it also
   * knocks the terminal out of line with the provider switcher below it.
   */
  flush?: boolean
}

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive xterm host — init lock, resize, theme/font sync and link handling all coordinate one xterm instance via shared refs; the most tuned file in the terminal feature, no safe seam.
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
  remoteConnectionId,
  attachOnly = false,
  onSessionGone,
  flush = false,
}) => {
  const terminalContainerRef = useRef<HTMLDivElement>(null)
  const xtermRef = useRef<Terminal | null>(null)
  const addonsRef = useRef<TerminalAddons | null>(null)
  const [isInitialized, setIsInitialized] = useState(false)
  const [isSearchVisible, setIsSearchVisible] = useState(false)
  const [searchResults, setSearchResults] = useState({ current: 0, total: 0 })
  const isInitializingRef = useRef(false)
  const copyGuardAttachedRef = useRef(false)
  // Alt-drag puts xterm's selection in COLUMN mode, where each row is its own
  // slice and joining rows would be plain wrong. The mode is not on the public
  // API, so it is read off the mousedown that starts the drag.
  const columnSelectRef = useRef(false)
  // Last rows/cols pushed to the daemon PTY, per connection. refitAndSyncPty
  // reads this to decide whether a fit needs a resize IPC: it pushes whenever
  // xterm's grid differs from what the daemon was last told (so a no-op fit on a
  // freshly-remounted pane split still reconciles the stale PTY), and skips when
  // they already match (so redundant fits don't spam resize IPC).
  const lastSyncedPtyDimsRef = useRef<LastSyncedPtyDims | null>(null)

  // Held in a ref so an inline arrow from the parent (the agent pane's
  // setAttachment) does not re-identify doReconnect and churn the
  // transport-drop subscription on every render.
  const onSessionGoneRef = useRef(onSessionGone)
  useEffect(() => {
    onSessionGoneRef.current = onSessionGone
  }, [onSessionGone])

  // Latest visibility, read inside fitTerminal so the fit path can gate the PTY
  // push on it WITHOUT putting isVisible in fitTerminal's dep list — which would
  // re-identify fitTerminal on every tab switch and churn all its dependent
  // effects (ResizeObserver, theme/font, active-catch-up) for shell tabs too,
  // whose isVisible flips on tab switch as well. This effect runs before the
  // active-catch-up effect on a visibility flip (definition order), so the ref is
  // fresh by the time that effect calls fitTerminal on becoming visible.
  const isVisibleRef = useRef(isVisible)
  useEffect(() => {
    isVisibleRef.current = isVisible
  }, [isVisible])

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
  const { getTerminalTheme } = useTerminalTheme()
  const effectiveTerminalFontSize = Math.round(terminalFontSize * zoomLevel * 10) / 10
  const effectiveTerminalLetterSpacing = terminalLetterSpacing * zoomLevel
  const effectiveTerminalCursorWidth = Math.max(1, Math.round(terminalCursorWidth * zoomLevel))

  // The session whose resolve last ACQUIRED the init lock (in flight or landed),
  // vs the session the owner currently WANTS attached (the latest sessionId prop).
  // They diverge exactly when a swap raced an in-flight resolve — the reconcile
  // below is what closes the gap. attachedSessionIdRef is written only at
  // lock-acquire (doReconnect / initializeTerminal); desiredSessionIdRef only by
  // the swap effect further down.
  const attachedSessionIdRef = useRef(sessionId)
  const desiredSessionIdRef = useRef(sessionId)
  // Fresh-closure reconciler (assigned every render, after doReconnect exists).
  // Held in a ref so releaseInitLock can stay identity-stable.
  const reconcileAttachmentRef = useRef<() => void>(() => {})

  // EVERY release of the init lock reconciles. doReconnect serializes on the lock
  // and silently drops a call that finds it held — correct for a transport-drop
  // storm, but a SWAP that lands mid-resolve must not be lost (see the reconcile
  // effect below). Whoever holds the lock therefore checks, on release, whether
  // the pane re-pointed the terminal while it was resolving, and fires the swap
  // it swallowed.
  const releaseInitLock = useCallback(() => {
    isInitializingRef.current = false
    reconcileAttachmentRef.current()
  }, [])

  // Re-resolve the terminal connection after a transport drop (daemon restart)
  // without recreating the xterm UI. Runs resolveTerminalConnection which
  // handles the reuse/re-attach/create decision, updates the store, and then
  // bumps reconnectKey so useTerminalConnection re-subscribes its listener on
  // the freshly-attached connection object. Doubles as the attach-only SWAP
  // executor: the imperative-reattach effect below calls it when the pane points
  // this terminal at a new session.
  //
  // This is the second door onto createTerminal, and for an attach-only terminal
  // it is the DANGEROUS one: the agent's PTY can die at any moment while the pane
  // sits open (daemon restart, CLI exit, crash), and the drop lands here — where,
  // without the attachOnly flag, the resolver would helpfully spawn a bare shell
  // into the agent's frame. Both doors are flagged; neither may spawn.
  const doReconnect = useCallback(async () => {
    if (isInitializingRef.current) return
    // Resolve against the OWNING workspace (see the workspaceId prop doc):
    // hidden keep-alive workspaces reconnect here too, and the active
    // workspace may be a different one entirely.
    const wsId = workspaceId ?? getActiveWorkspaceId()
    if (!wsId) return
    // Owner first, worktree second: an explicit chatId is this PTY's owner; a
    // shell tab falls back to the chat owning wsId's worktree.
    const base = chatId ? `${chatBase(chatId)}/terminals` : terminalsBaseForWorkspace(wsId)
    const existingSession = getSession(sessionId)
    // The connection this view is attached to RIGHT NOW, captured BEFORE the
    // resolve re-points it. Used to conserve the attach ledger across the
    // reconnect (release the old count before retaining the resolved one). This
    // is the same value the unmount effect reads to release.
    const prevConnectionId = existingSession?.connectionId
    isInitializingRef.current = true
    // The resolve now in flight is FOR this session — recorded at lock-acquire so
    // a swap racing this resolve is visible as attached ≠ desired at lock-release.
    attachedSessionIdRef.current = sessionId
    try {
      const result = await resolveTerminalConnection({
        workspaceId: wsId,
        tabSessionId: sessionId,
        storeConnectionId: existingSession?.connectionId,
        base,
        listLiveSessions: () => terminalListLive(base),
        createTerminal: () => terminalCreate(base, existingSession?.profileId),
        attachOnly,
      })
      if ('gone' in result) {
        // Attach-only and the PTY is gone: the owner renders its ended state.
        onSessionGoneRef.current?.(sessionId)
        return
      }
      // This view is now attached to result.connectionId — count it so the LAST
      // releaser (not merely the first unmount) performs the detach. attachOnly
      // only: shell tabs never share a transport across views and never detach on
      // unmount, so they must not touch the ledger.
      //
      // CONSERVE THE LEDGER ACROSS A RECONNECT: release the PREVIOUS connection
      // before retaining the resolved one. Without the release, a plain
      // transport-drop reconnect — SAME sessionId AND connectionId, so neither the
      // swap reconciler nor the unmount effect runs — would retain a SECOND time
      // and leak count[C] 1→2; the unmount-time release then nets 2→1, returns
      // false, and the detach is SKIPPED — leaving the transport AND the resolver's
      // terminalHasTransport(C) alive, so the next mount short-circuits reuse
      // WITHOUT re-attaching → a blank pane (the exact bug this path fixes).
      //
      // Ledger-only — NEVER terminalDetach here: on a transport drop the old
      // transport is already dead (detaching it would close a corpse); on an
      // id-changing swap the outgoing transport is detached by the attach-only
      // detach effect's sessionId-change cleanup. On a same-id reconnect this nets
      // count[C] back to where it was; on an id change it drops the stale entry and
      // counts the fresh one. Guarded so an undefined/empty prevConnectionId no-ops.
      if (attachOnly) {
        if (prevConnectionId) releaseAttach(prevConnectionId)
        retainAttach(result.connectionId)
      }
      updateSession(sessionId, { connectionId: result.connectionId })
      saveReconnect(wsId, sessionId, result.connectionId)
      // Re-inject the file-link styles for this session. On an attach-only SWAP the
      // init effect's cleanup removed the OUTGOING session's styles and nothing else
      // re-injects (initializeTerminal, the only other injector, never runs again on
      // a mounted terminal). Idempotent by style-element id, so the transport-drop
      // path — same session, styles still present — is a no-op.
      injectLinkStyles(sessionId, terminalContainerRef.current?.id || `terminal-${sessionId}`)
      // Sync PTY dimensions after re-attach so TUI apps redraw correctly, and
      // record it so a follow-up no-op fit on the new connection doesn't re-push.
      const term = xtermRef.current
      if (term) {
        void terminalResize(result.connectionId, term.rows, term.cols).catch(() => {})
        lastSyncedPtyDimsRef.current = {
          connId: result.connectionId,
          rows: term.rows,
          cols: term.cols,
        }
      }
      setReconnectKey((k) => k + 1)
    } catch (err) {
      console.error('[terminal] transport-drop reconnect failed:', err)
      toast.error(
        'Terminal disconnected',
        'Could not reconnect to the terminal session. Try closing and reopening the tab.',
      )
    } finally {
      releaseInitLock()
    }
  }, [attachOnly, chatId, getSession, releaseInitLock, sessionId, updateSession, workspaceId])

  // The reconciler: called at every init-lock release. If the pane re-pointed this
  // terminal at a different session while a resolve was in flight (the guarded
  // doReconnect swallowed that swap), catch up now: release the transport the
  // stale resolve may have JUST attached — its sessionId-keyed detach cleanup ran
  // BEFORE the transport existed, so nobody else ever would — and fire the swap
  // again. Each pass attaches the LATEST desired session, so a chain of racing
  // swaps converges instead of looping. Assigned every render so it closes over
  // the freshest doReconnect (whose closure sessionId IS the desired session).
  useEffect(() => {
    reconcileAttachmentRef.current = () => {
      if (!attachOnly) return
      if (attachedSessionIdRef.current === desiredSessionIdRef.current) return
      const staleConnectionId = useTerminalStore
        .getState()
        .getSession(attachedSessionIdRef.current)?.connectionId
      // Release-then-detach: only tear down the orphaned intermediate transport
      // when this view was its last holder. A co-view still attached to the same
      // connectionId keeps it alive (release returns false).
      if (staleConnectionId && releaseAttach(staleConnectionId))
        void terminalDetach(staleConnectionId).catch(() => {})
      void doReconnect()
    }
  })

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

  // IMPERATIVE REATTACH — swap the PTY under a LIVE xterm instead of remounting it.
  //
  // The agent chat pane points ONE mounted terminal at a succession of PTYs: a
  // provider switch or a chat revive replaces the agent's runner, and the pane feeds
  // its new terminalSessionId straight in as a prop change. It used to force that
  // through with key={sessionId}, which unmounted the ENTIRE component — socket,
  // xterm instance, gridSlack observers, WS listeners — and rebuilt it on every
  // switch. Here we keep the instance and swap only the transport: the outgoing PTY
  // is released by the attach-only detach effect below (its cleanup runs on this same
  // sessionId change), and doReconnect attaches the incoming one through the SAME
  // resolver — so it can never spawn a bare shell — then bumps reconnectKey so
  // useTerminalConnection re-listens on the fresh socket. The daemon replays its
  // screen model on attach as a snapshot frame, which resets the buffer, so the
  // outgoing agent's scrollback never bleeds into the incoming one.
  //
  // Attach-only only: a shell tab never swaps sessionId under a mounted xterm, and
  // its portaled socket must survive layout changes untouched.
  //
  // attachedSessionIdRef is NOT written here — only at lock-acquire inside
  // doReconnect/initializeTerminal. If this effect marked the target attached
  // before the resolve had even started, a doReconnect swallowed by the init lock
  // (a swap racing an in-flight resolve) would look already-done to the reconciler
  // and the swap would be lost — the racing-swaps bug this structure exists to
  // prevent. A guarded-away call here is fine PRECISELY because the lock holder
  // reconciles on release (see releaseInitLock) and re-fires it.
  useEffect(() => {
    desiredSessionIdRef.current = sessionId
    if (!attachOnly) return
    if (attachedSessionIdRef.current === sessionId) return
    void doReconnect()
  }, [attachOnly, sessionId, doReconnect])

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

  // Fit xterm to its container AND reconcile the daemon PTY to xterm's resulting
  // grid on the final pass. Routing EVERY fit caller (ResizeObserver, active
  // catch-up, theme/font, portal-refit, imperative resize()) through
  // refitAndSyncPty is what closes the pane-split blank-top bug: the size is
  // pushed to the daemon whenever it differs from what the daemon was last told,
  // instead of relying on xterm's onResize (which only fires when xterm's own
  // rows/cols change) or on a one-shot external event. The WebGL repaint stays
  // change-gated (an unconditional refresh per fit is expensive and compounds
  // with WKWebView CA-layer re-rasterization) — only the PTY push is
  // unconditional-on-dims-differ. Defined here (not with the other early refs)
  // because it reads currentConnectionIdRef, returned by useTerminalConnection
  // above; refs in its closure are stable, so the empty dep list is correct.
  const fitTerminal = useCallback(
    (attempts = 1) => {
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

        if (attempt < attempts - 1) {
          // Intermediate pass: just fit and keep polling; the PTY reconcile and
          // the repaint decision happen once, on the final pass.
          addons.fitAddon.fit()
          attempt += 1
          rafId = requestAnimationFrame(runFit)
          return
        }

        const prevRows = term?.rows ?? 0
        const prevCols = term?.cols ?? 0
        // Single PTY-size owner: only the VISIBLE view pushes terminalResize. A
        // hidden attach-only view (an inactive chat tab kept alive behind
        // visibility:hidden) still fits xterm to its box locally — so its grid is
        // correct the instant it is shown — but routes the resize into a no-op and
        // the last-synced record into a throwaway, so neither the daemon PTY nor
        // the SHARED lastSyncedRef is advanced. The active-catch-up fit re-pushes
        // the correct size when the tab becomes visible (lastSyncedRef is still
        // stale, so it differs and pushes). Strictly attachOnly: a shell tab is
        // always the sole owner of its PTY and pushes as before.
        const hiddenAttachOnly = attachOnly && !isVisibleRef.current
        // Final pass: fit AND push the size to the daemon whenever it differs
        // from what the daemon was last told (refitAndSyncPty also calls fit()).
        refitAndSyncPty({
          container,
          fitAddon: addons.fitAddon,
          terminal: term,
          getConnectionId: () => currentConnectionIdRef.current,
          resize: hiddenAttachOnly
            ? () => {}
            : (connId, rows, cols) => {
                void terminalResize(connId, rows, cols).catch(() => {})
              },
          lastSyncedRef: hiddenAttachOnly ? { current: null } : lastSyncedPtyDimsRef,
        })

        if (term && (term.rows !== prevRows || term.cols !== prevCols)) {
          // Only force a full WebGL repaint when dimensions actually changed. An
          // unconditional refresh(0, rows-1) on every fit call causes expensive
          // full-canvas repaints that compound with WKWebView's CA layer
          // re-rasterization, making the app feel sluggish after terminal updates.
          term.refresh(0, term.rows - 1)
        }
      }

      rafId = requestAnimationFrame(runFit)

      return () => {
        if (rafId !== null) cancelAnimationFrame(rafId)
      }
    },
    // attachOnly is constant for a terminal's whole life (shell vs. agent view),
    // so listing it here does NOT churn fitTerminal on tab switches; isVisible is
    // read via isVisibleRef precisely so it does not appear here.
    [attachOnly, currentConnectionIdRef],
  )

  const handleTerminalFileDrop = useCallback(
    (event: React.DragEvent<HTMLDivElement>) => {
      const paths = extractDroppedFilePaths(event.dataTransfer)
      const text = formatDroppedPathsForTerminal(paths)
      if (!text) return

      event.preventDefault()
      event.stopPropagation()
      writeBuffered(text, 'file-drop')
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
    // Same contract as doReconnect: the resolve now in flight is FOR this session
    // (the closure's), so a swap racing this init shows up as attached ≠ desired
    // when the lock is released — and the reconcile catches it up.
    attachedSessionIdRef.current = sessionId
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
      releaseInitLock()
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
          writeBuffered(override, 'modifier-enter-override')
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
        // Observational only: records the RAW key/input/composition events this
        // textarea receives, so a duplicated or missing character can be traced
        // to the event that produced it (or shown to have no event at all).
        // Disposed with the terminal, alongside the beforeinput listener below.
        installInputTapeGlobal()
        // react-doctor-disable-next-line effect-needs-cleanup -- listeners live on xterm's own textarea and are torn down by `xterm.dispose()` (l.822); tracer can't follow it.
        observeInputEvents(terminal.textarea)
        // react-doctor-disable-next-line effect-needs-cleanup -- listener lives on xterm's own textarea and is torn down by `xterm.dispose()` (l.822); tracer can't follow it.
        terminal.textarea.addEventListener('beforeinput', (event) => {
          if (event.inputType === 'insertReplacementText' || event.inputType === 'insertFromDrop') {
            const text = event.dataTransfer?.getData('text/plain') ?? event.data
            if (!text || !currentConnectionIdRef.current) return

            event.preventDefault()
            writeBuffered(text, `beforeinput:${event.inputType}`)
          }
        })
      }

      // PASTE IS xterm's JOB. It is deliberately NOT intercepted here.
      //
      // This used to carry a capture-phase paste listener that read the clipboard,
      // rewrote CRLF to LF and wrote the result straight to the PTY. That path
      // skipped xterm's paste pipeline entirely, and with it BRACKETED PASTE: the
      // \x1b[200~ … \x1b[201~ wrapper that tells the receiving program "these bytes
      // are pasted text, not typing". Without it a pasted command block ran a line
      // at a time as the newlines arrived, and a multi-line message pasted into an
      // agent chat was submitted line by line — the "paste arrives in blocks" bug.
      // The comment that stood here even cited bracketed paste as the reason no
      // confirmation prompt was needed, while the code was the thing removing it.
      //
      // xterm's own handler (registered on the textarea and on .xterm inside
      // terminal.open()) does the whole job correctly: it normalizes every \r\n AND
      // bare \n to a single \r — which is also the real fix for BUG-016, the
      // Windows-authored snippet that ran each line twice — brackets the payload
      // when the program has asked for bracketed paste, and emits it through
      // onData, the same route every keystroke takes. Adding anything in front of
      // it can only take capability away.

      // Copy, unlike paste, DOES need us: the daemon repaints row by row so xterm
      // never records an auto-wrap, and its own selection reader would put a
      // newline at every row boundary. See selection-text.ts. Capture phase on the
      // container so this runs before xterm's own copy listener on .xterm, which
      // stopPropagation then keeps out. The ref guard stops an init retry (e.g. a
      // PTY create failure) stacking a second listener.
      if (!copyGuardAttachedRef.current) {
        copyGuardAttachedRef.current = true
        const container = terminalContainerRef.current
        container.addEventListener(
          'mousedown',
          (event) => {
            columnSelectRef.current = event.altKey
          },
          true,
        )
        container.addEventListener(
          'copy',
          (event) => {
            const term = xtermRef.current
            if (!term || columnSelectRef.current) return
            const range = term.getSelectionPosition()
            if (!range) return

            const text = selectionTextPreservingWraps(
              { cols: term.cols, getLine: (y) => term.buffer.active.getLine(y) },
              range,
            )
            if (!text) return

            event.clipboardData?.setData('text/plain', text)
            event.preventDefault()
            event.stopPropagation()
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
            toast.error('Cannot open file', `${absolutePath} is outside the current workspace.`)
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
          releaseInitLock()
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

      // Derive the workspace base for attach/listLive paths. Prefer the OWNING
      // workspace (the workspaceId prop): with keep-alive, a hidden workspace's
      // terminal can (re)initialize while a different workspace is active.
      // getActiveWorkspaceId() is a safe fallback here (we're inside an async
      // callback, not the render path).
      const wsId = workspaceId ?? getActiveWorkspaceId()
      if (!wsId) throw new Error('no active workspace for terminal')
      // Owner first, worktree second — see doReconnect.
      const base = chatId ? `${chatBase(chatId)}/terminals` : terminalsBaseForWorkspace(wsId)

      // Resolve: reuse live transport → re-attach detached → create fresh (or,
      // under attachOnly, report the session gone rather than spawning a shell).
      const result = await resolveTerminalConnection({
        workspaceId: wsId,
        tabSessionId: sessionId,
        storeConnectionId: existingSession?.connectionId,
        base,
        listLiveSessions: () => terminalListLive(base),
        createTerminal: () => terminalCreate(base, existingSession?.profileId),
        attachOnly,
      })
      if ('gone' in result) {
        // Attach-only and the PTY is gone. Leave the terminal uninitialized (no
        // connection, nothing to write to) and let the owner render its ended
        // state; it will unmount us.
        releaseInitLock()
        onSessionGoneRef.current?.(sessionId)
        return
      }
      const activeConnectionId = result.connectionId

      // Count this view's attachment (attachOnly only) so the detach on unmount is
      // performed by the LAST releaser — a co-view sharing this connectionId during
      // a pane move must not lose its transport to our unmount. Mirrors doReconnect.
      if (attachOnly) retainAttach(activeConnectionId)

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
      // Record what we just pushed so the fitTerminal(3) below (and the first
      // ResizeObserver fit) treat this size as already-synced and don't re-push
      // it when the fit is a no-op — the daemon is reconciled here.
      lastSyncedPtyDimsRef.current = {
        connId: activeConnectionId,
        rows: terminal.rows,
        cols: terminal.cols,
      }

      // No snapshot replay: xterm is portaled and never remounts mid-session,
      // so the live PTY redrawing via SIGWINCH is the source of truth.

      setIsInitialized(true)
      releaseInitLock()

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
      releaseInitLock()
    }
  }, [
    attachOnly,
    currentConnectionIdRef,
    fitTerminal,
    getSession,
    getTerminalTheme,
    isInitialized,
    onReady,
    onTerminalRef,
    releaseInitLock,
    remoteConnectionId,
    sessionId,
    terminalCursorBlink,
    terminalCursorStyle,
    terminalFontFamily,
    effectiveTerminalCursorWidth,
    effectiveTerminalFontSize,
    effectiveTerminalLetterSpacing,
    terminalLineHeight,
    terminalScrollback,
    updateSession,
    workingDirectory,
    workspaceId,
    chatId,
    writeBuffered,
  ])

  // Read the freshest theme + refit through an Effect Event so this effect only
  // re-runs when the theme id itself changes — not every time getTerminalTheme
  // or fitTerminal takes a new identity (which would needlessly tear down and
  // restart the timer). Same rationale as the init effects below; the dedicated
  // font/setting effect above already fits on those changes. See useEffectEvent.
  const onThemeTick = useEffectEvent(() => {
    const term = xtermRef.current
    if (!term) return
    term.options.theme = getTerminalTheme()
    term.refresh(0, term.rows - 1)
    fitTerminal(4)
  })
  useEffect(() => {
    if (!xtermRef.current) return
    const timer = setTimeout(onThemeTick, 10)
    return () => clearTimeout(timer)
  }, [terminalThemeId])

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

  // `initializeTerminal` changes identity whenever any terminal theme/font/setting
  // changes (large dep list). Wrapping the init calls in Effect Events keeps the two
  // init effects below from tearing down + restarting their timer/rAF on every such
  // change, while still calling the freshest initializeTerminal. See useEffectEvent.
  const onInitTick = useEffectEvent(() => {
    if (!isInitialized && !isInitializingRef.current) {
      void initializeTerminal()
    }
  })
  const onAttemptInit = useEffectEvent(() => {
    void initializeTerminal()
  })

  useEffect(() => {
    if (!isVisible) return

    let mounted = true
    const initTimer = setTimeout(() => {
      if (mounted) onInitTick()
    }, 200)

    return () => {
      mounted = false
      clearTimeout(initTimer)
      removeLinkStyles(sessionId)
    }
  }, [isVisible, sessionId])

  useEffect(() => {
    if (isInitialized || !isVisible || !terminalContainerRef.current) return

    let rafId: number | null = null
    const container = terminalContainerRef.current

    const attemptInitialize = () => {
      // xtermRef (not only isInitialized state) guards re-entry: it is set
      // synchronously mid-init, so a stale-closure frame that lands between init
      // completing and this effect re-running cannot start a SECOND init.
      if (isInitialized || xtermRef.current) return

      // The init lock is held by an in-flight resolve — initializeTerminal from
      // the timer path, or an attach-only swap's doReconnect that raced this
      // mount. Returning here without rescheduling used to DROP the init attempt
      // forever when the holder was a swap (nothing else re-kicks this poll), and
      // a terminal that never creates its xterm is permanently blank. Keep
      // polling until the lock frees; the guard above stops the loop the moment
      // an init actually lands.
      if (isInitializingRef.current) {
        rafId = requestAnimationFrame(attemptInitialize)
        return
      }

      const rect = container.getBoundingClientRect()
      if (rect.width <= 0 || rect.height <= 0) {
        rafId = requestAnimationFrame(attemptInitialize)
        return
      }

      onAttemptInit()
    }

    rafId = requestAnimationFrame(attemptInitialize)

    return () => {
      if (rafId !== null) cancelAnimationFrame(rafId)
    }
  }, [isInitialized, isVisible])

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
      copyGuardAttachedRef.current = false
    }
  }, [])

  // THE TRANSPORT MAY NOT OUTLIVE THE XTERM IT FEEDS — for an attach-only terminal.
  //
  // A shell tab's xterm is kept mounted for its whole life (pane-container holds
  // `terminal` buffers behind visibility:hidden precisely to preserve them), so its
  // socket is right to survive a pane split or a tab move: the same xterm, still
  // holding the same screen, goes on receiving into it.
  //
  // An attach-only terminal is the opposite. It is a VIEW onto a PTY the agent owns,
  // and it is destroyed and rebuilt every time its chat tab is switched away from,
  // closed and reopened, or its workspace is left and re-entered. A socket that
  // outlives it is not an optimisation but a trap: the daemon serializes its screen
  // model to a client at ATTACH and nowhere else, so a surviving transport sends the
  // next mount down the resolver's reuse branch (live transport → no attach), and the
  // brand-new, empty xterm is never sent the screen. The agent keeps working in a
  // terminal nobody redrew — the pane sits blank until something unrelated (a resize
  // → SIGWINCH → the CLI repaints itself) happens to fill it back in.
  //
  // Releasing it here keeps the PTY running (detach is not close) and makes the next
  // mount an attach — which is, by definition, a redraw.
  useEffect(() => {
    if (!attachOnly) return
    return () => {
      const connectionId = useTerminalStore.getState().getSession(sessionId)?.connectionId
      // Detach only when this was the transport's LAST live view. A pane move (or a
      // transient duplicate) can leave a co-view attached to the same connectionId;
      // release returns false there and keeps its transport — the co-view stays live
      // instead of blanking. An unknown/last count returns true and detaches as before.
      if (connectionId && releaseAttach(connectionId))
        void terminalDetach(connectionId).catch(() => {})
    }
  }, [attachOnly, sessionId])

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
  // react-doctor-disable-next-line effect-needs-cleanup -- cleanup exists (l.919-923: removeEventListener ×2 + clearTimeout); tracer can't follow it.
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
        // ALWAYS push here (not gated on dims-differ like refitAndSyncPty): a DPR
        // flip changes cell metrics without changing cols/rows, and the running
        // TUI still needs a SIGWINCH to re-measure and redraw. Update the
        // last-synced record so a follow-up fit treats this as reconciled.
        if (connId) {
          void terminalResize(connId, term.rows, term.cols).catch(() => {})
          lastSyncedPtyDimsRef.current = { connId, rows: term.rows, cols: term.cols }
        }
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
  // fitTerminal + sessionId read fresh through an Effect Event so this listener
  // is registered once per init, not re-subscribed every time fitTerminal takes
  // a new identity (tab switch, setting change). See useEffectEvent.
  const onRefitRequest = useEffectEvent((event: Event) => {
    const detail = (event as CustomEvent<{ sessionId: string }>).detail
    if (!detail || detail.sessionId !== sessionId) return
    fitTerminal(4)
    const term = xtermRef.current
    if (term) term.refresh(0, term.rows - 1)
  })
  useEffect(() => {
    if (!isInitialized) return
    const handler = (event: Event) => onRefitRequest(event)
    window.addEventListener('crowbar-terminal-refit', handler)
    return () => window.removeEventListener('crowbar-terminal-refit', handler)
  }, [isInitialized])

  useEffect(() => {
    if (!addonsRef.current || !terminalContainerRef.current || !isInitialized) return

    // Mirror Monaco's pattern: suppress fitting during pane/sidebar drags
    // (data-pane-resizing attribute) and do one final fit once the drag ends.
    // Without this, fitAddon.fit() + terminalResize() IPC fires every frame
    // during drag — far heavier than editor.layout() and causes canvas glitches.
    let rafId: number | null = null
    let cancelPoll: (() => void) | null = null
    let needsFitAfterResize = false

    // Fit + reconcile the PTY once, if the container currently has a box. Every
    // call routes through fitTerminal → refitAndSyncPty, so a no-op fit on a
    // freshly-remounted pane split still pushes the (now stale) daemon PTY size.
    const runFitIfSized = () => {
      const container = terminalContainerRef.current
      if (!addonsRef.current || !container) return
      const rect = container.getBoundingClientRect()
      if (rect.width > 0 && rect.height > 0) {
        fitTerminal(3)
      }
    }

    // Recovery that does NOT depend on the one-shot `pane-resize-end` window
    // event: a terminal remounted mid-drag (a pane split) never receives that
    // event, so relying on it alone drops the final fit and leaves the daemon
    // PTY at its pre-split size (the blank-top bug). Poll on rAF until the
    // drag attribute clears, then apply the last container size exactly once.
    const startResizePoll = () => {
      if (cancelPoll) return // already polling this drag
      cancelPoll = pollUntilResizeSettles({
        isResizing: () => document.documentElement.hasAttribute('data-pane-resizing'),
        onSettled: () => {
          cancelPoll = null
          if (!needsFitAfterResize) return
          needsFitAfterResize = false
          runFitIfSized()
        },
        requestFrame: (cb) => requestAnimationFrame(cb),
        cancelFrame: (id) => cancelAnimationFrame(id),
      })
    }

    const resizeObserver = new ResizeObserver(() => {
      if (document.documentElement.hasAttribute('data-pane-resizing')) {
        needsFitAfterResize = true
        startResizePoll()
        return
      }
      if (rafId) cancelAnimationFrame(rafId)
      rafId = requestAnimationFrame(() => {
        rafId = null
        runFitIfSized()
      })
    })

    // Additional (faster) trigger for the common case where this terminal is
    // still mounted when the drag ends; the rAF poll above is the guaranteed
    // recovery for the remount case.
    const handlePaneResizeEnd = () => {
      if (!needsFitAfterResize) return
      needsFitAfterResize = false
      if (cancelPoll) {
        cancelPoll()
        cancelPoll = null
      }
      if (rafId !== null) cancelAnimationFrame(rafId)
      rafId = requestAnimationFrame(() => {
        rafId = null
        runFitIfSized()
      })
    }
    window.addEventListener('pane-resize-end', handlePaneResizeEnd)

    resizeObserver.observe(terminalContainerRef.current)
    const cleanupFit = fitTerminal(3)

    return () => {
      resizeObserver.disconnect()
      if (rafId) cancelAnimationFrame(rafId)
      if (cancelPoll) cancelPoll()
      window.removeEventListener('pane-resize-end', handlePaneResizeEnd)
      cleanupFit?.()
    }
  }, [fitTerminal, isInitialized])

  // SIZE-RECONCILE ON BECOMING VISIBLE — even in an UNFOCUSED pane (attach-only).
  //
  // The active-catch-up effect below also re-fits and re-pushes the PTY size on
  // show, but it is gated on pane FOCUS (isActive = isActivePane && isVisible). The
  // fitTerminal path SUPPRESSES the PTY push while an attach-only view is HIDDEN
  // (hiddenAttachOnly routes the resize into a no-op so a background chat tab does
  // not fight the visible owner for the shared PTY size), delegating the re-push to
  // a become-visible fit. If that view is then revealed in an UNFOCUSED pane — e.g.
  // its sibling tab is closed while its split pane is not the focused one — after
  // its geometry changed while hidden, the focus-gated catch-up never runs and the
  // daemon PTY stays at a stale size until the pane is focused (a transient
  // misdraw). Reconcile on VISIBILITY alone here so the size is correct the moment
  // the view is shown, focused or not; focus-specific work stays on isActive below.
  //
  // attachOnly only: a shell tab pushes its PTY size even while hidden (the
  // suppression is attachOnly-gated), so it is never stale on show and must keep its
  // existing focus-gated catch-up untouched — this effect must not change it.
  useEffect(() => {
    if (!attachOnly || !isVisible || !isInitialized || !xtermRef.current) return
    const cleanupFit = fitTerminal(6)
    return () => cleanupFit?.()
  }, [attachOnly, isVisible, isInitialized, fitTerminal])

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

  // Read the latest search/zoom state + handlers via an Effect Event so the
  // global keydown listener subscribes once per active session instead of
  // re-subscribing whenever clearSearch/handleZoom/handleZoomReset or
  // isSearchVisible change.
  const onWindowKeyDown = useEffectEvent((event: KeyboardEvent) => {
    const isTerminalFocused =
      terminalContainerRef.current?.contains(event.target as Node) ||
      terminalContainerRef.current?.contains(document.activeElement)
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
  })
  useEffect(() => {
    if (!isActive) return

    const handleKeyDown = (event: KeyboardEvent) => onWindowKeyDown(event)
    window.addEventListener('keydown', handleKeyDown, true)
    return () => window.removeEventListener('keydown', handleKeyDown, true)
  }, [isActive])

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
    [fitTerminal],
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
        {/* react-doctor-disable-next-line no-static-element-interactions -- this div is xterm.js's mount point: xterm renders its own canvas + a hidden `.xterm-helper-textarea` inside it, which is the actual focusable/keyboard-operable surface (role, typing, screen-reader relay all live on that textarea, owned by xterm's own accessibility layer). onMouseDown here only forwards DOM focus onto that textarea for clicks that land on the container's own padding rather than the canvas; it isn't a distinct interactive control of its own. */}
        <div
          ref={terminalContainerRef}
          id={`terminal-${sessionId}`}
          data-terminal-drop-target
          data-terminal-session-id={sessionId}
          className="xterm-container flex h-full min-h-0 flex-1 text-foreground"
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
