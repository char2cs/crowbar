import type { ReactNode } from 'react'
import type { NoticeTone } from '@/features/agent/transcript/lib/marker-failure'

export interface FailureNoticeProps {
  tone: NoticeTone
  headline: string
  hint?: string
  actions?: ReactNode
  headlineTestId?: string
}

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
