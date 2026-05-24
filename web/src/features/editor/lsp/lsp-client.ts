// LSP is out of scope for Crowbar. All methods are no-ops.
// Original Athas file replaced with stub.

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

class LspClientImpl {
  static _instance: LspClientImpl | null = null
  static getInstance(): LspClientImpl {
    if (!LspClientImpl._instance) LspClientImpl._instance = new LspClientImpl()
    return LspClientImpl._instance
  }

  isStarted = false

  isRunning(): boolean { return false }
  isAvailable(): boolean { return false }

  async startServer(_filePath: string): Promise<void> {}
  async stopServer(): Promise<void> {}

  async getCompletions(_filePath: string, _line: number, _character: number, _trigger?: string): Promise<unknown[]> {
    return []
  }

  async getHover(_filePath: string, _line: number, _character: number): Promise<null> {
    return null
  }

  async getDefinition(_filePath: string, _line: number, _character: number): Promise<LspLocation | null> {
    return null
  }

  async getReferences(_filePath: string, _line: number, _character: number): Promise<LspLocation[]> {
    return []
  }

  async getDocumentSymbols(_filePath: string): Promise<unknown[]> {
    return []
  }

  async formatDocument(_filePath: string): Promise<unknown[]> {
    return []
  }

  async getCodeActions(_filePath: string, _line: number, _character: number): Promise<unknown[]> {
    return []
  }

  async applyCodeAction(_action: unknown): Promise<{ success: boolean }> {
    return { success: false }
  }

  async prepareRename(_filePath: string, _line: number, _character: number): Promise<null> {
    return null
  }

  async rename(_filePath: string, _line: number, _character: number, _newName: string): Promise<null> {
    return null
  }

  async getInlayHints(_filePath: string, _startLine: number, _endLine: number): Promise<unknown[]> {
    return []
  }

  async getSemanticTokens(_filePath: string): Promise<null> {
    return null
  }

  async documentOpen(_filePath: string, _content: string): Promise<void> {}
  async documentChange(_filePath: string, _changes: unknown[]): Promise<void> {}
  async documentSave(_filePath: string): Promise<void> {}
  async documentClose(_filePath: string): Promise<void> {}

  async getSignatureHelp(_filePath: string, _line: number, _character: number): Promise<null> {
    return null
  }

  onDiagnosticsUpdate(_handler: (_filePath: string, _diagnostics: unknown[]) => void): () => void {
    return () => {}
  }

  notifyDocumentOpen(_filePath: string, _content: string): Promise<void> { return this.documentOpen(_filePath, _content) }
  notifyDocumentClose(_filePath: string): Promise<void> { return this.documentClose(_filePath) }
  notifyDocumentChange(_filePath: string, _changes: unknown[]): Promise<void> { return this.documentChange(_filePath, _changes) }
  notifyDocumentSave(_filePath: string): Promise<void> { return this.documentSave(_filePath) }

  async startForFile(_filePath: string, _rootFolderPath?: string, _opts?: { forceRetry?: boolean }): Promise<boolean> { return false }
  getActiveServerEntries(): unknown[] { return [] }
  async getActiveServerEntryForFile(_filePath: string, _languageId?: string): Promise<null> { return null }
  async restartTrackedServer(_serverId: string): Promise<void> {}
  async stopTrackedServer(_serverId: string): Promise<void> {}
}

/** Alias for LspClientImpl exported as LspClient for Athas compatibility */
export { LspClientImpl as LspClient }
export type { LspClientImpl as LspClientType }
export const lspClient = new LspClientImpl()
export default lspClient
