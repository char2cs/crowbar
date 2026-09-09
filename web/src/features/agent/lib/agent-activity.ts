import type {
  AgentActivity,
  AgentChoice,
  AgentChoiceOption,
  AgentChoiceQuestion,
  AgentInterruption,
  AgentToolCall,
} from '@/features/agent/api/agent-api'

/** The empty activity, so a caller never has to null-check four lists. */
export const NO_ACTIVITY: AgentActivity = {
  toolCalls: [],
  subagents: [],
  interruptions: [],
  choices: [],
}

/** An interruption the agent is still blocked on.
 *
 *  Only the LATEST unresolved one is surfaced: they are states, not a log, and a
 *  stack of "waiting for permission" banners tells a user nothing the top one
 *  does not. */
export function blockedOn(activity: AgentActivity): AgentInterruption | null {
  const open = activity.interruptions.filter((i) => !i.resolvedAt)
  if (open.length === 0) return null
  return open.reduce((latest, next) => (next.seq > latest.seq ? next : latest))
}

/**
 * The prompts the agent is still waiting on a human to answer, oldest first.
 *
 * Unlike an interruption these are NOT collapsed to the latest one. An
 * interruption is a state and a stack of them tells a reader nothing the top one
 * does not; a prompt is a QUESTION, each is separately answerable, and hiding all
 * but the newest would leave the CLI blocked on one nobody was shown.
 *
 * `pending` is read straight off the server on every poll, which is what makes a
 * prompt answered at the terminal disappear here without this view being told:
 * the terminal can resolve one at any instant, so this is advisory and the next
 * read is the authority.
 */
export function pendingChoices(activity: AgentActivity): AgentChoice[] {
  return activity.choices.filter((choice) => choice.pending).sort((a, b) => a.seq - b.seq)
}

/**
 * What a permission is actually about to touch.
 *
 * A prompt carries the tool's NAME but never its target — the target is reported
 * on the tool call the permission gates, which the provider opened moments
 * earlier in the same turn. '' when no such call is in flight, because a guess
 * about which file is about to be written would be worse than saying nothing.
 */
export function choiceToolTarget(activity: AgentActivity, choice: AgentChoice): string {
  if (!choice.toolName) return ''
  const gated = activity.toolCalls.find(
    (call) =>
      call.turnId === choice.turnId && call.name === choice.toolName && call.status === 'running',
  )
  return gated?.target ?? ''
}

/**
 * What a prompt is asking, in ONE shape whatever wrote it.
 *
 * A prompt recorded since questions were modelled carries them directly. One
 * recorded before that — the graceful fallback, not a migration — is a single
 * question described by the prompt's own `question` and `options`, so it is
 * presented as a list of one. Callers therefore never branch on which of the two
 * a record happens to be, and a three-question prompt is never mistaken for a
 * one-question one.
 *
 * A permission and an elicitation ask no question in this sense: their controls
 * are allow/deny and the MCP verbs, which are not a pick from a list the agent
 * offered.
 */
export function choiceQuestions(choice: AgentChoice): AgentChoiceQuestion[] {
  if (choice.questions && choice.questions.length > 0) return choice.questions
  if (choice.kind !== 'question') return []
  const options = choice.options.filter((option) => option.kind === 'answer')
  if (options.length === 0) return []
  return [
    {
      id: 'q0',
      title: choice.title,
      text: choice.question,
      multi: choice.multi ?? false,
      options,
    },
  ]
}

/** The headline of a prompt: what is being asked, in one line.
 *
 *  Each kind is a genuinely different thing to put to someone, which is why they
 *  are not collapsed into one string — and why a kind this build has never heard
 *  of falls through to whatever the provider did say rather than to a guess. */
export function describeChoice(choice: AgentChoice): string {
  switch (choice.kind) {
    case 'tool_permission':
      return choice.toolName ? `Run ${choice.toolName}?` : 'The agent is asking for permission'
    case 'question': {
      if (choice.question || choice.title) return choice.question || choice.title || ''
      // Several questions have no single headline, and naming one of them would be
      // a lie a reader could act on — so the count is what is said instead.
      const asked = choice.questions?.length ?? 0
      return asked > 1 ? `The agent has ${asked} questions` : 'The agent has a question'
    }
    case 'elicitation':
      return choice.question || 'A tool is asking for some details'
    default:
      return choice.question || choice.title || 'The agent is waiting for your answer'
  }
}

/** The second line under the headline — the permission's target, or the title the
 *  headline did not already use. '' draws no line at all. */
export function choiceDetail(activity: AgentActivity, choice: AgentChoice): string {
  if (choice.kind === 'tool_permission') return choiceToolTarget(activity, choice)
  const headline = describeChoice(choice)
  return choice.title && choice.title !== headline ? choice.title : ''
}

/** An option with no label is named by its kind — Crowbar's own word for it —
 *  so a provider that labels nothing still gets a legible control rather than
 *  a blank. Shared by the live card (composer-choice.tsx) and the resolved
 *  record below, so the two can never spell the same option differently. */
