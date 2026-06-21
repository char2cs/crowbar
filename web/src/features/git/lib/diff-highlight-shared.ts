import { indexedDBParserCache } from '@/features/editor/lib/wasm-parser/cache-indexeddb'
import {
  fetchHighlightQuery,
  getDefaultParserWasmUrl,
} from '@/features/editor/lib/wasm-parser/extension-assets'
import { tokenizeByLine } from '@/features/editor/lib/wasm-parser/tokenizer'
import type { HighlightToken, ParserConfig } from '@/features/editor/lib/wasm-parser/types'
import { getLanguageIdFromPath } from '@/features/editor/utils/language-id'
import type { GitDiffLine } from '../types/git-types'

export interface ReconstructedContent {
  content: string
  lineMapping: Map<number, number>
}

export function getLanguageId(filePath: string): string | null {
  return getLanguageIdFromPath(filePath)
}

export function reconstructContent(
  lines: GitDiffLine[],
  version: 'old' | 'new',
): ReconstructedContent {
  const contentLines: string[] = []
  const lineMapping = new Map<number, number>()

  lines.forEach((line, diffIndex) => {
    if (line.line_type === 'header') return

    const includeInOld = line.line_type === 'context' || line.line_type === 'removed'
    const includeInNew = line.line_type === 'context' || line.line_type === 'added'

    if ((version === 'old' && includeInOld) || (version === 'new' && includeInNew)) {
      lineMapping.set(contentLines.length, diffIndex)
      contentLines.push(line.content)
    }
  })

  return {
    content: contentLines.join('\n'),
    lineMapping,
  }
}

export function mapTokensToDiffLines(
  tokensByLine: Map<number, HighlightToken[]>,
  lineMapping: Map<number, number>,
): Map<number, HighlightToken[]> {
  const result = new Map<number, HighlightToken[]>()

  for (const [reconstructedLine, tokens] of tokensByLine) {
    const diffIndex = lineMapping.get(reconstructedLine)
    if (diffIndex !== undefined) {
      const adjustedTokens = tokens.map((token) => ({
        ...token,
        startPosition: {
          row: 0,
          column: token.startPosition.column,
        },
        endPosition: {
          row: token.endPosition.row - token.startPosition.row,
          column: token.endPosition.column,
        },
      }))
      result.set(diffIndex, adjustedTokens)
    }
  }

  return result
}

export async function resolveParserConfig(languageId: string): Promise<ParserConfig> {
  const cached = await indexedDBParserCache.get(languageId)

  let wasmPath = getDefaultParserWasmUrl(languageId)
  let highlightQuery: string | undefined

  if (cached) {
    wasmPath = cached.sourceUrl || wasmPath
    highlightQuery = cached.highlightQuery
  }

  if (!highlightQuery || highlightQuery.trim().length === 0) {
    try {
      const { query } = await fetchHighlightQuery(languageId, {
        wasmUrl: wasmPath,
        cacheMode: 'no-store',
      })
      highlightQuery = query || highlightQuery
    } catch {
      // Ignore fetch errors — highlight query is optional
    }
  }

  return { languageId, wasmPath, highlightQuery }
}

/**
 * Tokenize both sides of a diff by reconstructed content.
 * Returns a merged map of diffIndex → HighlightToken[] (old tokens take precedence for context).
 */
export async function tokenizeDiffLines(
  lines: GitDiffLine[],
  languageId: string,
): Promise<Map<number, HighlightToken[]>> {
  const oldContent = reconstructContent(lines, 'old')
  const newContent = reconstructContent(lines, 'new')

  const config = await resolveParserConfig(languageId)

  const [oldTokensByLine, newTokensByLine] = await Promise.all([
    oldContent.content
      ? tokenizeByLine(oldContent.content, languageId, config)
      : Promise.resolve(new Map<number, HighlightToken[]>()),
    newContent.content
      ? tokenizeByLine(newContent.content, languageId, config)
      : Promise.resolve(new Map<number, HighlightToken[]>()),
  ])

  const oldTokenMap = mapTokensToDiffLines(oldTokensByLine, oldContent.lineMapping)
  const newTokenMap = mapTokensToDiffLines(newTokensByLine, newContent.lineMapping)

  const merged = new Map<number, HighlightToken[]>()
  for (const [index, tokens] of oldTokenMap) {
    merged.set(index, tokens)
  }
  for (const [index, tokens] of newTokenMap) {
    merged.set(index, tokens)
  }

  return merged
}
