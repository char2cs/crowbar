/**
 * Pure LSP ⇄ Monaco conversions for the daemon-backed Monaco providers
 * (monaco-lsp-providers.ts). No runtime Monaco import: Monaco's enums are
 * plain numbers, mirrored here, so this stays unit-testable and out of any
 * chunk that does not already load Monaco.
 *
 * Positions: LSP is 0-based line/character (UTF-16), Monaco is 1-based
 * lineNumber/column (UTF-16) — a plain ±1 on both axes.
 */
import type * as Monaco from 'monaco-editor'
import type {
  CodeAction,
  Command,
  CompletionItem,
  CompletionList,
  Diagnostic,
  DocumentSymbol,
  Hover,
  MarkedString,
  MarkupContent,
  SignatureHelp,
  SymbolInformation,
  TextEdit,
} from 'vscode-languageserver-types'

export interface LspRange {
  start: { line: number; character: number }
  end: { line: number; character: number }
}

/** A daemon Location: workspace-relative path (absolute when outside the worktree). */
export interface DaemonLocation {
  filePath: string
  range: LspRange
}

/** The daemon's WorkspaceEdit: edits keyed by workspace-relative path. */
export interface DaemonWorkspaceEdit {
  changes?: Record<string, TextEdit[]>
  documentChanges?: Array<{ textDocument: { uri: string }; edits: TextEdit[] } | unknown>
}

export function toMonacoRange(range: LspRange): Monaco.IRange {
  return {
    startLineNumber: range.start.line + 1,
    startColumn: range.start.character + 1,
    endLineNumber: range.end.line + 1,
    endColumn: range.end.character + 1,
  }
}

export function toLspPosition(position: Monaco.IPosition): { line: number; character: number } {
  return { line: position.lineNumber - 1, character: position.column - 1 }
}

export function toLspRange(range: Monaco.IRange): LspRange {
  return {
    start: { line: range.startLineNumber - 1, character: range.startColumn - 1 },
    end: { line: range.endLineNumber - 1, character: range.endColumn - 1 },
  }
}

// LSP CompletionItemKind (1-based, LSP order) → monaco.languages.CompletionItemKind.
const COMPLETION_KIND: Record<number, number> = {
  1: 18, // Text
  2: 0, // Method
  3: 1, // Function
  4: 2, // Constructor
  5: 3, // Field
  6: 4, // Variable
  7: 5, // Class
  8: 7, // Interface
  9: 8, // Module
  10: 9, // Property
  11: 12, // Unit
  12: 13, // Value
  13: 15, // Enum
  14: 17, // Keyword
  15: 28, // Snippet
  16: 19, // Color
  17: 20, // File
  18: 21, // Reference
  19: 23, // Folder
  20: 16, // EnumMember
  21: 14, // Constant
  22: 6, // Struct
  23: 10, // Event
  24: 11, // Operator
  25: 24, // TypeParameter
}
const MONACO_COMPLETION_TEXT = 18
const INSERT_AS_SNIPPET = 4

function toMarkdown(
  content: MarkupContent | MarkedString | MarkedString[] | string | undefined | null,
): Monaco.IMarkdownString[] {
  if (content === undefined || content === null) return []
  if (Array.isArray(content)) return content.flatMap((part) => toMarkdown(part))
  if (typeof content === 'string') return content ? [{ value: content }] : []
  if ('kind' in content) {
    if (!content.value) return []
    return content.kind === 'markdown'
      ? [{ value: content.value }]
      : [{ value: escapeMarkdown(content.value) }]
  }
  // MarkedString { language, value }: a fenced code block.
  return content.value ? [{ value: '```' + content.language + '\n' + content.value + '\n```' }] : []
}

