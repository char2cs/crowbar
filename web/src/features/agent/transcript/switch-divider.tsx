import type { AgentProvider } from '@/features/agent/api/agent-api'

export type SwitchKind = 'provider' | 'model' | 'effort'

function switchLabel(what: SwitchKind, detail: string, providers: AgentProvider[]): string {
  switch (what) {
    case 'provider': {
      const name = providers.find((p) => p.id === detail)?.displayName ?? detail
      return `Switched to ${name || 'a different provider'}`
    }
    case 'model':
      return `Model: ${detail || 'default'}`
    case 'effort':
      return `Effort: ${detail || 'default'}`
  }
}

/**
 * The provider/model/effort boundary — Crowbar's own doing, never something a
 * provider reports (see engine/agents' InterruptProviderSwitched/
 * InterruptModelChanged/InterruptEffortChanged). Reuses CompactionDivider's
 * markup byte-for-byte, same rule that already lets InterruptedDivider borrow
 * it: one visual language for "something changed here", tag text swapped.
 */
export function SwitchDivider({
  what,
  detail,
  providers,
}: {
  what: SwitchKind
  detail: string
  providers: AgentProvider[]
}) {
  return (
    <div className="divider" role="separator" data-testid={`agent-${what}-switch-divider`}>
      <span className="ln" />
      <span className="tag">{switchLabel(what, detail, providers)}</span>
      <span className="ln" />
    </div>
  )
}
