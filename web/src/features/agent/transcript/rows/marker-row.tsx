import { memo } from 'react'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { MarkerItem } from '@/features/agent/stores/lib/transcript-items'

function pill(item: MarkerItem, providers: AgentProvider[]): { label: string; testId: string } {
  switch (item.marker) {
    case 'compaction_ended':
      return { label: 'Compacted', testId: 'agent-compaction-divider' }
    case 'turn_interrupted':
      // Closed on the provider going quiet, not by anyone: worded so it never
      // reads as something the user did.
      return item.reason === 'provider_idle'
        ? { label: 'Interrupted unexpectedly', testId: 'agent-inferred-interrupt-divider' }
        : { label: 'Interrupted', testId: 'agent-interrupted-divider' }
    case 'turn_failed':
      return { label: 'Turn failed', testId: 'agent-failed-divider' }
    case 'provider_switched': {
      const name = providers.find((p) => p.id === item.detail)?.displayName ?? item.detail
      return {
        label: `Switched to ${name || 'a different provider'}`,
        testId: 'agent-provider-switch-divider',
      }
    }
    case 'model_changed':
      return { label: `Model: ${item.detail || 'default'}`, testId: 'agent-model-switch-divider' }
    case 'effort_changed':
      return { label: `Effort: ${item.detail || 'default'}`, testId: 'agent-effort-switch-divider' }
    default:
      return { label: item.detail || item.marker, testId: 'agent-marker-divider' }
  }
}

/** One boundary in the conversation, drawn as a pill on a line. */
function MarkerRowBody({ item, providers }: { item: MarkerItem; providers: AgentProvider[] }) {
  const { label, testId } = pill(item, providers)
  return (
    <div
      className="divider"
      role="separator"
      data-row-id={item.id}
      data-testid="agent-event-divider"
    >
      <span className="ln" />
      <span className="tag" data-testid={testId}>
        {label}
      </span>
      <span className="ln" />
    </div>
  )
}

export const MarkerRow = memo(MarkerRowBody)
