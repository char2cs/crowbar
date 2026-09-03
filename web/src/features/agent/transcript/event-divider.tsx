import type { AgentProvider } from '@/features/agent/api/agent-api'
import type { DividerTag } from '@/features/agent/transcript/lib/flatten-transcript-rows'

function tagLabel(tag: DividerTag, providers: AgentProvider[]): string {
  switch (tag.kind) {
    case 'compaction':
      return tag.trigger === 'manual' ? 'Compacted' : 'Compacted automatically'
    case 'interrupted':
      return 'Interrupted'
    case 'provider': {
      const name = providers.find((p) => p.id === tag.detail)?.displayName ?? tag.detail
      return `Switched to ${name || 'a different provider'}`
    }
    case 'model':
      return `Model: ${tag.detail || 'default'}`
    case 'effort':
      return `Effort: ${tag.detail || 'default'}`
  }
}

function tagTestId(tag: DividerTag): string {
  switch (tag.kind) {
    case 'compaction':
      return 'agent-compaction-divider'
    case 'interrupted':
      return 'agent-interrupted-divider'
    case 'provider':
      return 'agent-provider-switch-divider'
    case 'model':
      return 'agent-model-switch-divider'
    case 'effort':
      return 'agent-effort-switch-divider'
  }
}

/**
 * One boundary line, one or more pills. Everything that landed before the
 * same next message — a compaction, an interrupted turn, a provider/model/
 * effort switch — used to draw its own full-width wavy divider, stacking
 * into a wall of identical lines whenever more than one happened back to
 * back. This is the merge: one `.divider`, N `.tag` pills inside it, same
 * markup CompactionDivider always used, just able to hold more than one tag.
 *
 * Order is the caller's: agent-chat-view.tsx sorts the underlying
 * interruptions chronologically before grouping them by anchor, so `tags`
 * arrives already in the order these things actually happened.
 */
export function EventDivider({
  tags,
  providers,
}: {
  tags: DividerTag[]
  providers: AgentProvider[]
}) {
  return (
    <div className="divider" role="separator" data-testid="agent-event-divider">
      <span className="ln" />
      {tags.map((tag) => (
        <span key={tag.kind} className="tag" data-testid={tagTestId(tag)}>
          {tagLabel(tag, providers)}
        </span>
      ))}
      <span className="ln" />
    </div>
  )
}
