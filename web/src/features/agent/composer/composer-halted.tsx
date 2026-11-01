interface ComposerHaltedProps {
  message: string
  /** When the limit lifts, if the provider said. */
  resetsAt?: string
}

function resetLabel(resetsAt: string): string {
  const at = new Date(resetsAt)
  if (Number.isNaN(at.getTime())) return ''
  return `Resets ${at.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}`
}

/**
 * Relays the provider's own words, never paraphrased: Crowbar cannot stand behind
 * them. Footer selectors stay live — switching model can clear a limit.
 */
export function ComposerHalted({ message, resetsAt }: ComposerHaltedProps) {
  const when = resetsAt ? resetLabel(resetsAt) : ''
  return (
    <div className="pill halted" role="alert">
      <span className="msg">{message}</span>
      {when && <span className="when">{when}</span>}
    </div>
  )
}
