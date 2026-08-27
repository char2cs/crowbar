/**
 * Marks where a person stopped the chat's first turn after it had already
 * dispatched — the frozen document (message-row.tsx) stays exactly where it
 * is, the dock does not snap back to blank, and this is the record that
 * something happened here rather than the turn simply trailing off.
 *
 * Reuses `CompactionDivider`'s real markup byte-for-byte, tag text swapped —
 * not a new visual language, the same rule `CompactionDivider` itself already
 * uses to tell "Compacted" from "Compacted automatically" apart.
 *
 * LOCAL ONLY. Unlike a compaction boundary, nothing on the backend records
 * that a person clicked Stop — `stopChat` gracefully ends the CLI and leaves
 * no notice, no interruption entry, nothing this client could read back. This
 * draws from this session's own memory of having clicked it, which is real —
 * the click happened — even though it will not survive a reload the way
 * everything else this transcript draws does.
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
