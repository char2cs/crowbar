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
