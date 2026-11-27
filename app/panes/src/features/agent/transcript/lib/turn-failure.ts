export type FailureKind = 'auth' | 'limit' | 'overloaded' | 'other'

export interface TurnFailure {
  kind: FailureKind
  headline: string
  hint?: string
  tag: string
}

const KINDS: readonly FailureKind[] = ['auth', 'limit', 'overloaded']

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

function limitHint(resetsAt: Date | undefined, now: Date): string {
  if (!resetsAt || Number.isNaN(resetsAt.getTime()) || resetsAt <= now) return ANOTHER_MODEL
  const clock = resetsAt.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  return `Resets at ${clock}, in ${untilLabel(resetsAt.getTime() - now.getTime())}. ${ANOTHER_MODEL}`
}

const capitalised = (text: string) => text.charAt(0).toUpperCase() + text.slice(1)

export interface FailureInput {
  kind?: string
  provider?: string
  resetsAt?: string
  now?: Date
}

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

const SENTENCE_END = /[.!?](?=\s+[A-Z0-9])/

export function warningHeadline(detail = ''): { text: string; rest?: string } {
  const said = detail.trim()
  const end = said.search(SENTENCE_END)
  if (end < 0) return { text: said }
  return { text: said.slice(0, end + 1), rest: said.slice(end + 1).trim() }
}
