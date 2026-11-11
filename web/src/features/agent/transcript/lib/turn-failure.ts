/** What kind of trouble failed a turn, as far as its words tell: a sign-in has a
 *  way out the person can take, a limit lifts by itself, an overload passes. */
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

const AUTH = /\b40[13]\b|unauthori[sz]ed|authentication|not logged in|invalid api key|log ?in/i
const OVERLOADED = /\b529\b|overloaded/i
const LIMIT = /\b429\b|rate.?limit|usage.?limit|too many requests|quota/i
// "resets at 7pm", "reset at 7:30 PM": the provider's own clock, read in the person's.
const RESET_AT = /resets?\s+(?:at\s+)?(\d{1,2})(?::(\d{2}))?\s*(am|pm)/i

function kindOf(said: string): FailureKind {
  if (AUTH.test(said)) return 'auth'
  if (OVERLOADED.test(said)) return 'overloaded'
  return LIMIT.test(said) ? 'limit' : 'other'
}

/** The next time the clock reads `hour:minute` after `now`, from the words "resets at 7pm". */
function resetFromWords(said: string, now: Date): Date | undefined {
  const match = RESET_AT.exec(said)
  if (!match) return undefined
  const hour = Number(match[1])
  if (hour < 1 || hour > 12) return undefined
  const afternoon = match[3]?.toLowerCase() === 'pm'
  const at = new Date(now)
  at.setHours((hour % 12) + (afternoon ? 12 : 0), Number(match[2] ?? 0), 0, 0)
  if (at <= now) at.setDate(at.getDate() + 1)
  return at
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
  reason?: string
  detail?: string
  /** The provider's name, as the person knows it; "the agent" when none is known. */
  provider?: string
  /** When a limit lifts, where the provider reported it. */
  resetsAt?: string
  now?: Date
}

/** A failed turn in the board's words. The provider's own text only decides which
 *  words these are: it is never shown, and neither are its codes or numbers. */
export function describeFailure({
  reason = '',
  detail = '',
  provider = 'the agent',
  resetsAt,
  now = new Date(),
}: FailureInput): TurnFailure {
  const said = `${reason}\n${detail}`
  const name = provider || 'the agent'
  switch (kindOf(said)) {
    case 'auth':
      return {
        kind: 'auth',
        headline: `Signed out of ${name}`,
        hint: 'Sign in again to continue.',
        tag: `Stopped · signed out of ${name}`,
      }
    case 'limit': {
      const lifts = resetsAt ? new Date(resetsAt) : resetFromWords(said, now)
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
        headline: `${capitalised(name)} is busy`,
        hint: 'Try again in a moment.',
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

// A sentence ends at a stop before a capital; a dot inside a path or a URL does not.
const SENTENCE_END = /[.!?](?=\s+[A-Z])/

/** A provider's warning: its first sentence in bold, the rest as the line under it. */
export function warningHeadline(detail = ''): { text: string; rest?: string } {
  const said = detail.trim()
  const end = said.search(SENTENCE_END)
  if (end < 0) return { text: said }
  return { text: said.slice(0, end + 1), rest: said.slice(end + 1).trim() }
}