function escapeMarkdown(text: string): string {
  return text.replace(/[\\`*_{}[\]()#+\-.!|<>]/g, '\\$&')
}

function documentation(
  doc: string | MarkupContent | undefined,
): string | Monaco.IMarkdownString | undefined {
  if (doc === undefined) return undefined
  if (typeof doc === 'string') return doc
  return doc.kind === 'markdown' ? { value: doc.value } : doc.value
}

function isInsertReplaceEdit(
  edit: CompletionItem['textEdit'],
): edit is { newText: string; insert: LspRange; replace: LspRange } {
  return !!edit && 'insert' in edit
}

export function completionToMonaco(
  result: CompletionList | CompletionItem[] | null,
  defaultRange: Monaco.IRange,
): Monaco.languages.CompletionList {
  if (!result) return { suggestions: [] }
  const items = Array.isArray(result) ? result : result.items
  const incomplete = Array.isArray(result) ? false : result.isIncomplete
  const suggestions = items.map((item): Monaco.languages.CompletionItem => {
    const edit = item.textEdit
    let range: Monaco.IRange | Monaco.languages.CompletionItemRanges = defaultRange
    let insertText = item.insertText ?? item.label
    if (edit) {
      insertText = edit.newText
      range = isInsertReplaceEdit(edit)
        ? { insert: toMonacoRange(edit.insert), replace: toMonacoRange(edit.replace) }
        : toMonacoRange(edit.range)
    }
    return {
      label: item.labelDetails
        ? {
            label: item.label,
            detail: item.labelDetails.detail,
            description: item.labelDetails.description,
          }
        : item.label,
      kind: COMPLETION_KIND[item.kind ?? 1] ?? MONACO_COMPLETION_TEXT,
      detail: item.detail,
      documentation: documentation(item.documentation),
      sortText: item.sortText,
      filterText: item.filterText,
      preselect: item.preselect,
      insertText,
      insertTextRules: item.insertTextFormat === 2 ? INSERT_AS_SNIPPET : undefined,
      range,
      additionalTextEdits: item.additionalTextEdits?.map((e) => ({
        range: toMonacoRange(e.range),
        text: e.newText,
      })),
      tags: item.deprecated || item.tags?.includes(1) ? [1] : undefined,
    }
  })
  return { suggestions, incomplete }
}

export function hoverToMonaco(hover: Hover | null): Monaco.languages.Hover | null {
  if (!hover) return null
  const contents = toMarkdown(hover.contents)
  if (contents.length === 0) return null
  return { contents, range: hover.range ? toMonacoRange(hover.range) : undefined }
}

export function signatureHelpToMonaco(
  help: SignatureHelp | null,
): Monaco.languages.SignatureHelpResult | null {
  if (!help || help.signatures.length === 0) return null
  return {
    value: {
      activeSignature: help.activeSignature ?? 0,
      activeParameter: help.activeParameter ?? 0,
      signatures: help.signatures.map((sig) => ({
        label: sig.label,
        documentation: documentation(sig.documentation),
        activeParameter: sig.activeParameter ?? undefined,
        parameters: (sig.parameters ?? []).map((param) => ({
          label: param.label,
          documentation: documentation(param.documentation),
        })),
      })),
    },
    dispose() {},
  }
}

function isDocumentSymbol(symbol: DocumentSymbol | SymbolInformation): symbol is DocumentSymbol {
  return 'selectionRange' in symbol
}

export function documentSymbolsToMonaco(
  symbols: Array<DocumentSymbol | SymbolInformation> | null,
): Monaco.languages.DocumentSymbol[] {
  if (!symbols) return []
  const convert = (symbol: DocumentSymbol | SymbolInformation): Monaco.languages.DocumentSymbol => {
    if (isDocumentSymbol(symbol)) {
      return {
        name: symbol.name,
        detail: symbol.detail ?? '',
        kind: symbol.kind - 1,
        tags: symbol.deprecated ? [1] : [],
        range: toMonacoRange(symbol.range),
        selectionRange: toMonacoRange(symbol.selectionRange),
        children: symbol.children?.map(convert),
      }
    }
    const range = toMonacoRange(symbol.location.range)
    return {
      name: symbol.name,
      detail: symbol.containerName ?? '',
      kind: symbol.kind - 1,
      tags: symbol.deprecated ? [1] : [],
      range,
      selectionRange: range,
      containerName: symbol.containerName,
    }
  }
  return symbols.map(convert)
}

/** Monaco marker severity (Hint 1, Info 2, Warning 4, Error 8) → LSP severity. */
export function markerToLspDiagnostic(marker: Monaco.editor.IMarkerData): Diagnostic {
  const severity =
    marker.severity === 8 ? 1 : marker.severity === 4 ? 2 : marker.severity === 2 ? 3 : 4
  const code = typeof marker.code === 'string' ? marker.code : (marker.code?.value ?? undefined)
  return {
    range: toLspRange(marker),
    severity: severity as Diagnostic['severity'],
    message: marker.message,
    source: marker.source,
    code,
  }
}

/** Every text edit a workspace edit carries, grouped by workspace-relative path. */
export function workspaceEditByPath(edit: DaemonWorkspaceEdit | null): Map<string, TextEdit[]> {
  const byPath = new Map<string, TextEdit[]>()
  if (!edit) return byPath
  for (const [path, edits] of Object.entries(edit.changes ?? {})) {
    byPath.set(path, [...(byPath.get(path) ?? []), ...edits])
  }
  for (const change of edit.documentChanges ?? []) {
    if (!change || typeof change !== 'object' || !('textDocument' in change)) continue
    const docChange = change as { textDocument: { uri: string }; edits: TextEdit[] }
    const path = docChange.textDocument.uri
    byPath.set(path, [...(byPath.get(path) ?? []), ...docChange.edits])
  }
  return byPath
}

export function isCommand(action: Command | CodeAction): action is Command {
  return typeof (action as Command).command === 'string'
}
