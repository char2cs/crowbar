// LSP client backed by the Go daemon: document sync (didOpen/didChange/
// didSave/didClose) over REST, diagnostics over the owning chat's /lsp/ws
// topic, and the feature requests the Monaco providers (monaco-lsp-providers.ts)
// make. Everything is addressed by the workspace the caller names — the
// buffer's own — never by whichever workspace is active.

import { apiFetch } from '@/lib/api'
import { wsManager } from '@/lib/ws/manager'
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
  diagnostics?: LspDiagnostic[]
}

/** The owner the LSP diagnostics markers are published under in Monaco. */
export const LSP_MARKER_OWNER = 'crowbar-lsp'

// Trailing quiet period before an edited document's text is sent to the
// server (requests flush it early, see flushChange).
const CHANGE_DEBOUNCE_MS = 400

type DiagnosticsHandler = (filePath: string, diagnostics: LspDiagnostic[]) => void
type OpenedListener = (wsId: string, filePath: string) => void

/** The LSP route base for `wsId`, or null while it has none (home, no chat). */
function lspBase(wsId: string): string | null {
  if (isHomeWorkspace(wsId) || !getOwningChatId(wsId)) return null
  return lspBaseForWorkspace(wsId)
}

function post(url: string, body: unknown, signal?: AbortSignal): Promise<unknown> {
  return apiFetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal,
  })
}

/**
 * One workspace's LSP state. It exists while it holds an open document or a
 * diagnostics handler; its socket is open while it exists and the workspace's
 * owning chat is known (the scope registry is the owner of that fact).
 */
class WorkspaceSession {
  readonly handlers = new Set<DiagnosticsHandler>()
  /** Open refcount per path: one didOpen for the first holder, didClose with the last. */
  readonly openRefs = new Map<string, number>()
  /** Opens not sent yet because the owning chat is not known yet. */
  readonly deferred = new Map<string, { content: string; languageId: string }>()
  readonly pendingChanges = new Map<
    string,
    { read: () => string | null; timer: ReturnType<typeof setTimeout> }
  >()
  private lastByFile = new Map<string, LspDiagnostic[]>()
  /** The chat whose topic the socket is on, and its unsubscribe. */
  private socket: { chatId: string; close: () => void } | null = null
  private readonly stopWatchingScope: () => void

  constructor(
    readonly wsId: string,
    private readonly sendOpen: (
      base: string,
      path: string,
      languageId: string,
      text: string,
    ) => void,
  ) {
    this.stopWatchingScope = subscribeToWorkspaceScope(wsId, () => this.followScope())
    this.followScope()
  }

  base(): string | null {
    return lspBase(this.wsId)
  }

  isIdle(): boolean {
    return this.handlers.size === 0 && this.openRefs.size === 0
  }

  addHandler(handler: DiagnosticsHandler): void {
    this.handlers.add(handler)
    for (const [filePath, diagnostics] of this.lastByFile) handler(filePath, diagnostics)
  }

  cancelChange(filePath: string): void {
    const pending = this.pendingChanges.get(filePath)
    if (!pending) return
    clearTimeout(pending.timer)
    this.pendingChanges.delete(filePath)
  }

  dispose(): void {
    this.stopWatchingScope()
    this.closeSocket()
    for (const filePath of [...this.pendingChanges.keys()]) this.cancelChange(filePath)
  }

  // The owning chat arriving opens the socket and sends deferred opens; it
  // going away (the workspace was deleted) closes the socket for good.
  private followScope(): void {
    const chatId = this.base() ? getOwningChatId(this.wsId) : null
    if (chatId === (this.socket?.chatId ?? null)) return
    this.closeSocket()
    if (!chatId) return
    const base = this.base()!
    this.socket = {
      chatId,
      close: wsManager.subscribe(`${base}/ws`, (raw) => this.dispatch(raw as DiagnosticsEvent)),
    }
    const opens = [...this.deferred]
    this.deferred.clear()
    for (const [path, { content, languageId }] of opens)
      this.sendOpen(base, path, languageId, content)
  }

  private closeSocket(): void {
    if (!this.socket) return
    this.socket.close()
    this.socket = null
    // Markers from a server that is gone would never be cleared otherwise.
    this.publish(new Map([...this.lastByFile.keys()].map((path) => [path, []])))
  }

  private dispatch(event: DiagnosticsEvent): void {
    if (!event?.diagnostics) return
    const byFile = new Map<string, LspDiagnostic[]>()
    for (const diag of event.diagnostics) {
      const list = byFile.get(diag.filePath) ?? []
      list.push(diag)
      byFile.set(diag.filePath, list)
    }
    // Files that had diagnostics and no longer do are cleared.
    for (const filePath of this.lastByFile.keys()) {
      if (!byFile.has(filePath)) byFile.set(filePath, [])
    }
    this.publish(byFile)
  }

  private publish(byFile: Map<string, LspDiagnostic[]>): void {
    this.lastByFile = new Map([...byFile].filter(([, diagnostics]) => diagnostics.length > 0))
    for (const [filePath, diagnostics] of byFile) {
      for (const handler of this.handlers) handler(filePath, diagnostics)
    }
  }
}

class LspClientImpl {
  static _instance: LspClientImpl | null = null
  static getInstance(): LspClientImpl {
    if (!LspClientImpl._instance) LspClientImpl._instance = new LspClientImpl()
    return LspClientImpl._instance
  }

