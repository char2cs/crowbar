import { useState, type ReactNode } from 'react'
import type { FailureKind } from '@/features/agent/transcript/lib/turn-failure'

export interface FailureNoticeProps {
  /** `warning` is a provider's heads-up: the same notice, without a failed turn. */
  kind: FailureKind | 'warning'
  headline: string
  /** One short second line. */
  hint?: ReactNode
  /** The provider's raw words, reachable only behind a compact Details toggle. */
  detail?: string
  /** Buttons, primary last; they sit at the right of the text. */
  actions?: ReactNode
  /** Marks the headline for tests and the transcript's own lookups. */
  headlineTestId?: string
}

/** A failure or warning in at most two short lines, with its way on at the right. */
export function FailureNotice({
  kind,
  headline,
  hint,
  detail,
  actions,
  headlineTestId,
}: FailureNoticeProps) {
  const [open, setOpen] = useState(false)
  return (
    <div
      className="failure-notice"
      role="status"
      data-kind={kind}
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
        {detail && (
          <button
            type="button"
            className="failure-notice-details"
            aria-expanded={open}
            onClick={() => setOpen((o) => !o)}
          >
            Details
          </button>
        )}
      </div>
      {actions && <div className="failure-notice-actions">{actions}</div>}
      {open && detail && <pre className="failure-notice-raw">{detail}</pre>}
    </div>
  )
}
