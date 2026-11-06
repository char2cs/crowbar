import { memo, useState } from 'react'
import { CaretRightIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { useChatStore } from '@/features/agent/stores/chat-store'
import type { MarkerItem } from '@/features/agent/stores/lib/transcript-items'
import { limitResetsAt, useChatTelemetry } from '@/features/agent/hooks/use-chat-entity'
import {
  describeFailure,
  resetLabel,
  type FailureKind,
} from '@/features/agent/transcript/lib/turn-failure'

/** What a marker row can do: re-send a failed turn, open the CLI, start over. */
export interface MarkerActions {
  /** Sends the prompt `requestId` again, as its retry. */
  retry: (requestId: string) => void
  openTerminal: () => void
  /** The next prompt starts a fresh conversation. */
  newConversation: () => void
}

const EXIT_TEXT: Record<string, string> = {
  exited: 'The agent exited',
  connection_lost: 'Crowbar lost its connection to the agent',
  transport_overflow: 'Crowbar lost its connection to the agent',
  spawn_failed: 'The agent could not start',
}

function interruptPill(reason: string | undefined) {
  // Closed on the provider going quiet, not by anyone: never worded as the user's doing.
  if (reason === 'provider_idle') {
    return { label: 'Interrupted unexpectedly', testId: 'agent-inferred-interrupt-divider' }
  }
  const byYou = reason === 'stopped' || reason === 'retracted'
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
    case 'turn_failed':
      return { label: 'Turn failed', testId: 'agent-failed-divider' }
    case 'session_conflict':
      // Two chats never share a session: this chat kept its own and ended the CLI that left it.
      return {
        label: 'Another chat holds that session; its CLI was ended so nothing goes unrecorded',
        testId: 'agent-session-conflict-divider',
      }
    case 'session_lost':
      return { label: 'The session was lost', testId: 'agent-session-lost-divider' }
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
    case 'warning':
      return { label: item.detail || 'The agent warned of a problem', testId: 'agent-warning' }
    default:
      return { label: item.detail || item.marker, testId: 'agent-marker-divider' }
  }
}

/** True while nothing was asked after the marker: no later prompt in the
 *  transcript or on its way. */
function useNewestFailure(chatId: string, id: string): boolean {
  return useChatStore((s) => {
    const feed = s.feeds[chatId]
    if (!feed || (s.outbox[chatId]?.length ?? 0) > 0) return false
    const at = feed.order.indexOf(id)
    return at >= 0 && feed.order.slice(at + 1).every((later) => feed.items[later]?.kind !== 'user')
  })
}

/** The request id of the prompt that started `turnId`. */
function usePromptOf(chatId: string, turnId: string | undefined): string | undefined {
  return useChatStore((s) => {
    const feed = s.feeds[chatId]
    if (!feed || !turnId) return undefined
    for (const id of feed.order) {
      const item = feed.items[id]
      if (item?.kind === 'user' && item.turnId === turnId) return item.requestId
    }
    return undefined
  })
}

/** The way on from the newest failure: a sign-in to run, or when a limit lifts. */
function FailureHint({
  chatId,
  kind,
  provider,
}: {
  chatId: string
  kind: FailureKind
  provider?: AgentProvider
}) {
  const telemetry = useChatTelemetry(chatId)
  const resetsAt = kind === 'limit' ? limitResetsAt(telemetry) : undefined
  const when = resetsAt ? resetLabel(resetsAt) : ''
  if (kind === 'auth' && provider?.signIn) {
    return (
      <p className="marker-hint" data-testid="agent-failed-hint">
        Sign in: run <code>{provider.signIn}</code> in a terminal
      </p>
    )
  }
  if (!when) return null
  return (
    <p className="marker-hint" data-testid="agent-failed-hint">
      {when}
    </p>
  )
}

// The reason shows on the row itself: a failure read only behind a toggle reads as a hang.
function FailedActions({
  chatId,
  item,
  actions,
  provider,
}: MarkerProps & { actions: MarkerActions; provider?: AgentProvider }) {
  const [open, setOpen] = useState(false)
  const [sent, setSent] = useState(false)
  const requestId = usePromptOf(chatId, item.turnId)
  const newest = useNewestFailure(chatId, item.id)
  const failure = describeFailure(item.reason, item.detail)
  return (
    <div className="marker-failure" data-testid="agent-failed-turn">
      <p className="marker-reason" data-testid="agent-failed-reason">
        {failure.text}
      </p>
      {newest && <FailureHint chatId={chatId} kind={failure.kind} provider={provider} />}
      <div className="marker-actions">
        {failure.more && (
          <button
            type="button"
            className="reasoning-toggle"
            aria-expanded={open}
            onClick={() => setOpen((o) => !o)}
          >
            Details
            <CaretRightIcon size={12} />
          </button>
        )}
        {requestId !== undefined && newest && (
          <Button
            size="sm"
            variant="secondary"
            disabled={sent}
            onClick={() => {
              setSent(true)
              actions.retry(requestId)
            }}
          >
            Retry
          </Button>
        )}
        <Button size="sm" variant="outline" onClick={actions.openTerminal}>
          Open terminal
        </Button>
      </div>
      {open && <pre className="marker-detail">{failure.more}</pre>}
    </div>
  )
}

interface MarkerProps {
  chatId: string
  item: MarkerItem
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
  const { label, testId } = pill(item, providers)
  // Starting over is the answer only while nothing has followed the loss.
  const latest = useChatStore((s) => s.feeds[chatId]?.order.at(-1) === item.id)
  return (
    <div className="marker-row" data-row-id={item.id}>
      <div className="divider" role="separator" data-testid="agent-event-divider">
        <span className="wavy-rule" />
        <span className="tag" data-testid={testId}>
          {label}
        </span>
        <span className="wavy-rule" />
      </div>
      {item.marker === 'turn_failed' && (
        <FailedActions
          chatId={chatId}
          item={item}
          actions={actions}
          provider={providers.find((p) => p.id === provider)}
        />
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

export const MarkerRow = memo(MarkerRowBody)
