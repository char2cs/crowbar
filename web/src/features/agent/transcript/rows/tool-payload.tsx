import { useEffect, useState } from 'react'
import { readBlob } from '@/features/agent/api/agent-chat-reads'

type Read = { state: 'loading' } | { state: 'read'; text: string | null } | { state: 'failed' }

function prettyPayload(text: string): string {
  try {
    return JSON.stringify(JSON.parse(text), null, 2)
  } catch {
    return text // the provider's bytes, as they arrived
  }
}

/** One side, read when it mounts. A swept blob reads null and is said
 *  plainly; any other failure is transient and offers a retry. */
function PayloadSide({
  chatId,
  label,
  blobRef,
}: {
  chatId: string
  label: string
  blobRef: string
}) {
  const [attempt, setAttempt] = useState(0)
  const [read, setRead] = useState<Read & { key: string }>({ state: 'loading', key: '' })
  const key = `${blobRef}#${attempt}`

  useEffect(() => {
    const controller = new AbortController()
    readBlob(chatId, blobRef, controller.signal).then(
      (text) => setRead({ state: 'read', text, key }),
      () => {
        if (!controller.signal.aborted) setRead({ state: 'failed', key })
      },
    )
    return () => controller.abort()
  }, [chatId, blobRef, key])

  const current: Read = read.key === key ? read : { state: 'loading' }
  return (
    <div className="payload-side">
      <p className="payload-label">{label}</p>
      {current.state === 'loading' ? (
        <p className="payload-status">Loading…</p>
      ) : current.state === 'failed' ? (
        <p className="payload-status">
          Couldn’t load this.{' '}
          <button type="button" className="payload-retry" onClick={() => setAttempt((n) => n + 1)}>
            Try again
          </button>
        </p>
      ) : current.text === null ? (
        <p className="payload-status">No longer available</p>
      ) : (
        <pre>
          <code>{prettyPayload(current.text)}</code>
        </pre>
      )}
    </div>
  )
}

/** A tool call's request and result, each read by blob ref once opened; a
 *  ref that lands while the panel is open adds its side. */
export function ToolPayload({
  chatId,
  inputRef,
  resultRef,
}: {
  chatId: string
  inputRef?: string
  resultRef?: string
}) {
  return (
    <div className="payload" data-testid="agent-tool-payload">
      {inputRef && (
        <PayloadSide key={inputRef} chatId={chatId} label="Request" blobRef={inputRef} />
      )}
      {resultRef && (
        <PayloadSide key={resultRef} chatId={chatId} label="Result" blobRef={resultRef} />
      )}
    </div>
  )
}
