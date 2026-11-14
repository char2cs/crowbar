/** What kind of trouble failed a turn, as the provider's descriptor classified it: a sign-in
 *  has a way out the person can take, a limit lifts by itself, an overload passes. */
export type FailureKind = 'auth' | 'limit' | 'overloaded' | 'other'

export interface TurnFailure {
  kind: FailureKind
  /** One plain sentence: no error code, HTTP number or JSON. */
  headline: string
  /** At most one short second line. */
  hint?: string
  /** The transcript's divider for the stopped turn. */
  tag: string
}

const KINDS: readonly FailureKind[] = ['auth', 'limit', 'overloaded']

/** The kind a row names; none, or one this build does not know, is any other trouble. */
function failureKindOf(named: string | undefined): FailureKind {
  return KINDS.find((kind) => kind === named) ?? 'other'
}

function untilLabel(ms: number): string {
  const minutes = Math.max(1, Math.round(ms / 60_000))
  if (minutes < 60) return `${minutes}m`
  const rest = minutes % 60
  return rest === 0 ? `${Math.floor(minutes / 60)}h` : `${Math.floor(minutes / 60)}h ${rest}m`
}

const ANOTHER_MODEL = 'Another model can keep going now.'

/** "Resets at 7:00 PM, in 2h 14m. Another model can keep going now." */
function limitHint(resetsAt: Date | undefined, now: Date): string {
  if (!resetsAt || Number.isNaN(resetsAt.getTime()) || resetsAt <= now) return ANOTHER_MODEL
  const clock = resetsAt.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  return `Resets at ${clock}, in ${untilLabel(resetsAt.getTime() - now.getTime())}. ${ANOTHER_MODEL}`
}

const capitalised = (text: string) => text.charAt(0).toUpperCase() + text.slice(1)

export interface FailureInput {
  /** The kind the daemon classified the failure as (a turn_failed row's `kind`). */
  kind?: string
  /** The provider's name, as the person knows it; "the agent" when none is known. */
  provider?: string
  /** When a limit lifts, where the provider reported it. */
  resetsAt?: string
  now?: Date
}

/** A failed turn in the board's words: the provider's own text and codes are never shown. */
export function describeFailure({
  kind,
  provider = 'the agent',
  resetsAt,
  now = new Date(),
}: FailureInput): TurnFailure {
  const name = provider || 'the agent'
  switch (failureKindOf(kind)) {
    case 'auth':
      return {
        kind: 'auth',
        headline: `Signed out of ${name}`,
        hint: 'Sign in again to continue.',
        tag: `Stopped · signed out of ${name}`,
      }
    case 'limit': {
      const lifts = resetsAt ? new Date(resetsAt) : undefined
      return {
        kind: 'limit',
        headline: 'Usage limit reached',
        hint: limitHint(lifts, now),
        tag: 'Stopped · usage limit reached',
      }
    }
    case 'overloaded':
      return {
        kind: 'overloaded',
        headline: `${capitalised(name)} is busy right now`,
        hint: 'Try again in a moment. Nothing was lost.',
        tag: `${capitalised(name)} was busy · this turn did not finish`,
      }
    default: {
      const headline = `${capitalised(name)} had a problem on its side`
      return {
        kind: 'other',
        headline,
        hint: 'Try again. Nothing was lost.',
        tag: `Stopped · ${headline}`,
      }
    }
  }
}

// A sentence ends at a stop before a capital or a figure; a dot inside a path or a URL does not.
const SENTENCE_END = /[.!?](?=\s+[A-Z0-9])/

/** A provider's warning: its first sentence in bold, the rest as the line under it. */
export function warningHeadline(detail = ''): { text: string; rest?: string } {
  const said = detail.trim()
  const end = said.search(SENTENCE_END)
  if (end < 0) return { text: said }
  return { text: said.slice(0, end + 1), rest: said.slice(end + 1).trim() }
}
