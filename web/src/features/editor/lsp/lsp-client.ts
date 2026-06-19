// LSP client backed by the Go daemon: document sync (didOpen/didChange/
// didClose) over REST and diagnostics over the /v0/ws/lsp topic. The
// position-addressed features (completion/hover/…) remain no-ops for now; only
// the diagnostics path is wired, which is what renders editor squiggles.

import type { CompletionItem } from 'vscode-languageserver-types'
import { apiFetch } from '@/lib/api'
import { wsManager } from '@/lib/ws/manager'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceBase } from '@/lib/workspace-scope-url'

export interface LspError {
  message: string
  code?: string
}

export interface LspLocation {
  uri: string
  range: {
    start: { line: number; character: number }
    end: { line: number; character: number }
  }
}

export interface Definition {
  uri: string
  range: {
    start: { line: number; character: number }
    end: { line: number; character: number }
  }
}

export interface TextEdit {
  range: {
    start: { line: number; character: number }
    end: { line: number; character: number }
  }
  newText: string
}

export interface InlayHint {
  line: number
  character: number
  label: string
  kind?: string
  paddingLeft: boolean
  paddingRight: boolean
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

type DiagnosticsHandler = (filePath: string, diagnostics: LspDiagnostic[]) => void

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
  // Open refcount per file (I4): the retained-editor path has TWO independent
  // owners that each open/close the same managed file — the satellite hook
  // (diagnostics lifecycle) and `useLspIntegration` (server start + rich
  // services). Reference-count opens so exactly ONE `/didOpen` POST goes out
  // (the first opener) and `/didClose` only fires when the LAST holder closes.
  private openRefs = new Map<string, number>()

  isRunning(): boolean {
    return this.unsubscribe !== null
  }
  isAvailable(): boolean {
    return true
  }

  // Subscribe to the workspace's diagnostics topic. Snapshot-on-subscribe
  // replays current diagnostics; later batches arrive live.
  private ensureSubscribed(): void {
    const wsId = getActiveWorkspaceId()
    if (!wsId) return
    if (this.wsId === wsId && this.unsubscribe) return

    this.unsubscribe?.()
    this.wsId = wsId
    this.lastByFile.clear()
    // The new workspace's documents are not open yet; drop stale refcounts so a
    // first open there still POSTs `/didOpen`.
    this.openRefs.clear()
    this.unsubscribe = wsManager.subscribe(`${workspaceBase(wsId)}/lsp/ws`, (raw) =>
      this.dispatch(raw as DiagnosticsEvent),
    )
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
    for (const [filePath, diagnostics] of byFile) {
      for (const handler of this.handlers) handler(filePath, diagnostics)
    }
  }

  private wsBase(): string | null {
    const wsId = getActiveWorkspaceId()
    return wsId ? `${workspaceBase(wsId)}/lsp` : null
  }

  async startServer(_filePath: string): Promise<void> {
    this.ensureSubscribed()
  }
  async stopServer(): Promise<void> {}

  async getCompletions(
    _filePath: string,
    _line: number,
    _character: number,
    _trigger?: string,
  ): Promise<CompletionItem[]> {
    return []
  }
  async getHover(_filePath: string, _line: number, _character: number): Promise<null> {
    return null
  }
  async getDefinition(
    _filePath: string,
    _line: number,
    _character: number,
  ): Promise<Definition[] | null> {
    return null
  }
  async getReferences(
    _filePath: string,
    _line: number,
    _character: number,
  ): Promise<LspLocation[]> {
    return []
  }
  async getDocumentSymbols(_filePath: string): Promise<unknown[]> {
    return []
  }
  async formatDocument(_filePath: string, _content?: string): Promise<string | null> {
    return null
  }
  async formatRange(
    _filePath: string,
    _content?: string,
    _range?: unknown,
  ): Promise<string | null> {
    return null
  }
  async getCodeActions(_filePath: string, _line: number, _character: number): Promise<unknown[]> {
    return []
  }
  async applyCodeAction(_filePath: string, _action: unknown): Promise<{ success: boolean }> {
    return { success: false }
  }

  async prepareRename(
    _filePath: string,
    _line: number,
    _character: number,
  ): Promise<{
    range: { start: { line: number; character: number }; end: { line: number; character: number } }
    start: { line: number; character: number }
    end: { line: number; character: number }
    placeholder: string
  } | null> {
    return null
  }

  async rename(
    _filePath: string,
    _line: number,
    _character: number,
    _newName: string,
  ): Promise<null> {
    return null
  }

  async getInlayHints(
    _filePath: string,
    _startLine: number,
    _endLine: number,
  ): Promise<InlayHint[]> {
    return []
  }

  async getSemanticTokens(
    _filePath: string,
  ): Promise<
    { line: number; startChar: number; length: number; tokenType: number; tokenModifiers: number }[]
  > {
    return []
  }

  async getCodeLens(
    _filePath: string,
  ): Promise<{ line: number; title: string; command?: string; arguments?: unknown[] }[]> {
    return []
  }

  async getSignatureHelp(
    _filePath: string,
    _line: number,
    _character: number,
  ): Promise<{
    signatures: {
      label: string
      documentation?: { kind: string; value: string } | string
      parameters?: {
        label: string | [number, number]
        documentation?: { kind: string; value: string } | string
      }[]
      activeParameter?: number
    }[]
    activeSignature?: number
    activeParameter?: number
  } | null> {
    return null
  }

  async getSignatureTriggerCharacters(_filePath: string): Promise<string[]> {
    return []
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
    if (!base) return
    await apiFetch(`${base}/didOpen`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: filePath, languageId, text: content }),
    }).catch(() => {})
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

  async documentSave(_filePath: string, _content?: string): Promise<void> {}

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
    const base = this.wsBase()
    if (!base) return
    await apiFetch(`${base}/didClose`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path: filePath }),
    }).catch(() => {})
  }

  onDiagnosticsUpdate(handler: DiagnosticsHandler): () => void {
    this.ensureSubscribed()
    this.handlers.add(handler)
    // Replay current diagnostics so a late subscriber paints immediately.
    for (const [filePath, diagnostics] of this.lastByFile) handler(filePath, diagnostics)
    return () => {
      this.handlers.delete(handler)
    }
  }

  notifyDocumentOpen(filePath: string, content: string, languageId = 'plaintext'): Promise<void> {
    return this.documentOpen(filePath, content, languageId)
  }
  notifyDocumentClose(filePath: string): Promise<void> {
    return this.documentClose(filePath)
  }
  // The editor calls this with incremental edits; full-text didChange is driven
  // separately by the Monaco component effect, so this stays a no-op.
  async notifyDocumentChange(
    _filePath: string,
    _changes: unknown,
    _version?: number,
  ): Promise<void> {}
  notifyDocumentSave(filePath: string, content?: string): Promise<void> {
    return this.documentSave(filePath, content)
  }

  async startForFile(
    _filePath: string,
    _rootFolderPath?: string,
    _opts?: { forceRetry?: boolean },
  ): Promise<boolean> {
    return false
  }
  async stopForFile(_filePath: string): Promise<void> {}
  getActiveServerEntries(): { key: string; displayName: string }[] {
    return []
  }
  async getActiveServerEntryForFile(_filePath: string, _languageId?: string): Promise<null> {
    return null
  }
  async restartTrackedServer(_serverId: string): Promise<void> {}
  async stopTrackedServer(_serverId: string): Promise<void> {}
}

export { LspClientImpl as LspClient }
export type { LspClientImpl as LspClientType }
export const lspClient = new LspClientImpl()
export default lspClient