export function optionLabel(option: AgentChoiceOption): string {
  if (option.label) return option.label
  return option.kind.charAt(0).toUpperCase() + option.kind.slice(1)
}

/** Choices no longer pending, oldest first — the record of what was decided,
 *  once nobody is waiting on it any more. */
export function resolvedChoices(activity: AgentActivity): AgentChoice[] {
  return activity.choices.filter((choice) => !choice.pending).sort((a, b) => a.seq - b.seq)
}

/** What a resolved choice's `answeredOptionIds` actually named, read back
 *  against the options it was asked with — a permission's allow/deny, an
 *  elicitation's verb, or a question's own answers. Empty when the choice
 *  carries none: nothing was answered through Crowbar (`proceeded`,
 *  `abandoned`), or it predates this being recorded at all. */
export function pickedOptionLabels(choice: AgentChoice): string[] {
  const ids = choice.answeredOptionIds
  if (!ids || ids.length === 0) return []
  const all: AgentChoiceOption[] = [
    ...choice.options,
    ...(choice.questions ?? []).flatMap((q) => q.options),
  ]
  const byID = new Map(all.map((option) => [option.id, option]))
  return ids.map((id) => {
    const option = byID.get(id)
    return option ? optionLabel(option) : id
  })
}

/** One line for a choice that is no longer pending — the transcript's only
 *  record that a permission was ever asked, once it stops blocking anyone.
 *  `Bash · Allow` reads the same way a tool row's own `name · target` does:
 *  what it was about, then what happened.
 *
 *  `proceeded` (decided at the CLI's own terminal) and `abandoned` (never
 *  decided) are told apart from `answered` because they are different facts —
 *  and from each other, because "the terminal handled it" and "nobody
 *  answered" call for different reactions from a reader scanning back. */
export function describeResolvedChoice(choice: AgentChoice): string {
  const subject = choice.toolName || describeChoice(choice)
  if (choice.resolution === 'proceeded') return `${subject} · answered at the terminal`
  if (choice.resolution === 'abandoned') return `${subject} · left unanswered`
  const picked = pickedOptionLabels(choice)
  const verb = picked.length > 0 ? picked.join(', ') : 'Answered'
  return `${subject} · ${verb}`
}

/** Tool calls still running, oldest first — which is the order they started and
 *  the order a reader scans. */
export function runningTools(activity: AgentActivity): AgentToolCall[] {
  return activity.toolCalls.filter((c) => c.status === 'running').sort((a, b) => a.seq - b.seq)
}

/** Subagents still working. Starts and stops do NOT balance on either provider —
 *  a stop also fires for anonymous internal subagents — so this counts what has
 *  a start and no end, and never tries to reconcile the two populations. */
export function runningSubagents(activity: AgentActivity): number {
  return activity.subagents.filter((s) => !s.endedAt).length
}

/** How a tool call reads in one line: the tool, and what it acted on when the
 *  provider said so. */
export function describeTool(call: AgentToolCall): string {
  if (!call.target) return call.name
  return `${call.name} · ${call.target}`
}

/**
 * The interruption kinds that mean the agent is waiting on a PERSON.
 *
 * The rest are things Crowbar did to the chat itself — it stopped the turn, it
 * switched provider, model or effort — and the transcript already draws a pill for
 * each. The agent is not blocked on anyone for those, so a turn in flight during
 * one is still a turn in flight.
 *
 * Compaction is deliberately absent: it is the CLI's own housekeeping, and its
 * ledger record is born already resolved, so it is driven by a live push instead
 * (see WorkingLine's `compactingLive`).
 */
const PERSON_BLOCKING: ReadonlySet<string> = new Set(['permission', 'notification', 'elicitation'])

export function blocksOnAPerson(interruption: AgentInterruption | null): boolean {
  return interruption !== null && PERSON_BLOCKING.has(interruption.kind)
}

/** Human copy for why the agent is stopped. Each kind is a genuinely different
 *  thing to tell someone, which is why they are not collapsed into one string. */
export function describeInterruption(interruption: AgentInterruption): string {
  switch (interruption.kind) {
    case 'permission':
      return interruption.detail
        ? `Waiting for your permission to run ${interruption.detail}`
        : 'Waiting for your permission'
    case 'notification':
      return interruption.detail || 'The agent needs your attention'
    case 'elicitation':
      return interruption.detail || 'The agent is waiting for input'
    case 'compaction':
      return 'Compacting the conversation to free context'
    default:
      return interruption.detail || 'The agent is waiting'
  }
}

/** A duration a person can read at a glance. Sub-second work is reported in
 *  milliseconds because "0s" reads as "nothing happened". */
export function formatDuration(ms: number | undefined): string {
  if (!ms || ms < 0) return ''
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  const minutes = Math.floor(ms / 60_000)
  const seconds = Math.round((ms % 60_000) / 1000)
  return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
}
