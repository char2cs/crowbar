import { useState, type ReactNode } from 'react'
import { ErrorBoundary } from '@/components/error-boundary'

function MessageError({ text, retry }: { text: string; retry: () => void }) {
  const [plain, setPlain] = useState(false)
  return (
    <div className="message-error" role="alert">
      <div className="tx">
        <b>This message could not be displayed.</b>{' '}
        <span className="s">The rest of the chat is not affected.</span>
      </div>
      {!plain && (
        <button type="button" className="message-error-action" onClick={() => setPlain(true)}>
          Show as plain text
        </button>
      )}
      <button type="button" className="message-error-action" onClick={retry}>
        Try again
      </button>
      {plain && <p className="message-error-text">{text}</p>}
    </div>
  )
}

/** One message's render failure stays on that message: a notice with two ways on, its text
 *  as written one click away. It never keys its child, so a streaming reply is not remounted. */
export function MessageBoundary({ text, children }: { text: string; children: ReactNode }) {
  return (
    <ErrorBoundary resetKey={text} fallback={(retry) => <MessageError text={text} retry={retry} />}>
      {children}
    </ErrorBoundary>
  )
}