  private sessions = new Map<string, WorkspaceSession>()
  private openListeners = new Set<OpenedListener>()

  private session(wsId: string): WorkspaceSession {
    let session = this.sessions.get(wsId)
    if (!session) {
      session = new WorkspaceSession(wsId, (base, path, languageId, text) => {
        void this.postOpen(wsId, base, path, languageId, text).catch(() => {})
      })
      this.sessions.set(wsId, session)
    }
    return session
  }

  private releaseIfIdle(session: WorkspaceSession): void {
    if (!session.isIdle() || this.sessions.get(session.wsId) !== session) return
    this.sessions.delete(session.wsId)
    session.dispose()
  }

  // didOpen is what spawns a server daemon-side, so its completion is when a
  // status indicator should look again.
  private async postOpen(
    wsId: string,
    base: string,
    filePath: string,
    languageId: string,
    text: string,
  ): Promise<void> {
    await post(`${base}/didOpen`, { path: filePath, languageId, text })
    for (const listener of this.openListeners) listener(wsId, filePath)
  }

  /** Called each time a document's didOpen reached the daemon. */
  onDocumentOpened(listener: OpenedListener): () => void {
    this.openListeners.add(listener)
    return () => {
      this.openListeners.delete(listener)
    }
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
    const base = lspBase(wsId)
    if (!base) return null
    const result = (await post(`${base}/${route}`, body, signal)) as T | null
    return result ?? null
  }

  async status(wsId: string, filePath: string): Promise<LspServerStatus | null> {
    const base = lspBase(wsId)
    if (!base) return null
    return apiFetch<LspServerStatus>(`${base}/status?path=${encodeURIComponent(filePath)}`)
  }

  /** Restart the running server for `filePath`'s language (daemon-side). */
  restart(wsId: string, filePath: string): Promise<LspServerStatus | null> {
    return this.request<LspServerStatus>(wsId, 'restart', { path: filePath })
  }

  /**
   * Start the server for an open document whose server is not running (it
   * crashed, or was never installed when the file opened): re-send didOpen.
   */
  async reopen(wsId: string, filePath: string, content: string, languageId: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const base = session?.base()
    if (!session?.openRefs.has(filePath) || !base) return
    await this.postOpen(wsId, base, filePath, languageId, content)
  }

  async documentOpen(
    wsId: string,
    filePath: string,
    content: string,
    languageId: string,
  ): Promise<void> {
    const session = this.session(wsId)
    const refs = session.openRefs.get(filePath) ?? 0
    session.openRefs.set(filePath, refs + 1)
    if (refs > 0) return
    const base = session.base()
    if (!base) {
      session.deferred.set(filePath, { content, languageId })
      return
    }
    await this.postOpen(wsId, base, filePath, languageId, content).catch(() => {})
  }

  async documentSave(wsId: string, filePath: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const base = session?.base()
    if (!session?.openRefs.has(filePath) || session.deferred.has(filePath) || !base) return
    await post(`${base}/didSave`, { path: filePath }).catch(() => {})
  }

  async documentClose(wsId: string, filePath: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const refs = session?.openRefs.get(filePath) ?? 0
    if (!session || refs === 0) return
    if (refs > 1) {
      session.openRefs.set(filePath, refs - 1)
      return
    }
    session.openRefs.delete(filePath)
    session.cancelChange(filePath)
    const neverSent = session.deferred.delete(filePath)
    const base = session.base()
    this.releaseIfIdle(session)
    if (neverSent || !base) return
    await post(`${base}/didClose`, { path: filePath }).catch(() => {})
  }

  /**
   * Debounce a full-text didChange for an open document. `read` is called when
   * the change is actually sent, so a burst of edits costs one read + POST;
   * it returns null when the text is gone (model disposed) and nothing is sent.
   */
  scheduleChange(wsId: string, filePath: string, read: () => string | null): void {
    const session = this.sessions.get(wsId)
    if (!session?.openRefs.has(filePath)) return
    session.cancelChange(filePath)
    session.pendingChanges.set(filePath, {
      read,
      timer: setTimeout(() => void this.flushChange(wsId, filePath), CHANGE_DEBOUNCE_MS),
    })
  }

  /**
   * Send a scheduled didChange now. Position-addressed requests (completion,
   * signature help, …) await this first so the server answers against the
   * text the user sees, not the text from before the debounce window.
   */
  async flushChange(wsId: string, filePath: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const pending = session?.pendingChanges.get(filePath)
    if (!session || !pending) return
    session.cancelChange(filePath)
    const text = pending.read()
    if (text === null) return
    const deferred = session.deferred.get(filePath)
    if (deferred) {
      deferred.content = text
      return
    }
    const base = session.base()
    if (!base) return
    await post(`${base}/didChange`, { path: filePath, text }).catch(() => {})
  }

  /** Receive `wsId`'s diagnostics, starting with the current batch. */
  onDiagnosticsUpdate(wsId: string, handler: DiagnosticsHandler): () => void {
    const session = this.session(wsId)
    session.addHandler(handler)
    return () => {
      session.handlers.delete(handler)
      this.releaseIfIdle(session)
    }
  }
}

export { LspClientImpl as LspClient }
