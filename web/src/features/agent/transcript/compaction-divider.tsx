/**
 * The compaction boundary.
 *
 * Everything above this line is no longer in the model's context; the transcript
 * still holds all of it. That gap is the entire reason the line exists — without
 * it a reader scrolls up, sees the conversation, and reasonably assumes the
 * agent can still see it too.
 *
 * `manual` and `auto` are kept apart because they answer different questions: one
 * is something the user did, the other is something that happened to them.
 */
export function CompactionDivider({ trigger }: { trigger?: 'manual' | 'auto' | string }) {
  return (
    <div className="divider" role="separator" data-testid="agent-compaction-divider">
      <span className="ln" />
      <span className="tag">{trigger === 'manual' ? 'Compacted' : 'Compacted automatically'}</span>
      <span className="ln" />
    </div>
  )
}
