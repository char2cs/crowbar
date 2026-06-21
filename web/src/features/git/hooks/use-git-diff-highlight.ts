import { useEffect, useMemo, useState } from 'react'
import type { HighlightToken } from '@/features/editor/lib/wasm-parser/types'
import {
  getLanguageId,
  mapTokensToDiffLines,
  reconstructContent,
  resolveParserConfig,
} from '../lib/diff-highlight-shared'
import type { GitDiffLine } from '../types/git-types'
import { tokenizeByLine } from '@/features/editor/lib/wasm-parser/tokenizer'

export function useDiffHighlighting(
  lines: GitDiffLine[],
  filePath: string,
): Map<number, HighlightToken[]> {
  const [tokenMap, setTokenMap] = useState<Map<number, HighlightToken[]>>(new Map())

  const languageId = useMemo(() => getLanguageId(filePath), [filePath])

  const { oldContent, newContent } = useMemo(() => {
    const old = reconstructContent(lines, 'old')
    const newC = reconstructContent(lines, 'new')
    return { oldContent: old, newContent: newC }
  }, [lines])

  useEffect(() => {
    if (!languageId) {
      setTokenMap(new Map())
      return
    }

    const lang = languageId
    let cancelled = false

    async function tokenize() {
      try {
        const config = await resolveParserConfig(lang)

        const [oldTokensByLine, newTokensByLine] = await Promise.all([
          oldContent.content
            ? tokenizeByLine(oldContent.content, lang, config)
            : Promise.resolve(new Map<number, HighlightToken[]>()),
          newContent.content
            ? tokenizeByLine(newContent.content, lang, config)
            : Promise.resolve(new Map<number, HighlightToken[]>()),
        ])

        if (cancelled) return

        const oldTokenMap = mapTokensToDiffLines(oldTokensByLine, oldContent.lineMapping)
        const newTokenMap = mapTokensToDiffLines(newTokensByLine, newContent.lineMapping)

        const merged = new Map<number, HighlightToken[]>()

        for (const [index, tokens] of oldTokenMap) {
          merged.set(index, tokens)
        }
        for (const [index, tokens] of newTokenMap) {
          merged.set(index, tokens)
        }

        setTokenMap(merged)
      } catch {
        setTokenMap(new Map())
      }
    }

    tokenize()

    return () => {
      cancelled = true
    }
  }, [languageId, oldContent, newContent])

  return tokenMap
}
