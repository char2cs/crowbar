/**
 * Simulates a streaming LLM response word-by-word with configurable delays.
 *
 * Returns a `cancel` function that immediately stops all pending timeouts.
 * Safe to call multiple times (idempotent). This is mock infrastructure —
 * replace with a real SSE reader when the backend is ready.
 */
export function simulateStream(
  text: string,
  onChunk: (chunk: string) => void,
  onDone: () => void,
): () => void {
  const words = text.split(' ')
  let i = 0
  let cancelled = false
  let timerId: ReturnType<typeof setTimeout>

  const tick = () => {
    if (cancelled) return
    if (i >= words.length) {
      onDone()
      return
    }
    onChunk((i === 0 ? '' : ' ') + words[i])
    i++
    timerId = setTimeout(tick, 40)
  }

  timerId = setTimeout(tick, 400) // initial delay before first token

  return () => {
    cancelled = true
    clearTimeout(timerId)
  }
}
