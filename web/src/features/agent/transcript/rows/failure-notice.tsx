import type { ReactNode } from 'react'
import type { NoticeTone } from '@/features/agent/transcript/lib/marker-failure'

export interface FailureNoticeProps {
  tone: NoticeTone
  headline: string
  /** One short second line. */
  hint?: string
  /** Buttons, primary last; they sit at the right of the text. */
  actions?: ReactNode
  /** Marks the headline for tests and the dock's own lookups. */
  headlineTestId?: string
}

/** The one failure notice: at most two plain lines with its way on at the right. No
 *  icon, no code, no raw provider text. */
export function FailureNotice({
  tone,
  headline,
  hint,
  actions,
  headlineTestId,
}: FailureNoticeProps) {
  return (
    <section
      className="tray-section failure-notice"
      role="status"
      data-tone={tone}
      data-testid="agent-failure-notice"
    >
      <div className="failure-notice-text">
        <p className="failure-notice-headline" data-testid={headlineTestId}>
          {headline}
        </p>
        {hint && (
          <p className="failure-notice-hint" data-testid="agent-failed-hint">
            {hint}
          </p>
        )}
      </div>
      {actions && <div className="failure-notice-actions">{actions}</div>}
    </section>
  )
}
