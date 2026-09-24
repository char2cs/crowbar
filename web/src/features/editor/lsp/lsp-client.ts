// LSP client backed by the Go daemon: document sync (didOpen/didChange/
// didSave/didClose) over REST, diagnostics over the chat's /lsp/ws topic, and
// the feature requests the Monaco providers (monaco-lsp-providers.ts) make.

import { apiFetch } from '@/lib/api'
import { wsManager } from '@/lib/ws/manager'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { isHomeWorkspace, lspBaseForWorkspace } from '@/lib/workspace-scope-url'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
/** The daemon's lifecycle view of the server for one file's language. */
export interface LspServerStatus {
  languageId?: string
  command?: string
  state: 'unsupported' | 'notInstalled' | 'stopped' | 'running'
}

export interface LspDiagnostic {
  filePath: string
  range: {
    start: { line: number; character: number }
    end: { line: number; character: number }
  }
  severity: string
  message: string
  source?: string
  code?: string
}

interface DiagnosticsEvent {
  wsId: string
  diagnostics: LspDiagnostic[]
}

// wsId is the workspace the batch was computed FOR — see dispatch()'s own
// doc for why a handler must check it, not just filePath.
/** The owner the LSP diagnostics markers are published under in Monaco. */
export const LSP_MARKER_OWNER = 'crowbar-lsp'

// Trailing quiet period before an edited document's text is sent to the
// server (requests flush it early, see flushChange).
const CHANGE_DEBOUNCE_MS = 400

type DiagnosticsHandler = (filePath: string, diagnostics: LspDiagnostic[], wsId: string) => void

class LspClientImpl {
  static _instance: LspClientImpl | null = null
  static getInstance(): LspClientImpl {
    if (!LspClientImpl._instance) LspClientImpl._instance = new LspClientImpl()
    return LspClientImpl._instance
  }

  isStarted = false
  private handlers = new Set<DiagnosticsHandler>()
  private wsId: string | null = null
  private unsubscribe: (() => void) | null = null
  private lastByFile = new Map<string, LspDiagnostic[]>()
  // Open refcount per file (I4): every pane showing a file opens it (the
  // satellite hook's diagnostics lifecycle). Reference-count opens so exactly
  // ONE `/didOpen` POST goes out (the first opener) and `/didClose` only fires
  // when the LAST holder closes.
  private openRefs = new Map<string, number>()
  // Opens whose `/didOpen` POST never went out because the owning chat id
  // wasn't recorded yet (see ensureSubscribed) — flushed once it arrives.
  // Without this, a file the editor already refcounts as "open" would never
  // actually get opened on the server, and would never get diagnostics.
  private pendingOpens = new Map<string, { content: string; languageId: string }>()
  private openListeners = new Set<(filePath: string) => void>()
  // Debounced didChange per open document (see scheduleChange/flushChange).
  private pendingChanges = new Map<
    string,
    { read: () => string | null; timer: ReturnType<typeof setTimeout> }
  >()
  // wsId this instance is currently waiting on an owning-chat-id for, and the
  // unsubscribe for that wait — see ensureSubscribed/awaitOwningChatId.
  private awaitingScopeFor: string | null = null
  private stopAwaitingScope: (() => void) | null = null

  // Subscribe to the workspace's diagnostics topic. Snapshot-on-subscribe
  // replays current diagnostics; later batches arrive live.
  private ensureSubscribed(): void {
    const wsId = getActiveWorkspaceId()
    // The project HOME workspace has no worktree, no owning chat, and no LSP
    // surface on the daemon at all (before or after the chat-scoped move) —
    // skip it rather than call lspBaseForWorkspace and catch its throw.
    if (!wsId || isHomeWorkspace(wsId)) return
    if (this.wsId === wsId && this.unsubscribe) return

    // lspBaseForWorkspace(wsId) throws until the sidebar's own chat-list fetch
    // records this workspace's owning chat id — a race against WorkspaceView's
    // own (often faster) hydration that is very much still live the instant a
    // buffer becomes a pane's active model (tab restoration on a cold
    // activation, well before any user gesture). This is called synchronously
    // from `onDiagnosticsUpdate`, so throwing here used to propagate straight
    // out of the satellite hook's effect and crash via the nearest error
    // boundary — identical shape to the fix in use-workspace-effects.ts. Wait
    // for the id via subscribeToWorkspaceScope instead, and retry once it
    // lands, rather than crashing or leaving diagnostics unsubscribed forever.
    if (!getOwningChatId(wsId)) {
      this.awaitOwningChatId(wsId)
      return
    }
    this.stopAwaitingScope?.()
    this.stopAwaitingScope = null
    this.awaitingScopeFor = null

    this.unsubscribe?.()
    this.wsId = wsId
    this.lastByFile.clear()
    // The new workspace's documents are not open yet; drop stale refcounts so a
    // first open there still POSTs `/didOpen`.
    this.openRefs.clear()
    this.unsubscribe = wsManager.subscribe(`${lspBaseForWorkspace(wsId)}/ws`, (raw) =>
      this.dispatch(raw as DiagnosticsEvent),
    )
  }

