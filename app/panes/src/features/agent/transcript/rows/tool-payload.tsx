import { useEffect, useState } from 'react'
import { readBlob } from '@/features/agent/api/agent-chat-reads'
import { payloadText } from '@/features/agent/transcript/lib/payload-text'
import { useChatHost } from '@/lib/host-resolve'

type Read = { state: 'loading' } | { state: 'read'; text: string | null } | { state: 'failed' }

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
  const hostId = useChatHost(chatId)
  const key = `${hostId}#${blobRef}#${attempt}`

  useEffect(() => {
    if (!hostId) return
    const controller = new AbortController()
    readBlob(hostId, chatId, blobRef, controller.signal).then(
      (text) => setRead({ state: 'read', text, key }),
      () => {
        if (!controller.signal.aborted) setRead({ state: 'failed', key })
      },
    )
    return () => controller.abort()
  }, [hostId, chatId, blobRef, key])

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
          <code>{payloadText(current.text)}</code>
        </pre>
      )}
    </div>
  )
}

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
