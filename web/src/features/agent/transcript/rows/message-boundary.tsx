import type { ReactNode } from 'react'
import { ErrorBoundary } from '@/components/error-boundary'

/** One message's render failure stays on that message: its text, as written, and a note.
 *  It never keys its child, so a streaming reply is not remounted as it grows. */
export function MessageBoundary({ text, children }: { text: string; children: ReactNode }) {
  return (
    <ErrorBoundary
      resetKey={text}
      fallback={
        <div className="message-error" role="alert">
          <p className="message-error-text">{text}</p>
          <p className="message-error-note">Could not render this message</p>
        </div>
      }
    >
      {children}
    </ErrorBoundary>
  )
}