  // Re-entrant wait for `wsId`'s owning chat id: a no-op while already waiting
  // on the SAME wsId (ensureSubscribed is called from onDiagnosticsUpdate,
  // documentOpen and onDiagnosticsUpdate alike, often several times before the id
  // lands), and drops any wait on a DIFFERENT wsId so switching workspaces
  // mid-wait can't leak a stale listener.
  private awaitOwningChatId(wsId: string): void {
    if (this.awaitingScopeFor === wsId) return
    this.stopAwaitingScope?.()
    this.awaitingScopeFor = wsId
    this.stopAwaitingScope = subscribeToWorkspaceScope(wsId, () => {
      if (!getOwningChatId(wsId)) return
      this.stopAwaitingScope?.()
      this.stopAwaitingScope = null
      this.awaitingScopeFor = null
      // Only matters if the app still cares about this workspace; a stale wait
      // for one the user has since navigated away from must not resubscribe.
      if (getActiveWorkspaceId() !== wsId) return
      this.ensureSubscribed()
      this.flushPendingOpens()
    })
  }

  // Send the `/didOpen` POSTs that documentOpen deferred while wsBase() had no
  // owning chat id to address them with (see documentOpen/wsBase). Cleared
  // eagerly so a POST that itself fails is not retried in a loop.
  private flushPendingOpens(): void {
    if (this.pendingOpens.size === 0) return
    const base = this.wsBase()
    if (!base) return
    const opens = this.pendingOpens
    this.pendingOpens = new Map()
    for (const [filePath, { content, languageId }] of opens) {
      void this.postOpen(base, filePath, languageId, content).catch(() => {})
    }
  }

