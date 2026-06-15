/**
 * Pure encoding of the repo's Tree-sitter highlight tokens into Monaco's
 * semantic-tokens wire format. No DOM, no Monaco — unit-testable.
 */
import { isIgnoredCapture, mapCaptureToClass } from '@/features/editor/lib/wasm-parser/capture-map'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'

/** Legend token types — the categories CAPTURE_TO_CLASS collapses to, minus `text`. */
export const SEMANTIC_TOKEN_TYPES = [
  'keyword',
  'function',
  'variable',
  'property',
  'constant',
  'number',
  'string',
  'comment',
  'type',
  'attribute',
  'tag',
  'operator',
  'punctuation',
] as const
export type SemanticTokenType = (typeof SEMANTIC_TOKEN_TYPES)[number]

export const SEMANTIC_TOKEN_LEGEND = {
  tokenTypes: SEMANTIC_TOKEN_TYPES as unknown as string[],
  tokenModifiers: [] as string[],
}

const TYPE_INDEX: Record<string, number> = Object.fromEntries(
  SEMANTIC_TOKEN_TYPES.map((t, i) => [t, i]),
)

/** tree-sitter capture name -> legend index, or -1 to skip (ignored or `text`). */
export function captureToTypeIndex(capture: string): number {
  if (isIgnoredCapture(capture)) return -1
  const category = mapCaptureToClass(capture).replace(/^token-/, '')
  const idx = TYPE_INDEX[category]
  return idx === undefined ? -1 : idx
}

/**
 * Encode highlight tokens to Monaco's semantic-tokens Uint32Array:
 * 5 ints/token [deltaLine, deltaStartChar, length, typeIndex, modifierSet=0],
 * each relative to the previously emitted token. Monaco tokens are per-line, so a
 * token spanning rows is split into one entry per covered line.
 * `getLineLength(row)` = length (excl. EOL) of the 0-based row.
 */
export function encodeTokens(
  tokens: HighlightToken[],
  getLineLength: (row: number) => number,
): Uint32Array {
  const data: number[] = []
  let prevLine = 0
  let prevChar = 0

  const push = (line: number, char: number, length: number, typeIndex: number) => {
    if (length <= 0) return
    const deltaLine = line - prevLine
    const deltaChar = deltaLine === 0 ? char - prevChar : char
    data.push(deltaLine, deltaChar, length, typeIndex, 0)
    prevLine = line
    prevChar = char
  }

  for (const t of tokens) {
    const typeIndex = captureToTypeIndex(t.type)
    if (typeIndex < 0) continue

    const { row: startRow, column: startCol } = t.startPosition
    const { row: endRow, column: endCol } = t.endPosition

    if (startRow === endRow) {
      push(startRow, startCol, endCol - startCol, typeIndex)
      continue
    }

    push(startRow, startCol, getLineLength(startRow) - startCol, typeIndex)
    for (let row = startRow + 1; row < endRow; row++) {
      push(row, 0, getLineLength(row), typeIndex)
    }
    push(endRow, 0, endCol, typeIndex)
  }

  return new Uint32Array(data)
}
