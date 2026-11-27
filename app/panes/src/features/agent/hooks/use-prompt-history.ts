import { useCallback, useRef } from 'react'

export function usePromptHistory(read: () => string[]) {
  const indexRef = useRef(-1)
  const stashRef = useRef('')
  const queryRef = useRef<string | null>(null)

  const reset = useCallback(() => {
    indexRef.current = -1
    queryRef.current = null
  }, [])

  const recallOlder = useCallback(
    (liveDraft: string): string | undefined => {
      const texts = read()
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
    [read],
  )

  const recallNewer = useCallback((): string | undefined => {
    if (indexRef.current === -1) return undefined
    if (indexRef.current === 0) {
      indexRef.current = -1
      return stashRef.current
    }
    indexRef.current -= 1
    const texts = read()
    return texts[texts.length - 1 - indexRef.current]
  }, [read])

  const searchOlder = useCallback(
    (typed: string): string | undefined => {
      const query = (queryRef.current ?? typed).trim().toLowerCase()
      const texts = read()
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
    [read],
  )

  return { recallOlder, recallNewer, searchOlder, reset }
}
