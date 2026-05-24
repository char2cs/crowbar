// web/src/features/lsp/lsp-client.ts
// LSP is out of scope for this session. All methods are no-ops.
// FUTURE: wire to a real language server via the Go backend.

export interface InlayHint {
  line: number;
  character: number;
  label: string;
  kind?: string;
  paddingLeft: boolean;
  paddingRight: boolean;
}

interface Position {
  line: number;
  character: number;
}

interface Range {
  start: Position;
  end: Position;
}

interface SignatureInfo {
  label: string;
  documentation?: { kind: string; value: string } | string;
  parameters?: {
    label: string | [number, number];
    documentation?: { kind: string; value: string } | string;
  }[];
  activeParameter?: number;
}

interface SignatureHelpResult {
  signatures: SignatureInfo[];
  activeSignature?: number;
  activeParameter?: number;
}

interface SemanticToken {
  line: number;
  startChar: number;
  length: number;
  tokenType: number;
  tokenModifiers: number;
}

interface CodeLensItem {
  line: number;
  title: string;
  command?: string;
  arguments?: unknown[];
}

export class LspClient {
  private static _instance: LspClient | null = null;

  static getInstance(): LspClient {
    if (!LspClient._instance) {
      LspClient._instance = new LspClient();
    }
    return LspClient._instance;
  }

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

  getActiveServerEntryForFile(_filePath: string): null { return null }

  getActiveServerEntries(): { key: string; displayName: string }[] {
    return [];
  }

  async prepareRename(
    _filePath: string,
    _line: number,
    _column: number,
  ): Promise<{ range: Range; start: Position; end: Position; placeholder: string } | null> {
    return null;
  }

  async rename(
    _filePath: string,
    _line: number,
    _column: number,
    _newName: string,
  ): Promise<null> {
    return null;
  }

  async getCodeLens(_filePath: string): Promise<CodeLensItem[]> {
    return [];
  }

  async getInlayHints(
    _filePath: string,
    _startLine: number,
    _endLine: number,
  ): Promise<InlayHint[]> {
    return [];
  }

  async getSemanticTokens(_filePath: string): Promise<SemanticToken[]> {
    return [];
  }

  async getSignatureHelp(
    _filePath: string,
    _line: number,
    _column: number,
  ): Promise<SignatureHelpResult | null> {
    return null;
  }
}

export const lspClient = new LspClient()
export default lspClient
