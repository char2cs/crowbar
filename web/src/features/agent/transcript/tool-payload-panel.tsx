import { useEffect, useState } from 'react'
import { getToolPayload } from '@/features/agent/api/agent-api'

interface Side {
  label: string
  text: string | null
  loading: boolean
}

/**
 * A tool call's own request/result bytes, fetched on demand.
 *
 * They are content-addressed and never shipped with the activity feed itself
 * (see `AgentToolCall`'s own doc), so this is the only place either side is
 * ever read — opened by expanding a finished tool row (turn-tools.tsx). `null`
 * is retention having swept the payload, an ordinary outcome to say plainly
 * rather than an error.
 */
export function ToolPayloadPanel({
  wsId,
  chatId,
  toolId,
  hasRequest,
  hasResult,
}: {
  wsId: string
  chatId: string
  toolId: string
  hasRequest: boolean
  hasResult: boolean
}) {
  const [sides, setSides] = useState<Side[]>(() => {
    const initial: (Side | null)[] = [
      hasRequest ? { label: 'Request', text: null, loading: true } : null,
      hasResult ? { label: 'Result', text: null, loading: true } : null,
    ]
    return initial.filter((s): s is Side => s !== null)
  })

  useEffect(() => {
    const controller = new AbortController()
    // Gates every setSides call below, separately from `controller.signal`:
    // the signal cancels the underlying fetch, this flag is what stops an
    // already-in-flight read's RESOLUTION (success or failure) from writing
    // state once this effect's own cleanup has run — a different row
    // expanded, or unmount. Without it, a stale read's label could clobber a
    // fresher read the re-run effect already started for the same label.
    let ignore = false
    const read = async (side: 'request' | 'result', label: string) => {
      try {
        // The fetch itself must always run regardless of `ignore` — it's the
        // RESULT that's conditionally applied below, not the request skipped.
        // react-doctor-disable-next-line async-defer-await -- see comment above, the fetch's own request is unconditional
        const text = await getToolPayload(wsId, chatId, toolId, side, controller.signal)
        if (ignore) return
        setSides((current) =>
          current.map((s) => (s.label === label ? { ...s, text, loading: false } : s)),
        )
      } catch {
        // A genuine failure leaves the panel saying nothing rather than a
        // wrong thing — the row itself is still there to retry by reopening
        // it. An IGNORED (stale) read must not, per this effect's own
        // `ignore` doc above.
        if (ignore) return
        setSides((current) =>
          current.map((s) => (s.label === label ? { ...s, loading: false } : s)),
        )
      }
    }
    if (hasRequest) void read('request', 'Request')
    if (hasResult) void read('result', 'Result')
    return () => {
      ignore = true
      controller.abort()
    }
  }, [wsId, chatId, toolId, hasRequest, hasResult])

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

function prettyPayload(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    // Not JSON is not a failure to report: it is the provider's bytes, shown as
    // they arrived — the same rule ComposerChoice's own schema viewer follows.
    return text
  }
}
