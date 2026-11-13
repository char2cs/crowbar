import { memo, useState } from 'react'
import { Button } from '@/components/ui/button'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { MarkerItem, TranscriptItem } from '@/features/agent/stores/lib/transcript-items'
import { markerFailure, stoppedByYou } from '@/features/agent/transcript/lib/marker-failure'
import { warningHeadline } from '@/features/agent/transcript/lib/turn-failure'
import type { NoticeActions } from '@/features/agent/transcript/lib/marker-failure'
import { MoveRetry, moveLabel } from '@/features/agent/transcript/rows/move-marker'

function switchedTo(providers: AgentProvider[], id: string | undefined) {
  const name = providers.find((p) => p.id === id)?.displayName ?? id
  return `Switched to ${name || 'a different provider'}`
}

/** One launch's switches, in one line. */
function launchPill(parts: NonNullable<MarkerItem['parts']>, providers: AgentProvider[]) {
  const said = parts.map(({ marker, detail }) => {
    if (marker === 'provider_switched') return switchedTo(providers, detail)
    if (marker === 'model_changed') return detail || 'default model'
    return `${detail || 'default'} effort`
  })
  return { label: said.join(' · '), testId: 'agent-switch-divider' }
}

interface Pill {
  label: string
  testId: string
}

function stopPill(item: MarkerItem, name: string | undefined): Pill | undefined {
  const failure = markerFailure(item, { provider: name })
  if (!failure) return undefined
  const testIds: Record<string, string> = {
    turn_failed: 'agent-failed-divider',
    session_lost: 'agent-session-lost-divider',
    runner_exited: 'agent-exit-divider',
    turn_interrupted:
      item.reason === 'provider_idle'
        ? 'agent-inferred-interrupt-divider'
        : 'agent-interrupted-divider',
  }
  return { label: failure.tag, testId: testIds[item.marker] ?? 'agent-marker-divider' }
}

function pill(item: MarkerItem, providers: AgentProvider[], provider?: string): Pill {
  if (item.parts) return launchPill(item.parts, providers)
  const stop = stopPill(item, providers.find((p) => p.id === provider)?.displayName)
  if (stop) return stop
  switch (item.marker) {
    case 'compaction_ended':
      return { label: 'Compacted', testId: 'agent-compaction-divider' }
    case 'turn_interrupted':
      return {
        label: stoppedByYou(item.reason) ? 'Interrupted · you stopped this turn' : 'Interrupted',
        testId: 'agent-interrupted-divider',
      }
    case 'from_record':
      return {
        label: 'Session lost · continued from Crowbar’s record',
        testId: 'agent-from-record-divider',
      }
    case 'provider_switched':
      return {
        label: switchedTo(providers, item.detail),
        testId: 'agent-provider-switch-divider',
      }
    case 'model_changed':
      return { label: `Model: ${item.detail || 'default'}`, testId: 'agent-model-switch-divider' }
    case 'effort_changed':
      return { label: `Effort: ${item.detail || 'default'}`, testId: 'agent-effort-switch-divider' }
    case 'move':
      return { label: moveLabel(item.move), testId: 'agent-move-divider' }
    default:
      return { label: item.detail || item.marker, testId: 'agent-marker-divider' }
  }
}

/** What a notice row says: a bold sentence, then what follows from it, in the daemon's
 *  own facts where it gave them. */
function noticeWords(item: MarkerItem): { text: string; rest?: string } | undefined {
  switch (item.marker) {
    case 'warning':
      return warningHeadline(item.detail)
    case 'session_conflict':
      return {
        text: 'This session moved to another chat.',
        rest: 'Its terminal here was closed so nothing goes unrecorded.',
      }
    case 'legacy_unreadable': {
      const count = /^(\d+) legacy record/.exec(item.detail ?? '')?.[1]
      if (!count) return { text: item.detail || 'Some older records could not be converted.' }
      return {
        text: `${count} older ${count === '1' ? 'record' : 'records'} could not be converted.`,
        rest: 'They were written before the last upgrade.',
      }
    }
    default:
      return undefined
  }
}

/** Whether a marker is drawn as a notice card rather than a pill on a line. */
export const isNotice = (item: TranscriptItem | undefined) =>
  item?.kind === 'marker' && noticeWords(item) !== undefined

/** What a notice offers, when it names something to do: the daemon's own facts decide. */
function noticeAction(
  item: MarkerItem,
  actions: NoticeActions,
): { label: string; onClick: () => void } | undefined {
  if (item.marker === 'warning' && /\b(context|compact)/i.test(item.detail ?? '')) {
    return actions.compact && { label: 'Compact now', onClick: actions.compact }
  }
  const other = item.chat
  if (item.marker === 'session_conflict' && other) {
    return { label: 'Open that chat', onClick: () => actions.openChat(other) }
  }
  return undefined
}

/** A boundary the person is told about in a line of its own. */
function NoticeRow({
  item,
  text,
  rest,
  actions,
}: {
  item: MarkerItem
  text: string
  rest?: string
  actions: NoticeActions
}) {
  const [shown, setShown] = useState(false)
  const records = item.marker === 'legacy_unreadable' ? item.records : undefined
  const action = noticeAction(item, actions)
  return (
    <article className="row" data-row-id={item.id} data-role="notice">
      <div className="marker-notice" role="status" data-testid="agent-marker-notice">
        <div className="tx">
          <b>{text || 'The agent warned of a problem'}</b>
          {rest && (
            <>
              {' '}
              <span className="s">{rest}</span>
            </>
          )}
        </div>
        {action && (
          <Button size="sm" variant="outline" className="notice-action" onClick={action.onClick}>
            {action.label}
          </Button>
        )}
        {records && records.length > 0 && (
          <Button
            size="sm"
            variant="outline"
            className="notice-action"
            aria-expanded={shown}
            onClick={() => setShown((was) => !was)}
          >
            {shown ? 'Hide them' : 'Show them'}
          </Button>
        )}
        {shown && records && (
          <ul className="marker-notice-records" data-testid="agent-marker-records">
            {records.map((record) => (
              <li key={record}>{record}</li>
            ))}
          </ul>
        )}
      </div>
    </article>
  )
}

interface MarkerProps {
  chatId: string
  item: MarkerItem
  providers: AgentProvider[]
  actions: NoticeActions
  /** The provider the marker's turn ran on. */
  provider?: string
}

/** One boundary in the conversation, drawn as a pill on a line. What stopped a turn is
 *  the dock's notice; here it stays only as the line it left. */
function MarkerRowBody({ chatId, item, providers, actions, provider }: MarkerProps) {
  const words = noticeWords(item)
  if (words) return <NoticeRow item={item} actions={actions} {...words} />
  const { label, testId } = pill(item, providers, provider)
  return (
    <div className="marker-row" data-row-id={item.id}>
      <div className="divider" role="separator" data-testid="agent-event-divider">
        <span className="wavy-rule" />
        <span className="tag" data-testid={testId}>
          {item.command && (
            <span className="slash-marker" data-testid="slash-command">
              {item.command}
            </span>
          )}
          <span className={item.command ? 'tag-result' : undefined}>{label}</span>
        </span>
        <span className="wavy-rule" />
      </div>
      {item.move && <MoveRetry chatId={chatId} move={item.move} />}
    </div>
  )
}

export const MarkerRow = memo(MarkerRowBody)
