/**
 * Pure conversions between Crowbar editor positions/ranges and Monaco's
 * 1-based positions/ranges, plus diagnostic→marker mapping.
 */

// See the comment in `monaco-diff-editor.tsx`: `editor.api` is the same real
// singleton as the bare 'monaco-editor' specifier, without eagerly bundling
// all built-in language contributions.
import { MarkerSeverity } from 'monaco-editor/esm/vs/editor/editor.api.js'
import type * as Monaco from 'monaco-editor'
import type { Position, Range } from '../types/editor'
import type { LspDiagnostic } from '../lsp/lsp-client'

export function toEditorPosition(
  model: Monaco.editor.ITextModel,
  position: Monaco.IPosition,
): Position {
  return {
    line: position.lineNumber - 1,
    column: position.column - 1,
    offset: model.getOffsetAt(position),
  }
}

export function toEditorRange(
  model: Monaco.editor.ITextModel,
  selection: Monaco.Selection,
): Range | undefined {
  if (selection.isEmpty()) return undefined
  const start = selection.getStartPosition()
  const end = selection.getEndPosition()
  return {
    start: toEditorPosition(model, start),
    end: toEditorPosition(model, end),
  }
}

function severityToMonaco(severity: string): Monaco.MarkerSeverity {
  switch (severity.toLowerCase()) {
    case 'error':
      return MarkerSeverity.Error
    case 'warning':
      return MarkerSeverity.Warning
    case 'hint':
      return MarkerSeverity.Hint
    default:
      return MarkerSeverity.Info
  }
}

// Backend diagnostics use 0-based line/character; Monaco markers are 1-based.
export function toMonacoMarker(diagnostic: LspDiagnostic): Monaco.editor.IMarkerData {
  return {
    severity: severityToMonaco(diagnostic.severity),
    message: diagnostic.message,
    source: diagnostic.source,
    code: diagnostic.code,
    startLineNumber: diagnostic.range.start.line + 1,
    startColumn: diagnostic.range.start.character + 1,
    endLineNumber: diagnostic.range.end.line + 1,
    endColumn: diagnostic.range.end.character + 1,
  }
}

// Diagnostic paths and buffer paths are both workspace-relative, but tolerate a
// leading-slash or absolute mismatch by comparing suffixes.
export function pathsMatch(a: string, b: string): boolean {
  if (a === b) return true
  return a.endsWith(b) || b.endsWith(a)
}
