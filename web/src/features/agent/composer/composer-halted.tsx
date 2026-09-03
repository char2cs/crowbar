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
 * The provider's own words about why it stopped, in the composer's slot.
 *
 * The text is RELAYED, never composed by Crowbar: a usage limit, a quota, a
 * refusal are the provider's sentences, and paraphrasing them would put Crowbar's
 * name on a claim it cannot stand behind. The footer selectors stay live under
 * this — switching model is the one thing that can actually clear a limit.
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
