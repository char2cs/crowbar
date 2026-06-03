// LSP is out of scope for Crowbar. All methods are no-ops.
// Original Athas file replaced with stub.

import type { CompletionItem } from "vscode-languageserver-types"

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

  async getCompletions(_filePath: string, _line: number, _character: number, _trigger?: string): Promise<CompletionItem[]> {
    return []
  }

  async getHover(_filePath: string, _line: number, _character: number): Promise<null> {
    return null
  }

  async getDefinition(_filePath: string, _line: number, _character: number): Promise<Definition[] | null> {
    return null
  }

  async getReferences(_filePath: string, _line: number, _character: number): Promise<LspLocation[]> {
    return []
  }

  async getDocumentSymbols(_filePath: string): Promise<unknown[]> {
    return []
  }

  async formatDocument(_filePath: string, _content?: string): Promise<string | null> {
    return null
  }

  async formatRange(_filePath: string, _content?: string, _range?: unknown): Promise<string | null> {
    return null
  }

  async getCodeActions(_filePath: string, _line: number, _character: number): Promise<unknown[]> {
    return []
  }

  async applyCodeAction(_filePath: string, _action: unknown): Promise<{ success: boolean }> {
    return { success: false }
  }

  async prepareRename(_filePath: string, _line: number, _character: number): Promise<{
    range: { start: { line: number; character: number }; end: { line: number; character: number } };
    start: { line: number; character: number };
    end: { line: number; character: number };
    placeholder: string;
  } | null> {
    return null
  }

  async rename(_filePath: string, _line: number, _character: number, _newName: string): Promise<null> {
    return null
  }

  async getInlayHints(_filePath: string, _startLine: number, _endLine: number): Promise<InlayHint[]> {
    return []
  }

  async getSemanticTokens(_filePath: string): Promise<{ line: number; startChar: number; length: number; tokenType: number; tokenModifiers: number }[]> {
    return []
  }

  async getCodeLens(_filePath: string): Promise<{ line: number; title: string; command?: string; arguments?: unknown[] }[]> {
    return []
  }

  async getSignatureHelp(_filePath: string, _line: number, _character: number): Promise<{
    signatures: {
      label: string;
      documentation?: { kind: string; value: string } | string;
      parameters?: { label: string | [number, number]; documentation?: { kind: string; value: string } | string }[];
      activeParameter?: number;
    }[];
    activeSignature?: number;
    activeParameter?: number;
  } | null> {
    return null
  }

  async getSignatureTriggerCharacters(_filePath: string): Promise<string[]> {
    return []
  }

  async documentOpen(_filePath: string, _content: string): Promise<void> {}
  async documentChange(_filePath: string, _changes: unknown, _version?: number): Promise<void> {}
  async documentSave(_filePath: string, _content?: string): Promise<void> {}
  async documentClose(_filePath: string): Promise<void> {}

  onDiagnosticsUpdate(_handler: (_filePath: string, _diagnostics: unknown[]) => void): () => void {
    return () => {}
  }

  notifyDocumentOpen(_filePath: string, _content: string): Promise<void> { return this.documentOpen(_filePath, _content) }
  notifyDocumentClose(_filePath: string): Promise<void> { return this.documentClose(_filePath) }
  notifyDocumentChange(_filePath: string, _changes: unknown, _version?: number): Promise<void> { return this.documentChange(_filePath, _changes, _version) }
  notifyDocumentSave(_filePath: string, _content?: string): Promise<void> { return this.documentSave(_filePath, _content) }

  async startForFile(_filePath: string, _rootFolderPath?: string, _opts?: { forceRetry?: boolean }): Promise<boolean> { return false }
  async stopForFile(_filePath: string): Promise<void> {}
  getActiveServerEntries(): { key: string; displayName: string }[] { return [] }
  async getActiveServerEntryForFile(_filePath: string, _languageId?: string): Promise<null> { return null }
  async restartTrackedServer(_serverId: string): Promise<void> {}
  async stopTrackedServer(_serverId: string): Promise<void> {}
}

/** Alias for LspClientImpl exported as LspClient for Athas compatibility */
export { LspClientImpl as LspClient }
export type { LspClientImpl as LspClientType }
export const lspClient = new LspClientImpl()
export default lspClient
