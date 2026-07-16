import { useCallback, useEffect, useMemo, useState } from 'react'
import type { GitDiff } from '../../types/git-types'
import { serializeGitDiffSourceForEditor } from '../../utils/diff-editor-content'
import { shouldUseScrollableDiffEditor } from '../../utils/diff-viewer-scale'
import { computeDiffMatches, type DiffSearchMatch } from '../../utils/diff-search'

const DEBOUNCE_MS = 150

export interface DiffSearchState {
  query: string
  setQuery: (query: string) => void
  caseSensitive: boolean
  toggleCaseSensitive: () => void
  matches: DiffSearchMatch[]
  matchesByFile: Map<string, DiffSearchMatch[]>
  total: number
  limited: boolean
  /** 0-based index into `matches`, or -1 when there are none. */
  currentIndex: number
  current: DiffSearchMatch | null
  /** Bumps whenever the active match should be revealed (navigation / new results). */
  revealNonce: number
  next: () => void
  prev: () => void
}

/**
 * Cross-file search over a multi-file diff. Serializes every file's unified
 * content once (lazily, only while `enabled`), debounces the query, and tracks
 * the active match for navigation. Returns matches grouped by file for the
 * editors to highlight.
 */
export function useDiffSearch(params: {
  files: GitDiff[]
  keyForIndex: (index: number) => string
  enabled: boolean
}): DiffSearchState {
  const { files, keyForIndex, enabled } = params
  const [query, setQuery] = useState('')
  const [caseSensitive, setCaseSensitive] = useState(false)
  const [debouncedQuery, setDebouncedQuery] = useState('')
  const [currentIndex, setCurrentIndex] = useState(-1)
  const [revealNonce, setRevealNonce] = useState(0)

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(query), DEBOUNCE_MS)
    return () => clearTimeout(timer)
  }, [query])

  // Only do the (potentially large) serialization when there is an active query.
  // `shouldSearch` is a boolean, so it stays true across keystrokes — files are
  // serialized once when search begins, not re-serialized per keystroke, and an
  // open-but-idle bar (or a working-tree refresh with no query) costs nothing.
  const shouldSearch = enabled && debouncedQuery.trim().length > 0
  const contents = useMemo(() => {
    if (!shouldSearch) return []
    // Large / raw-patch files render in a separate scrollable editor that has no
    // search highlight or line reveal, so exclude them from results entirely
    // (counting them would inflate the total with un-highlightable, un-revealable hits).
    return files.map((file) =>
      shouldUseScrollableDiffEditor(file) ? '' : serializeGitDiffSourceForEditor(file).content,
    )
  }, [files, shouldSearch])

  const { matches, limited } = useMemo(() => {
    if (!shouldSearch) return { matches: [] as DiffSearchMatch[], limited: false }
    return computeDiffMatches(contents, keyForIndex, debouncedQuery, {
      caseSensitive,
      wholeWord: false,
      useRegex: false,
    })
  }, [shouldSearch, contents, debouncedQuery, caseSensitive, keyForIndex])

  // New results → select the first match and reveal it. Adjust-during-render
  // (React docs "adjusting state when a prop changes"): `matches` derives
  // synchronously above, and resetting via an effect painted one frame where
  // the OLD currentIndex indexed into the NEW matches array (a transiently
  // wrong `current`). Keying the reset on the matches identity during render
  // removes that frame while keeping the exact same transitions.
  const [prevMatches, setPrevMatches] = useState(matches)
  if (prevMatches !== matches) {
    setPrevMatches(matches)
    setCurrentIndex(matches.length > 0 ? 0 : -1)
    if (matches.length > 0) setRevealNonce((nonce) => nonce + 1)
  }

  const matchesByFile = useMemo(() => {
    const map = new Map<string, DiffSearchMatch[]>()
    for (const match of matches) {
      const existing = map.get(match.fileKey)
      if (existing) existing.push(match)
      else map.set(match.fileKey, [match])
    }
    return map
  }, [matches])

  const next = useCallback(() => {
    if (matches.length === 0) return
    setCurrentIndex((index) => (index + 1) % matches.length)
    setRevealNonce((nonce) => nonce + 1)
  }, [matches.length])

  const prev = useCallback(() => {
    if (matches.length === 0) return
    setCurrentIndex((index) => (index - 1 + matches.length) % matches.length)
    setRevealNonce((nonce) => nonce + 1)
  }, [matches.length])

  const toggleCaseSensitive = useCallback(() => setCaseSensitive((value) => !value), [])

  const current = currentIndex >= 0 ? (matches[currentIndex] ?? null) : null

  return {
    query,
    setQuery,
    caseSensitive,
    toggleCaseSensitive,
    matches,
    matchesByFile,
    total: matches.length,
    limited,
    currentIndex,
    current,
    revealNonce,
    next,
    prev,
  }
}
