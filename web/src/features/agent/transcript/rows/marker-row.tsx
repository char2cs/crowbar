import { memo } from 'react'
import { Button } from '@/components/ui/button'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { MarkerItem } from '@/features/agent/stores/lib/transcript-items'
import {
  FailedNotice,
  RetryButton,
  WarningNotice,
  type MarkerActions,
  type MarkerProps,
} from '@/features/agent/transcript/rows/marker-failure'
import { MoveRetry, moveLabel } from '@/features/agent/transcript/rows/move-marker'

const EXIT_TEXT: Record<string, string> = {
  exited: 'The agent exited',
  connection_lost: 'Crowbar lost its connection to the agent',
  transport_overflow: 'Crowbar lost its connection to the agent',
  spawn_failed: 'The agent could not start',
}

const stoppedByYou = (reason: string | undefined) => reason === 'stopped' || reason === 'retracted'

function interruptPill(reason: string | undefined) {
  // Closed on the provider going quiet, not by anyone: never worded as the user's doing.
  if (reason === 'provider_idle') {
    return { label: 'Interrupted unexpectedly', testId: 'agent-inferred-interrupt-divider' }
  }
  const byYou = stoppedByYou(reason)
  return {
    label: byYou ? 'Interrupted · you stopped this turn' : 'Interrupted',
    testId: 'agent-interrupted-divider',
  }
}

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

function pill(item: MarkerItem, providers: AgentProvider[]): { label: string; testId: string } {
  if (item.parts) return launchPill(item.parts, providers)
  switch (item.marker) {
    case 'compaction_ended':
      return { label: 'Compacted', testId: 'agent-compaction-divider' }
    case 'turn_interrupted':
      return interruptPill(item.reason)
    case 'session_conflict':
      return {
        label: 'Another chat holds that session',
        testId: 'agent-session-conflict-divider',
      }
    case 'session_lost':
      return { label: 'The session was lost', testId: 'agent-session-lost-divider' }
    case 'from_record':
      return {
        label: 'Continued from Crowbar’s record',
        testId: 'agent-from-record-divider',
      }
    case 'runner_exited':
      return {
        label: EXIT_TEXT[item.reason ?? ''] ?? 'The agent stopped',
        testId: 'agent-exit-divider',
      }
    case 'provider_switched': {
      return {
        label: switchedTo(providers, item.detail),
        testId: 'agent-provider-switch-divider',
      }
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

/** One boundary in the conversation, drawn as a pill on a line, with what can
 *  be done about it when it is a failure. */
function MarkerRowBody({
  chatId,
  item,
  providers,
  provider,
  actions,
}: MarkerProps & { providers: AgentProvider[]; provider?: string; actions: MarkerActions }) {
  // A failure or warning is a notice of its own; every other boundary is a pill on a line.
  const notice = item.marker === 'turn_failed' || item.marker === 'warning'
  const { label, testId } = pill(item, providers)
  // Starting over is the answer only while nothing has followed the loss.
  const latest = useChatStore((s) => s.feeds[chatId]?.order.at(-1) === item.id)
  return (
    <div className="marker-row" data-row-id={item.id}>
      {!notice && (
        <div className="divider" role="separator" data-testid="agent-event-divider">
          <span className="wavy-rule" />
          <span className="tag" data-testid={testId}>
            {label}
          </span>
          <span className="wavy-rule" />
        </div>
      )}
      {item.marker === 'warning' && <WarningNotice detail={item.detail} />}
      {item.move && <MoveRetry chatId={chatId} move={item.move} />}
      {item.marker === 'turn_failed' && (
        <FailedNotice
          chatId={chatId}
          item={item}
          actions={actions}
          provider={providers.find((p) => p.id === provider)}
        />
      )}
      {/* A turn its CLI's exit cut short keeps what it said; the prompt can go again. */}
      {item.marker === 'turn_interrupted' && !stoppedByYou(item.reason) && (
        <div className="marker-actions">
          <RetryButton chatId={chatId} item={item} actions={actions} />
        </div>
      )}
      {item.marker === 'session_lost' && latest && (
        <div className="marker-actions">
          <Button size="sm" variant="secondary" onClick={actions.newConversation}>
            Start a new conversation
          </Button>
        </div>
      )}
      {item.marker === 'runner_exited' && latest && (
        <div className="marker-actions">
          <Button size="sm" variant="outline" onClick={actions.openTerminal}>
            Open terminal
          </Button>
        </div>
      )}
    </div>
  )
}

export type { MarkerActions }

export const MarkerRow = memo(MarkerRowBody)
