/** What kind of trouble failed a turn, as far as its words tell: a sign-in has a
 *  way out the person can take, a limit lifts by itself, an overload passes. */
export type FailureKind = 'auth' | 'limit' | 'overloaded' | 'other'

export interface TurnFailure {
  kind: FailureKind
  /** One plain sentence: no error code, HTTP number or JSON. */
  headline: string
  /** At most one short second line. */
  hint?: string
  /** The provider's raw words, kept for a Details toggle; never shown by default. */
  more?: string
}

const AUTH = /\b40[13]\b|unauthori[sz]ed|authentication|not logged in|invalid api key|log ?in/i
const OVERLOADED = /\b529\b|overloaded/i
const LIMIT = /\b429\b|rate.?limit|usage.?limit|too many requests|quota/i

// A reason that is a code, not a sentence: the provider gave no words of its own.
const CODE = /^[a-z_]+$/
// A status the provider put in front of its sentence: "API Error: 500 ...".
const STATUS_PREFIX = /^(?:api error:?\s*)?(?:(?:http|status)\s*)?\d{3}\b[:\s-]*/i

const SAID: Record<Exclude<FailureKind, 'other'>, Pick<TurnFailure, 'headline' | 'hint'>> = {
  auth: { headline: 'You are signed out', hint: 'Sign in again to continue.' },
  limit: { headline: 'Usage limit reached' },
  overloaded: { headline: 'The provider is overloaded.', hint: 'Try again in a moment.' },
}
const STOPPED = 'The provider stopped this turn.'

/** The message inside a provider error whose words end in its raw JSON body. */
function jsonMessage(words: string): string | undefined {
  const start = words.indexOf('{')
  if (start < 0 || !words.endsWith('}')) return undefined
  try {
    const body = JSON.parse(words.slice(start)) as {
      message?: unknown
      error?: { message?: unknown }
    }
    const message = body.error?.message ?? body.message
    return typeof message === 'string' && message ? message : undefined
  } catch {
    return undefined // not JSON after all: the words stand as they are
  }
}

function kindOf(said: string): FailureKind {
  if (AUTH.test(said)) return 'auth'
  if (OVERLOADED.test(said)) return 'overloaded'
  return LIMIT.test(said) ? 'limit' : 'other'
}

export function describeFailure(reason = '', detail = ''): TurnFailure {
  const words = detail.trim() || (CODE.test(reason) ? '' : reason.trim())
  const kind = kindOf(`${reason}\n${detail}`)
  const more = words || undefined
  if (kind !== 'other') return { kind, ...SAID[kind], more }
  const first = (jsonMessage(words) ?? words.split('\n')[0] ?? '').replace(STATUS_PREFIX, '')
  const headline = first.trim() || STOPPED
  return { kind, headline, more: more === headline ? undefined : more }
}

/** When a limit lifts, in the person's own clock; '' for an unreadable time. */
export function resetLabel(resetsAt: string): string {
  const at = new Date(resetsAt)
  if (Number.isNaN(at.getTime())) return ''
  return `Resets ${at.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}`
}

// A sentence ends at a stop before a capital; a dot inside a path or a URL does not.
const SENTENCE_END = /[.!?](?=\s+[A-Z])/

/** A provider's warning, said by its first sentence; `more` is its whole message. */
export function warningHeadline(detail = ''): { text: string; more?: string } {
  const said = detail.trim()
  const end = said.search(SENTENCE_END)
  return end < 0 ? { text: said } : { text: said.slice(0, end + 1), more: said }
}
