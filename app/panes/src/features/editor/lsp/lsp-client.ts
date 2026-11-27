import { apiFetch } from '@/lib/api/fetch'
import { holdScope, onSignal, watchScope } from '@/core/stream/stream'
import { report } from '@/lib/errors/report'
import { hostIdForWorkspace, isHostResolved, subscribeHostResolution } from '@/lib/host-resolve'
import { isHomeWorkspace, lspBaseForWorkspace } from '@/lib/workspace-scope-url'
import { getOwningChatId } from '@/lib/workspace-scope'

const reportSync = (err: unknown) =>
  report(err, { action: 'Sync the file with its language server' })

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

export const LSP_MARKER_OWNER = 'crowbar-lsp'

const CHANGE_DEBOUNCE_MS = 400

type DiagnosticsHandler = (filePath: string, diagnostics: LspDiagnostic[]) => void
type OpenedListener = (wsId: string, filePath: string) => void

type ReadText = () => string | null

interface OpenDocument {
  refs: number
  languageId: string
  read: ReadText
}

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

class WorkspaceSession {
  readonly handlers = new Set<DiagnosticsHandler>()
  readonly documents = new Map<string, OpenDocument>()
  readonly pendingChanges = new Map<string, ReturnType<typeof setTimeout>>()
  private lastByFile = new Map<string, LspDiagnostic[]>()
  private hold: { base: string; close: () => void } | null = null
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
    this.followScope()
  }

  base(): string | null {
    return this.hold?.base ?? null
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
    this.releaseScope()
    for (const filePath of [...this.pendingChanges.keys()]) this.cancelChange(filePath)
  }

  private followScope(): void {
    const base = lspBase(this.wsId)
    if (base === (this.hold?.base ?? null)) return
    this.releaseScope()
    if (!base) return
    const scope = `lsp:${getOwningChatId(this.wsId)}`
    const hostId = hostIdForWorkspace(this.wsId)
    const hold = { base, close: () => {} }
    const stopSignals = onSignal<DiagnosticsEvent>(hostId, scope, 'diagnostics', (event) =>
      this.dispatch(event),
    )
    let firstSync = true
    const stopWatch = watchScope(hostId, scope, {
      synced: () => {
        if (!firstSync) this.openDocuments(base)
        firstSync = false
      },
      failed: () => {
        firstSync = false
      },
    })
    const release = holdScope(hostId, scope)
    hold.close = () => {
      release()
      stopWatch()
      stopSignals()
    }
    this.hold = hold
    this.openDocuments(base)
  }

  private openDocuments(base: string): void {
    for (const [path, { languageId, read }] of this.documents) {
      const text = read()
      if (text !== null) this.sendOpen(base, path, languageId, text)
    }
  }

  private releaseScope(): void {
    if (!this.hold) return
    this.hold.close()
    this.hold = null
    this.publish(new Map([...this.lastByFile.keys()].map((path) => [path, []])))
  }

  private dispatch(event: DiagnosticsEvent): void {
    if (!event.diagnostics) return
    const byFile = new Map<string, LspDiagnostic[]>()
    for (const diag of event.diagnostics) {
      const list = byFile.get(diag.filePath) ?? []
      list.push(diag)
      byFile.set(diag.filePath, list)
    }
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
        void this.postOpen(wsId, base, path, languageId, text).catch(reportSync)
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

  onDocumentOpened(listener: OpenedListener): () => void {
    this.openListeners.add(listener)
    return () => {
      this.openListeners.delete(listener)
    }
  }

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

  restart(wsId: string, filePath: string): Promise<LspServerStatus | null> {
    return this.request<LspServerStatus>(wsId, 'restart', { path: filePath })
  }

  async reopen(wsId: string, filePath: string, content: string, languageId: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const base = session?.base()
    if (!session?.documents.has(filePath) || !base) return
    await this.postOpen(wsId, base, filePath, languageId, content)
  }

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
    const base = session.base()
    const text = read()
    if (!base || text === null) return
    await this.postOpen(wsId, base, filePath, languageId, text).catch(reportSync)
  }

  async documentSave(wsId: string, filePath: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const base = session?.base()
    if (!session?.documents.has(filePath) || !base) return
    await post(`${base}/didSave`, { path: filePath }).catch(reportSync)
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
    await post(`${base}/didClose`, { path: filePath }).catch(reportSync)
  }

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

  async flushChange(wsId: string, filePath: string): Promise<void> {
    const session = this.sessions.get(wsId)
    const open = session?.documents.get(filePath)
    if (!session || !open || !session.pendingChanges.has(filePath)) return
    session.cancelChange(filePath)
    const base = session.base()
    const text = open.read()
    if (!base || text === null) return
    await post(`${base}/didChange`, { path: filePath, text }).catch(reportSync)
  }

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
