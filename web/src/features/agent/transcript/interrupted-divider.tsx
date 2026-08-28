/**
 * Marks where a person stopped an in-flight turn — the frozen document
 * (message-row.tsx) stays exactly where it is, the dock does not snap back to
 * blank, and this is the record that something happened here rather than the
 * turn simply trailing off.
 *
 * Reuses `CompactionDivider`'s real markup byte-for-byte, tag text swapped —
 * not a new visual language, the same rule `CompactionDivider` itself already
 * uses to tell "Compacted" from "Compacted automatically" apart.
 *
 * Positioned the same way a compaction boundary is: a real, sequence-anchored
 * `stopped` interruption the backend records (turn.RecordStop) the instant it
 * asks the CLI to stop, not this session's own memory of having clicked it —
 * see agent-chat-view.tsx's `interruptedBefore`/`trailingInterruption`.
 */
export function InterruptedDivider() {
  return (
    <div className="divider" role="separator" data-testid="agent-interrupted-divider">
      <span className="ln" />
      <span className="tag">Interrupted</span>
      <span className="ln" />
    </div>
  )
}
