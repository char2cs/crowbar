import type { ActivityComponent } from '@/features/agent/lib/activity-components'
import { occurrenceKeys } from '@/features/agent/lib/occurrence-keys'
import { TurnDiffPreview } from '@/features/agent/activity/turn-diff-preview'
import {
  AlertIcon,
  FileIcon,
  GaugeIcon,
  LinkIcon,
  ListIcon,
  OfflineIcon,
  RefreshIcon,
} from '@/features/agent/shared/agent-icons'

interface PlanStep {
  text: string
  status: string
}

function textField(payload: Record<string, unknown>, ...keys: string[]): string | undefined {
  for (const key of keys) {
    const value = payload[key]
    if (typeof value === 'string' && value.trim()) return value
  }
  return undefined
}

function planSteps(payload: Record<string, unknown>): PlanStep[] {
  if (!Array.isArray(payload.steps)) return []
  return payload.steps.flatMap((value) => {
    if (!value || typeof value !== 'object') return []
    const step = value as Record<string, unknown>
    if (typeof step.text !== 'string' || !step.text.trim()) return []
    return [{ text: step.text, status: typeof step.status === 'string' ? step.status : 'pending' }]
  })
}

function humanKind(kind: ActivityComponent['kind']): string {
  return kind.replaceAll('_', ' ')
}

function safeExternalURL(value: string | undefined): string | undefined {
  if (!value) return undefined
  try {
    const url = new URL(value)
    return url.protocol === 'https:' || url.protocol === 'http:' ? url.href : undefined
  } catch {
    return undefined
  }
}

function Status({ value }: { value: ActivityComponent['status'] }) {
  return (
    <span className="activity-component-status" data-status={value}>
      {value}
    </span>
  )
}

/** Provider-neutral rich rows for canonical activity that is not a message,
 * tool, choice or subagent. Every branch depends only on the canonical kind. */
export function ActivityComponentRow({
  component,
  wsId,
}: {
  component: ActivityComponent
  wsId?: string
}) {
  const payload = component.payload
  const common = {
    className: 'row activity-component-row',
    'data-testid': 'agent-activity-component',
    'data-component-id': component.id,
    'data-component-kind': component.kind,
    'data-status': component.status,
    'data-parent-id': component.parentId,
    'data-nested': component.parentId && component.parentId !== component.turnId ? '' : undefined,
  }

  if (component.kind === 'plan') {
    const steps = planSteps(payload)
    const stepKeys = occurrenceKeys(steps, (step) => step.text)
    return (
      <article {...common}>
        <div className="assistant activity-card">
          <div className="activity-component-heading">
            <ListIcon size={14} />
            <b>Plan</b>
            <Status value={component.status} />
          </div>
          {steps.length > 0 ? (
            <ol className="activity-plan" data-testid="agent-durable-plan">
              {steps.map((step, index) => (
                <li key={stepKeys[index]} data-status={step.status}>
                  {step.text}
                </li>
              ))}
            </ol>
          ) : (
            <p className="activity-component-empty">No plan steps were reported.</p>
          )}
        </div>
      </article>
    )
  }

  if (component.kind === 'reasoning') {
    const text = textField(payload, 'text', 'summary', 'detail')
    return (
      <article {...common}>
        <div className="assistant historical-activity">
          <details className="thinking activity-reasoning" data-testid="agent-reasoning">
            <summary>Reasoning</summary>
            {text && <div className="thinking-body">{text.replace(/\*{1,3}/g, '')}</div>}
          </details>
        </div>
      </article>
    )
  }

  if (component.kind === 'diff') {
    const diff = textField(payload, 'unifiedDiff', 'text')
    return (
      <article {...common}>
        <div className="assistant historical-activity">
          <div className="turn-diff activity-turn-diff" data-testid="agent-turn-diff">
            {diff ? (
              <TurnDiffPreview diff={diff} turnId={component.turnId} wsId={wsId} />
            ) : (
              <p className="activity-component-empty">
                The provider reported changes without a patch.
              </p>
            )}
          </div>
        </div>
      </article>
    )
  }

  if (component.kind === 'citation') {
    const reportedURL = textField(payload, 'url', 'href')
    const url = safeExternalURL(reportedURL)
    const label = textField(payload, 'title', 'label', 'text') ?? reportedURL ?? 'Citation'
    return (
      <article {...common}>
        <div className="assistant activity-card activity-inline-card">
          <LinkIcon size={14} />
          {url ? (
            <a href={url} target="_blank" rel="noreferrer">
              {label}
            </a>
          ) : (
            <span>{label}</span>
          )}
          <Status value={component.status} />
        </div>
      </article>
    )
  }

  if (component.kind === 'attachment') {
    const label = textField(payload, 'name', 'filename', 'path', 'title') ?? 'Attachment'
    return (
      <article {...common}>
        <div className="assistant activity-card activity-inline-card">
          <FileIcon size={14} /> <span>{label}</span> <Status value={component.status} />
        </div>
      </article>
    )
  }

  if (component.kind === 'context_usage' || component.kind === 'rate_limits') {
    const used = payload.usedPercent
    const remaining = payload.remainingPercent
    const label = component.kind === 'context_usage' ? 'Context usage' : 'Rate limits'
    const amount =
      typeof used === 'number'
        ? `${Math.round(used)}% used`
        : typeof remaining === 'number'
          ? `${Math.round(remaining)}% remaining`
          : textField(payload, 'text', 'detail')
    return (
      <article {...common}>
        <div className="assistant activity-card activity-inline-card">
          <GaugeIcon size={14} />{' '}
          <span>
            {label}
            {amount ? ` · ${amount}` : ''}
          </span>
          <Status value={component.status} />
        </div>
      </article>
    )
  }

  const isConnection = component.kind === 'connection_notice'
  const isCompaction = component.kind === 'compaction'
  const Icon = isConnection ? OfflineIcon : isCompaction ? RefreshIcon : AlertIcon
  const text = textField(payload, 'text', 'message', 'detail', 'title') ?? humanKind(component.kind)
  return (
    <article {...common}>
      <div className="assistant activity-card activity-inline-card">
        <Icon size={14} /> <span>{text}</span> <Status value={component.status} />
      </div>
    </article>
  )
}
