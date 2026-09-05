import { useCallback, useRef } from 'react'

/**
 * Shell-style recall through this chat's own sent turns — ArrowUp/ArrowDown
 * step back and forward through what the person has actually typed here,
 * newest first, exactly like a terminal's own history.
 *
 * `texts` is whatever window of the user's own messages is currently loaded,
 * oldest first; recall never reaches past what has actually been paged in; a
 * chat with older history not yet loaded simply runs out of recall at the
 * oldest visible turn, the same way a terminal's history runs out at the
 * start of the session.
 */
export function usePromptHistory(texts: string[]) {
  // -1: not browsing — the box holds the person's own live draft. 0..texts
  // .length-1: browsing, counting back from the newest entry.
  const indexRef = useRef(-1)
  const stashRef = useRef('')

  /** A real edit (typing, not a recall) abandons wherever browsing had gotten
   *  to — the next ArrowUp starts a fresh walk from the newest turn again. */
  const reset = useCallback(() => {
    indexRef.current = -1
  }, [])

  const recallOlder = useCallback(
    (liveDraft: string): string | undefined => {
      if (texts.length === 0) return undefined
      if (indexRef.current === -1) {
        stashRef.current = liveDraft
        indexRef.current = 0
        return texts[texts.length - 1]
      }
      if (indexRef.current >= texts.length - 1) return undefined
      indexRef.current += 1
      return texts[texts.length - 1 - indexRef.current]
    },
    [texts],
  )

  const recallNewer = useCallback((): string | undefined => {
    if (indexRef.current === -1) return undefined
    if (indexRef.current === 0) {
      indexRef.current = -1
      return stashRef.current
    }
    indexRef.current -= 1
    return texts[texts.length - 1 - indexRef.current]
  }, [texts])

  return { recallOlder, recallNewer, reset }
}
