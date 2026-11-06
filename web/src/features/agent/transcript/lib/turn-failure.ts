/** What kind of trouble failed a turn, as far as its words tell: a sign-in has a
 *  way out the person can take, a limit lifts by itself, anything else is retried. */
export type FailureKind = 'auth' | 'limit' | 'other'

export interface TurnFailure {
  kind: FailureKind
  /** The provider's own first line, else a plain sentence for its code. */
  text: string
  /** The rest of what the provider said, when it said more than one line. */
  more?: string
}

const AUTH = /\b40[13]\b|unauthori[sz]ed|authentication|not logged in|invalid api key|log ?in/i
const LIMIT = /\b429\b|\b529\b|rate.?limit|usage.?limit|too many requests|overloaded|quota/i

// A reason that is a code, not a sentence: the provider gave no words of its own.
const CODE = /^[a-z_]+$/
const CODE_TEXT: Record<FailureKind, string> = {
  auth: 'The provider refused the sign-in.',
  limit: 'The provider’s usage limit was reached.',
  other: 'The provider stopped this turn.',
}

export function describeFailure(reason = '', detail = ''): TurnFailure {
  const said = `${reason}\n${detail}`
  const kind: FailureKind = AUTH.test(said) ? 'auth' : LIMIT.test(said) ? 'limit' : 'other'
  const words = detail.trim() || (CODE.test(reason) ? '' : reason.trim())
  if (!words) return { kind, text: CODE_TEXT[kind] }
  const [first = '', ...rest] = words.split('\n')
  return { kind, text: first, more: rest.join('\n').trim() || undefined }
}

/** When a limit lifts, in the person's own clock; '' for an unreadable time. */
export function resetLabel(resetsAt: string): string {
  const at = new Date(resetsAt)
  if (Number.isNaN(at.getTime())) return ''
  return `Resets ${at.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })}`
}