  // didOpen is what spawns a server daemon-side, so its completion is when a
  // status indicator should look again.
  private async postOpen(
    base: string,
    filePath: string,
    languageId: string,
    content: string,
  ): Promise<void> {
    await apiFetch(`${base}/didOpen`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: filePath, languageId, text: content }),
    })
    for (const listener of this.openListeners) listener(filePath)
  }

  /** Called with the path each time a document's didOpen reached the daemon. */
  onDocumentOpened(listener: (filePath: string) => void): () => void {
    this.openListeners.add(listener)
    return () => {
      this.openListeners.delete(listener)
    }
  }

  private dispatch(event: DiagnosticsEvent): void {
    if (!event?.diagnostics) return
    const byFile = new Map<string, LspDiagnostic[]>()
    for (const diag of event.diagnostics) {
      const list = byFile.get(diag.filePath) ?? []
      list.push(diag)
      byFile.set(diag.filePath, list)
    }
    // Clear files that previously had diagnostics but no longer do.
    for (const filePath of this.lastByFile.keys()) {
      if (!byFile.has(filePath)) byFile.set(filePath, [])
    }
    this.lastByFile = byFile
    // This singleton subscribes to ONE workspace's topic at a time, but
    // `handlers` accumulates one per MOUNTED pane, including panes showing a
    // DIFFERENT (non-active) workspace's file — those never unsubscribe just
    // because their workspace isn't the currently-subscribed one. Two
    // workspaces (two worktrees of the same repo, say) can easily share a
    // relative path, so a bare filePath match alone hands a background pane
    // another workspace's diagnostics for what LOOKS like its own file — same
    // shape as the Monaco model URI collision this session already
    // root-caused, one layer up. Handlers compare `event.wsId` against their
    // OWN pane's resolved workspace and ignore anything else.
    for (const [filePath, diagnostics] of byFile) {
      for (const handler of this.handlers) handler(filePath, diagnostics, event.wsId)
    }
  }

  private wsBase(): string | null {
    const wsId = getActiveWorkspaceId()
    if (!wsId || isHomeWorkspace(wsId)) return null
    // Same race as ensureSubscribed: lspBaseForWorkspace throws without a
    // recorded owning chat id. Callers here (getDefinition, documentChange,
    // documentClose, documentOpen) already tolerate a null base as "nothing to
    // do yet", so degrade the same way instead of throwing through them.
    if (!getOwningChatId(wsId)) return null
    return lspBaseForWorkspace(wsId)
  }

  /**
   * POST a feature request to `wsId`'s LSP route. Resolves null when the
   * workspace has no LSP surface yet (home workspace, owning chat not
   * recorded) — the same "nothing to do" the daemon answers for a language
   * with no server. Daemon failures propagate so providers can surface them.
   */
  async request<T>(
    wsId: string,
    route: string,
    body: unknown,
    signal?: AbortSignal,
  ): Promise<T | null> {
    if (isHomeWorkspace(wsId) || !getOwningChatId(wsId)) return null
    const result = await apiFetch<T | null>(`${lspBaseForWorkspace(wsId)}/${route}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal,
    })
    return result ?? null
  }

  async status(wsId: string, filePath: string): Promise<LspServerStatus | null> {
    if (isHomeWorkspace(wsId) || !getOwningChatId(wsId)) return null
    return apiFetch<LspServerStatus>(
      `${lspBaseForWorkspace(wsId)}/status?path=${encodeURIComponent(filePath)}`,
    )
  }

  /** Restart the running server for `filePath`'s language (daemon-side). */
  restart(wsId: string, filePath: string): Promise<LspServerStatus | null> {
    return this.request<LspServerStatus>(wsId, 'restart', { path: filePath })
  }

  /**
   * Start the server for an open document whose server is not running (it
   * crashed, or was never installed when the file opened): re-send didOpen.
   * The daemon spawns on didOpen and holds the ref until the one didClose the
   * refcount above will eventually send.
   */
  async reopen(filePath: string, content: string, languageId: string): Promise<void> {
    const base = this.wsBase()
    if (!base || !this.openRefs.has(filePath)) return
    await this.postOpen(base, filePath, languageId, content)
  }

  // Document lifecycle: opening a file subscribes to diagnostics and tells the
  // server to analyze it; changes re-trigger analysis.
  async documentOpen(filePath: string, content: string, languageId: string): Promise<void> {
    this.ensureSubscribed()
    // Dedupe concurrent owners: only the FIRST opener POSTs `/didOpen`; later
    // opens just bump the refcount so the document is opened exactly once.
    const refs = this.openRefs.get(filePath) ?? 0
    this.openRefs.set(filePath, refs + 1)
    if (refs > 0) return
    const base = this.wsBase()
    if (!base) {
      // No owning chat id yet — there is no route to POST to. Remember the
      // open so awaitOwningChatId's retry can send it once the scope resolves
      // (see flushPendingOpens); otherwise the server would never learn about
      // a file the editor already considers open, and it could never compute
      // diagnostics for it.
      this.pendingOpens.set(filePath, { content, languageId })
      return
    }
    await this.postOpen(base, filePath, languageId, content).catch(() => {})
  }

  async documentChange(filePath: string, content: string): Promise<void> {
    const base = this.wsBase()
    if (!base) return
    await apiFetch(`${base}/didChange`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: filePath, text: content }),
    }).catch(() => {})
  }

  async documentSave(filePath: string): Promise<void> {
    // Only a document the server has open can be saved on it.
    if (!this.openRefs.has(filePath)) return
    const base = this.wsBase()
    if (!base) return
    await apiFetch(`${base}/didSave`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: filePath }),
    }).catch(() => {})
  }

  async documentClose(filePath: string): Promise<void> {
    // Only the LAST holder closes the document; a close for a never-opened (or
    // already-closed) file is a no-op, matching the previous tolerant behavior.
    const refs = this.openRefs.get(filePath) ?? 0
    if (refs === 0) return
    if (refs > 1) {
      this.openRefs.set(filePath, refs - 1)
      return
    }
    this.openRefs.delete(filePath)
    this.cancelChange(filePath)
    // Closed before its didOpen ever went out (still waiting on the owning
    // chat id) — nothing pending to flush, and nothing on the server to close.
    this.pendingOpens.delete(filePath)
    const base = this.wsBase()
    if (!base) return
    await apiFetch(`${base}/didClose`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: filePath }),
    }).catch(() => {})
  }

  /**
   * Debounce a full-text didChange for an open document. `read` is called when
   * the change is actually sent, so a burst of edits costs one read + POST;
   * it returns null when the text is gone (model disposed) and nothing is sent.
   */
  scheduleChange(filePath: string, read: () => string | null): void {
    const pending = this.pendingChanges.get(filePath)
    if (pending) clearTimeout(pending.timer)
    this.pendingChanges.set(filePath, {
      read,
      timer: setTimeout(() => void this.flushChange(filePath), CHANGE_DEBOUNCE_MS),
    })
  }

  /**
   * Send a scheduled didChange now. Position-addressed requests (completion,
   * signature help, …) await this first so the server answers against the
   * text the user sees, not the text from before the debounce window.
   */
  async flushChange(filePath: string): Promise<void> {
    const pending = this.pendingChanges.get(filePath)
    if (!pending) return
    clearTimeout(pending.timer)
    this.pendingChanges.delete(filePath)
    const text = pending.read()
    if (text !== null) await this.documentChange(filePath, text)
  }

  private cancelChange(filePath: string): void {
    const pending = this.pendingChanges.get(filePath)
    if (!pending) return
    clearTimeout(pending.timer)
    this.pendingChanges.delete(filePath)
  }

  onDiagnosticsUpdate(handler: DiagnosticsHandler): () => void {
    this.ensureSubscribed()
    this.handlers.add(handler)
    // Replay current diagnostics so a late subscriber paints immediately.
    // `lastByFile` is cleared every time `this.wsId` changes (ensureSubscribed),
    // so every entry in it is guaranteed to belong to the CURRENT this.wsId.
    if (this.wsId) {
      const wsId = this.wsId
      for (const [filePath, diagnostics] of this.lastByFile) handler(filePath, diagnostics, wsId)
    }
    return () => {
      this.handlers.delete(handler)
    }
  }
}

export { LspClientImpl as LspClient }
