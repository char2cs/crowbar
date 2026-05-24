// web/src/features/lsp/lsp-client.ts
// LSP is out of scope for this session. All methods are no-ops.
// FUTURE: wire to a real language server via the Go backend.

export class LspClient {
  isEnabled(): boolean { return false }
  async start(): Promise<void> {}
  async stop(): Promise<void> {}
  async getCompletions(): Promise<[]> { return [] }
  async getHover(): Promise<null> { return null }
  async getDefinition(): Promise<null> { return null }
  async formatDocument(): Promise<[]> { return [] }
  async getReferences(): Promise<null> { return null }
  async getDocumentSymbols(): Promise<[]> { return [] }
  async documentOpen(): Promise<void> {}
  async documentChange(): Promise<void> {}
  async documentSave(): Promise<void> {}
  async documentClose(): Promise<void> {}
}

export const lspClient = new LspClient()
export default lspClient
