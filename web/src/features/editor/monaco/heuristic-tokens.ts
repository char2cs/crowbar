/**
 * Heuristic semantic tokens — a grammar-free, language-agnostic fallback.
 *
 * When no Tree-sitter grammar is available, Monaco's Monarch base only colors
 * keywords/strings/numbers, leaving every function and type name in the plain
 * foreground (flat-looking code). This adds `function` / `type` / `constant`
 * coloring for identifiers, for EVERY language, with no per-language config.
 *
 * It does NOT guess comment/string syntax. Instead it consumes Monaco's own
 * per-line tokenization (which already classifies comments/strings/keywords for
 * each language via that language's grammar) and only classifies identifiers
 * that Monaco left as plain code:
 *
 *   - `name(`              → function   (ident immediately before `(`)
 *   - `Name` (Capitalized) → type
 *   - `ALL_CAPS`           → constant
 *
 * Conservative by design (only these high-signal cases). Output reuses the
 * `HighlightToken` shape, so it flows through the same `encodeTokens` + theme
 * pipeline as the Tree-sitter path. (This is a heuristic — not how VSCode's
 * TextMate+LSP or Tree-sitter highlighting works; it's the no-grammar fallback.)
 */
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'

/** One Monaco line token: a start column and its (grammar-assigned) scope. */
export interface LineToken {
  offset: number
  type: string
}

const IDENT_START = /[A-Za-z_$]/
const IDENT_PART = /[A-Za-z0-9_$]/

/** Scopes whose text must never be re-colored (already handled by Monarch). */
function isSkippableScope(type: string): boolean {
  return /comment|string|keyword|number|regex|char|delimiter\.|operator/.test(type)
}

function classify(word: string, isCall: boolean): 'function' | 'type' | 'constant' | null {
  if (isCall) return 'function'
  if (!/[A-Za-z]/.test(word)) return null
  if (/^[A-Z][A-Z0-9_]*$/.test(word) && word.length > 1) return 'constant' // ALL_CAPS
  if (/^[A-Z]/.test(word)) return 'type'
  return null
}

export interface HeuristicScanOptions {
  /** Only emit tokens whose row is within [emitStartRow, emitEndRow] (0-based, inclusive). */
  emitStartRow?: number
  emitEndRow?: number
}

/**
 * Build heuristic semantic tokens from text + Monaco's per-line tokenization
 * (`monaco.editor.tokenize(text, languageId)` shape: one `LineToken[]` per line).
 * Pure and language-agnostic — Monaco's grammar tells us which spans are
 * comments/strings/keywords; we only classify identifiers in the remaining code.
 */
export function heuristicTokensFromLineTokens(
  text: string,
  lineTokens: LineToken[][],
  opts: HeuristicScanOptions = {},
): HighlightToken[] {
  const startRow = opts.emitStartRow ?? 0
  const endRow = opts.emitEndRow ?? Number.MAX_SAFE_INTEGER
  const out: HighlightToken[] = []
  const lines = text.split('\n')

  for (let row = 0; row < lines.length; row++) {
    if (row < startRow || row > endRow) continue
    const line = lines[row]
    const toks = lineTokens[row] ?? []

    // Mark columns covered by skippable scopes (comments/strings/keywords/…).
    const skip = new Array<boolean>(line.length).fill(false)
    for (let k = 0; k < toks.length; k++) {
      const begin = toks[k].offset
      const stop = k + 1 < toks.length ? toks[k + 1].offset : line.length
      if (isSkippableScope(toks[k].type)) {
        for (let c = begin; c < stop && c < line.length; c++) skip[c] = true
      }
    }

    // Scan identifiers in the code (non-skipped) regions of this line.
    let i = 0
    while (i < line.length) {
      if (!IDENT_START.test(line[i])) {
        i++
        continue
      }
      const start = i
      while (i < line.length && IDENT_PART.test(line[i])) i++
      if (skip[start]) continue

      const word = line.slice(start, i)
      let j = i
      while (j < line.length && (line[j] === ' ' || line[j] === '\t')) j++
      const isCall = line[j] === '('

      const type = classify(word, isCall)
      if (type) {
        out.push({
          type,
          startIndex: start,
          endIndex: i,
          startPosition: { row, column: start },
          endPosition: { row, column: i },
        })
      }
    }
  }

  return out
}
