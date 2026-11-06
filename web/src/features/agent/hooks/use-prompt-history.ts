import { useCallback, useRef } from 'react'

/** `texts` is the loaded window of the user's messages, oldest first; recall never
 *  reaches past what has been paged in. */
export function usePromptHistory(texts: string[]) {
  // -1: not browsing (the box holds the live draft); otherwise steps back from newest.
  const indexRef = useRef(-1)
  const stashRef = useRef('')
  // The words a ctrl+r search looks for; the box shows each match, not them.
  const queryRef = useRef<string | null>(null)

  /** A real edit restarts the next walk from the newest turn. */
  const reset = useCallback(() => {
    indexRef.current = -1
    queryRef.current = null
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

  /** The next older prompt containing `typed` (the words of the search under way). */
  const searchOlder = useCallback(
    (typed: string): string | undefined => {
      const query = (queryRef.current ?? typed).trim().toLowerCase()
      for (let step = indexRef.current + 1; step < texts.length; step++) {
        const text = texts[texts.length - 1 - step]!
        if (!text.toLowerCase().includes(query)) continue
        if (indexRef.current === -1) stashRef.current = typed
        queryRef.current = query
        indexRef.current = step
        return text
      }
      return undefined
    },
    [texts],
  )

  return { recallOlder, recallNewer, searchOlder, reset }
}
