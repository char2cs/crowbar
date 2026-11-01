import { useCallback, useRef } from 'react'

/** `texts` is the loaded window of the user's messages, oldest first; recall never
 *  reaches past what has been paged in. */
export function usePromptHistory(texts: string[]) {
  // -1: not browsing (the box holds the live draft); otherwise steps back from newest.
  const indexRef = useRef(-1)
  const stashRef = useRef('')

  /** A real edit restarts the next walk from the newest turn. */
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
