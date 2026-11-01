/** Drawn as soon as the first turn freezes, not on the first reply, so it never
 *  pops in with a layout shift. */
export function FirstTurnDivider() {
  return (
    <div className="divider" role="separator" data-testid="agent-first-turn-divider">
      <span className="ln" />
    </div>
  )
}
