// LSP client backed by the Go daemon: document sync (didOpen/didChange/
// didSave/didClose) over REST, diagnostics over the owning chat's /lsp/ws
// topic, and the feature requests the Monaco providers (monaco-lsp-providers.ts)
// make. Everything is addressed by the workspace the caller names — the
// buffer's own — never by whichever workspace is active.

import { apiFetch } from '@/lib/api'
import { wsManager } from '@/lib/ws/manager'
import { isReconnectSentinel } from '@/lib/ws/types'
import { isHostResolved, subscribeHostResolution } from '@/lib/host-resolve'
import { isHomeWorkspace, lspBaseForWorkspace } from '@/lib/workspace-scope-url'
import { getOwningChatId } from '@/lib/workspace-scope'

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

/** A document's text as its model has it now; null once the model is gone. */
type ReadText = () => string | null

interface OpenDocument {
  /** One didOpen for the first holder, didClose with the last. */
  refs: number
  languageId: string
  read: ReadText
}

/** The LSP route base for `wsId`, or null while it has none (home, no chat, host not resolved). */
function lspBase(wsId: string): string | null {
  if (isHomeWorkspace(wsId) || !getOwningChatId(wsId) || !isHostResolved({ wsId })) return null
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
 * owning chat and host are known (the scope registry and host resolution own those).
 */
class WorkspaceSession {
  readonly handlers = new Set<DiagnosticsHandler>()
  readonly documents = new Map<string, OpenDocument>()
  /** Debounced didChange timers per path. */
  readonly pendingChanges = new Map<string, ReturnType<typeof setTimeout>>()
  private lastByFile = new Map<string, LspDiagnostic[]>()
  /** The route base the socket is on, and its unsubscribe. */
  private socket: { base: string; close: () => void } | null = null
  private readonly stopWatching: () => void

  constructor(
    readonly wsId: string,
    private readonly sendOpen: (
      base: string,
      path: string,
      languageId: string,
      text: string,
    ) => void,
  ) {
    this.stopWatching = subscribeHostResolution(wsId, () => this.followScope())
    try {
      this.followScope()
    } catch (error) {
      this.dispose() // a session that never gets stored must not leave a listener behind
      throw error
    }
  }

  base(): string | null {
    return lspBase(this.wsId)
  }

  isIdle(): boolean {
    return this.handlers.size === 0 && this.documents.size === 0
  }

  addHandler(handler: DiagnosticsHandler): void {
    this.handlers.add(handler)
    for (const [filePath, diagnostics] of this.lastByFile) handler(filePath, diagnostics)
  }

  cancelChange(filePath: string): void {
    clearTimeout(this.pendingChanges.get(filePath))
    this.pendingChanges.delete(filePath)
  }

  dispose(): void {
    this.stopWatching()
    this.closeSocket()
    for (const filePath of [...this.pendingChanges.keys()]) this.cancelChange(filePath)
  }

  // The route base arriving (owning chat recorded, host resolved) or changing opens a
  // socket; it going away (workspace deleted, host revoked) closes it.
  private followScope(): void {
    const base = this.base()
    if (base === (this.socket?.base ?? null)) return
    this.closeSocket()
    if (!base) return
    this.socket = { base, close: wsManager.subscribe(`${base}/ws`, (raw) => this.dispatch(raw)) }
    this.openDocuments(base)
  }

  // The daemon closes a workspace's servers when its last LSP socket drops, so a new or
  // reopened socket faces servers with nothing open — and edits sent meanwhile were lost.
  private openDocuments(base: string): void {
    for (const [path, { languageId, read }] of this.documents) {
      const text = read()
      if (text !== null) this.sendOpen(base, path, languageId, text)
    }
  }

  private closeSocket(): void {
    if (!this.socket) return
    this.socket.close()
    this.socket = null
    // Markers from a server that is gone would never be cleared otherwise.
    this.publish(new Map([...this.lastByFile.keys()].map((path) => [path, []])))
  }

  private dispatch(frame: unknown): void {
    if (isReconnectSentinel(frame)) {
      if (this.socket) this.openDocuments(this.socket.base)
      return
    }
    const event = frame as DiagnosticsEvent | null
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
    if (!session?.documents.has(filePath) || !base) return
    await this.postOpen(wsId, base, filePath, languageId, content)
  }

  /** Open `filePath` for LSP; `read` is asked for its text each time it is (re)sent. */
  async documentOpen(
    wsId: string,
    filePath: string,
    read: ReadText,
    languageId: string,
  ): Promise<void> {
    const session = this.session(wsId)
    const open = session.documents.get(filePath)
    if (open) {
      open.refs++
      return
    }
    session.documents.set(filePath, { refs: 1, languageId, read })
    // No route yet (owning chat or host not known): the socket that brings one opens it.
    const base = session.base()
    const text = read()
    if (!base || text === null) return
    await this.postOpen(wsId, base, filePath, languageId, text).catch(() => {})
  }

  async documentSave(wsId: string, filePath: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const base = session?.base()
    if (!session?.documents.has(filePath) || !base) return
    await post(`${base}/didSave`, { path: filePath }).catch(() => {})
  }

  async documentClose(wsId: string, filePath: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const open = session?.documents.get(filePath)
    if (!session || !open) return
    if (open.refs > 1) {
      open.refs--
      return
    }
    session.documents.delete(filePath)
    session.cancelChange(filePath)
    const base = session.base()
    this.releaseIfIdle(session)
    if (!base) return
    await post(`${base}/didClose`, { path: filePath }).catch(() => {})
  }

  /**
   * Debounce a full-text didChange for an open document. `read` is called when
   * the change is actually sent, so a burst of edits costs one read + POST;
   * it returns null when the text is gone (model disposed) and nothing is sent.
   */
  scheduleChange(wsId: string, filePath: string, read: ReadText): void {
    const session = this.sessions.get(wsId)
    const open = session?.documents.get(filePath)
    if (!session || !open) return
    open.read = read
    session.cancelChange(filePath)
    session.pendingChanges.set(
      filePath,
      setTimeout(() => void this.flushChange(wsId, filePath), CHANGE_DEBOUNCE_MS),
    )
  }

  /**
   * Send a scheduled didChange now. Position-addressed requests (completion,
   * signature help, …) await this first so the server answers against the
   * text the user sees, not the text from before the debounce window.
   */
  async flushChange(wsId: string, filePath: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const open = session?.documents.get(filePath)
    if (!session || !open || !session.pendingChanges.has(filePath)) return
    session.cancelChange(filePath)
    // Without a route the edit waits in the model: the next socket's didOpen carries it.
    const base = session.base()
    const text = open.read()
    if (!base || text === null) return
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
