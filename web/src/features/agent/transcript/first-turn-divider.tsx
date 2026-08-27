/**
 * The boundary between the frozen first turn and the conversation proper.
 *
 * Plain, unlike `CompactionDivider` — there is nothing to say here beyond
 * "the document ends and the chat begins", so no tag rides the line. It draws
 * as soon as the first turn freezes, whether or not anything has replied to it
 * yet: waiting for a reply would mean the line popping in with a layout shift
 * once the agent responds.
 */
export function FirstTurnDivider() {
  return (
    <div className="divider" role="separator" data-testid="agent-first-turn-divider">
      <span className="ln" />
    </div>
  )
}
