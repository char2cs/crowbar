import { useEffect, useState } from 'react'
import { readBlob } from '@/features/agent/api/agent-chat-reads'

interface Side {
  label: string
  ref: string
  text: string | null
  loading: boolean
}

function prettyPayload(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text // the provider's bytes, as they arrived
  }
}

function sidesOf(inputRef?: string, resultRef?: string): Side[] {
  const sides: Side[] = []
  if (inputRef) sides.push({ label: 'Request', ref: inputRef, text: null, loading: true })
  if (resultRef) sides.push({ label: 'Result', ref: resultRef, text: null, loading: true })
  return sides
}

/** A tool call's request and result, read by blob ref only once opened. A
 *  swept blob reads null: said plainly, not as an error. */
export function ToolPayload({
  chatId,
  inputRef,
  resultRef,
}: {
  chatId: string
  inputRef?: string
  resultRef?: string
}) {
  const [sides, setSides] = useState(() => sidesOf(inputRef, resultRef))

  useEffect(() => {
    const controller = new AbortController()
    let ignore = false
    const settle = (ref: string, text: string | null) => {
      if (ignore) return
      setSides((current) =>
        current.map((s) => (s.ref === ref ? { ...s, text, loading: false } : s)),
      )
    }
    for (const ref of [inputRef, resultRef]) {
      if (!ref) continue
      readBlob(chatId, ref, controller.signal).then(
        (text) => settle(ref, text),
        () => settle(ref, null),
      )
    }
    return () => {
      ignore = true
      controller.abort()
    }
  }, [chatId, inputRef, resultRef])

  return (
    <div className="payload" data-testid="agent-tool-payload">
      {sides.map((side) => (
        <div key={side.label} className="payload-side">
          <p className="payload-label">{side.label}</p>
          {side.loading ? (
            <p className="payload-status">Loading…</p>
          ) : side.text === null ? (
            <p className="payload-status">No longer available</p>
          ) : (
            <pre>
              <code>{prettyPayload(side.text)}</code>
            </pre>
          )}
        </div>
      ))}
    </div>
  )
}
