import { buildSearchRegex, type SearchOptions } from '@/features/editor/utils/search'

/** A single search hit inside the multi-file diff. */
export interface DiffSearchMatch {
  /** The file's stable key (= the section/cache key in the stack). */
  fileKey: string
  /** Index of the file within multiDiff.files (for virtualizer.scrollToIndex). */
  fileIndex: number
  /** 1-based model line in the file's UNIFIED editor (matches its decorations). */
  lineNumber: number
  /** 1-based columns of the hit within the line. */
  startColumn: number
  endColumn: number
}

export interface DiffSearchResult {
  matches: DiffSearchMatch[]
  /** True when the match cap was hit and results were truncated. */
  limited: boolean
}

/** Cap matches so a huge commit (tens of thousands of changed lines) stays responsive. */
export const MAX_DIFF_MATCHES = 2000

/**
 * Search the UNIFIED content of every file for `query`, returning matches with
 * their file + unified model-line + column positions. `contents[i]` must be the
 * unified reconstruction of `files[i]` (so line indices map 1:1 to model lines),
 * and `keyForIndex(i)` the stable file key. Capped at MAX_DIFF_MATCHES.
 */
export function computeDiffMatches(
  contents: string[],
  keyForIndex: (index: number) => string,
  query: string,
  options: SearchOptions,
): DiffSearchResult {
  const regex = buildSearchRegex(query, options)
  if (!regex) return { matches: [], limited: false }

  const matches: DiffSearchMatch[] = []

  for (let fileIndex = 0; fileIndex < contents.length; fileIndex++) {
    const content = contents[fileIndex]
    if (!content) continue
    const fileKey = keyForIndex(fileIndex)
    const lines = content.split('\n')
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i]
      regex.lastIndex = 0
      let m: RegExpExecArray | null = regex.exec(line)
      while (m !== null) {
        matches.push({
          fileKey,
          fileIndex,
          lineNumber: i + 1,
          startColumn: m.index + 1,
          endColumn: m.index + m[0].length + 1,
        })
        if (matches.length >= MAX_DIFF_MATCHES) {
          return { matches, limited: true }
        }
        // Guard against zero-width matches looping forever.
        if (m.index === regex.lastIndex) regex.lastIndex++
        m = regex.exec(line)
      }
    }
  }

  return { matches, limited: false }
}

export type { SearchOptions }
