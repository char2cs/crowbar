/**
 * Pure conversions between Crowbar editor positions/ranges and Monaco's
 * 1-based positions/ranges, plus diagnostic→marker mapping. Extracted from
 * `monaco-editor.tsx` so both the legacy standalone path and the retained
 * per-pane satellites hook share one implementation.
 */

// See the comment in `monaco-diff-editor.tsx`: `editor.api` is the same real
// singleton as the bare 'monaco-editor' specifier, without eagerly bundling
// all built-in language contributions.
import { MarkerSeverity, Range as MonacoRange } from 'monaco-editor/esm/vs/editor/editor.api.js'
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

export function toMonacoPosition(position: Position): Monaco.IPosition {
  return {
    lineNumber: position.line + 1,
    column: position.column + 1,
  }
}

export function clampMonacoPosition(
  model: Monaco.editor.ITextModel,
  position: Monaco.IPosition,
): Monaco.IPosition {
  const lineNumber = Math.max(1, Math.min(model.getLineCount(), position.lineNumber))
  const maxColumn = model.getLineMaxColumn(lineNumber)
  const column = Math.max(1, Math.min(maxColumn, position.column))
  return { lineNumber, column }
}

export function toClampedMonacoPosition(
  model: Monaco.editor.ITextModel,
  position: Position,
): Monaco.IPosition {
  return clampMonacoPosition(model, toMonacoPosition(position))
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

export function toMonacoRange(model: Monaco.editor.ITextModel, range: Range): Monaco.Range {
  let start = toClampedMonacoPosition(model, range.start)
  let end = toClampedMonacoPosition(model, range.end)
  if (
    start.lineNumber > end.lineNumber ||
    (start.lineNumber === end.lineNumber && start.column > end.column)
  ) {
    ;[start, end] = [end, start]
  }
  return new MonacoRange(start.lineNumber, start.column, end.lineNumber, end.column)
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
