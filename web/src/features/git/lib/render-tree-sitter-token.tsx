import type { ReactNode } from 'react'
import type { TokenNode } from 'react-diff-view'
import type { HunkTokens } from 'react-diff-view'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'
import type { GitDiff, GitDiffLine } from '../types/git-types'
import {
  getLanguageId,
  reconstructContent,
  resolveParserConfig,
} from './diff-highlight-shared'
import { tokenizeByLine } from '@/features/editor/lib/wasm-parser/tokenizer'

// ──────────────────────────────────────────────
// Column-slice → TokenNode[] conversion
// ──────────────────────────────────────────────

/**
 * Convert a flat array of HighlightTokens (for a single line) into react-diff-view
 * TokenNode[]. Each HighlightToken becomes a node with `className: token.type` (e.g.
 * "token-keyword") and gaps between tokens become plain text nodes.
 *
 * When `tokens` is empty the function returns `[{ type: 'text', value: content }]` so
 * the line renders as plain text — graceful degradation when tree-sitter is unavailable.
 */
export function highlightTokensToTokenNodes(
  content: string,
  tokens: HighlightToken[],
): TokenNode[] {
  if (!tokens || tokens.length === 0) {
    return [{ type: 'text', value: content }]
  }

  const result: TokenNode[] = []
  let lastEnd = 0

  for (const token of tokens) {
    const start = token.startPosition.column
    const end = token.endPosition.column

    if (start > lastEnd) {
      const plain = content.slice(lastEnd, start)
      if (plain) {
        result.push({ type: 'text', value: plain })
      }
    }

    const value = content.slice(start, end)
    if (value) {
      // Use a custom type so renderTreeSitterToken can handle it;
      // also attach className so the default renderer works as a fallback.
      result.push({
        type: 'syntax',
        className: token.type,
        value,
      })
    }

    lastEnd = end
  }

  if (lastEnd < content.length) {
    const tail = content.slice(lastEnd)
    if (tail) {
      result.push({ type: 'text', value: tail })
    }
  }

  return result
}

// ──────────────────────────────────────────────
// renderToken implementation
// ──────────────────────────────────────────────

type DefaultRenderToken = (token: TokenNode, index: number) => ReactNode

/**
 * renderToken prop for react-diff-view's <Diff>.
 *
 * For our tree-sitter "syntax" nodes we render `<span className={token.className}>`.
 * For everything else (react-diff-view's own mark/edit/text nodes) we delegate to
 * `renderDefault` so the diff's built-in decorations (edit marks, selections) still work.
 */
export function renderTreeSitterToken(
  token: TokenNode,
  renderDefault: DefaultRenderToken,
  index: number,
): ReactNode {
  if (token.type === 'syntax' && token.className) {
    return (
      <span key={index} className={token.className as string}>
        {token.value as string}
      </span>
    )
  }
  return renderDefault(token, index)
}

// ──────────────────────────────────────────────
// buildDiffTokens — produces HunkTokens for <Diff tokens={...} />
// ──────────────────────────────────────────────

/**
 * Build react-diff-view HunkTokens from a GitDiff using the existing tree-sitter pipeline.
 *
 * HunkTokens shape:
 *   { old: TokenNode[][], new: TokenNode[][] }
 * where `old[lineNumber - 1]` and `new[lineNumber - 1]` hold the token arrays for
 * that 1-indexed line in each side of the diff.
 *
 * Graceful degradation: if tree-sitter is unavailable (wasm not loaded, language
 * unsupported, etc.) this returns `null` instead of throwing, so the caller can pass
 * `tokens={null}` to <Diff> and it falls back to plain-text rendering.
 */
export async function buildDiffTokens(diff: GitDiff): Promise<HunkTokens | null> {
  const languageId = getLanguageId(diff.file_path)
  if (!languageId) return null

  try {
    const config = await resolveParserConfig(languageId)

    const oldReconstructed = reconstructContent(diff.lines, 'old')
    const newReconstructed = reconstructContent(diff.lines, 'new')

    const [oldTokensByLine, newTokensByLine] = await Promise.all([
      oldReconstructed.content
        ? tokenizeByLine(oldReconstructed.content, languageId, config)
        : Promise.resolve(new Map<number, HighlightToken[]>()),
      newReconstructed.content
        ? tokenizeByLine(newReconstructed.content, languageId, config)
        : Promise.resolve(new Map<number, HighlightToken[]>()),
    ])

    // Build per-side arrays keyed by (1-indexed) file line number.
    // We need to map from reconstructed-content row → actual file line number.
    // The GitDiffLine carries old_line_number / new_line_number — we use those.

    // Build reverse map: reconstructedRow → file line number for each side
    const oldLineNumbers = buildRowToLineNumber(diff.lines, 'old')
    const newLineNumbers = buildRowToLineNumber(diff.lines, 'new')

    const oldTokens = buildSideArray(oldTokensByLine, oldLineNumbers, oldReconstructed.content)
    const newTokens = buildSideArray(newTokensByLine, newLineNumbers, newReconstructed.content)

    return { old: oldTokens, new: newTokens }
  } catch {
    return null
  }
}

/**
 * For a given side, returns a Map<reconstructedRow, fileLineNumber>.
 */
function buildRowToLineNumber(
  lines: GitDiffLine[],
  side: 'old' | 'new',
): Map<number, number> {
  const map = new Map<number, number>()
  let row = 0

  for (const line of lines) {
    if (line.line_type === 'header') continue

    const includeInOld = line.line_type === 'context' || line.line_type === 'removed'
    const includeInNew = line.line_type === 'context' || line.line_type === 'added'

    if (side === 'old' && includeInOld) {
      const fileLineNumber = line.old_line_number
      if (fileLineNumber !== undefined) {
        map.set(row, fileLineNumber)
      }
      row++
    } else if (side === 'new' && includeInNew) {
      const fileLineNumber = line.new_line_number
      if (fileLineNumber !== undefined) {
        map.set(row, fileLineNumber)
      }
      row++
    }
  }

  return map
}

/**
 * Build the per-side TokenNode[][] where index = lineNumber - 1.
 * Reconstructed content lines map to file line numbers via `rowToLineNumber`.
 */
function buildSideArray(
  tokensByRow: Map<number, HighlightToken[]>,
  rowToLineNumber: Map<number, number>,
  content: string,
): TokenNode[][] {
  if (!content) return []

  const contentLines = content.split('\n')
  // Find the maximum file line number to size the array
  let maxLineNumber = 0
  for (const lineNumber of rowToLineNumber.values()) {
    if (lineNumber > maxLineNumber) maxLineNumber = lineNumber
  }

  if (maxLineNumber === 0) return []

  const result: TokenNode[][] = new Array(maxLineNumber).fill(null).map(() => [])

  for (const [row, lineNumber] of rowToLineNumber) {
    const lineContent = contentLines[row] ?? ''
    const tokens = tokensByRow.get(row) ?? []
    result[lineNumber - 1] = highlightTokensToTokenNodes(lineContent, tokens)
  }

  return result
}
