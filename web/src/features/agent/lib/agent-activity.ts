import type {
  AgentChoice,
  AgentChoiceOption,
  AgentChoiceQuestion,
} from '@/features/agent/api/agent-chat-activity'

/**
 * What a prompt is asking, in one shape whatever wrote it: one recorded before
 * questions were modelled is a single question made of its own `question` and
 * `options`. A permission or an elicitation asks no question in this sense.
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

/** The headline of a prompt; a kind this build does not know falls through to
 *  whatever the provider said rather than to a guess. */
export function describeChoice(choice: AgentChoice): string {
  switch (choice.kind) {
    case 'tool_permission':
      return choice.toolName ? `Run ${choice.toolName}?` : 'The agent is asking for permission'
    case 'question': {
      if (choice.question || choice.title) return choice.question || choice.title || ''
      // Naming one of several questions as the headline would be a lie a reader could act on.
      const asked = choice.questions?.length ?? 0
      return asked > 1 ? `The agent has ${asked} questions` : 'The agent has a question'
    }
    case 'elicitation':
      return choice.question || 'A tool is asking for some details'
    default:
      return choice.question || choice.title || 'The agent is waiting for your answer'
  }
}

/** The line under the headline: what a permission is about to touch (reported
 *  on the tool call it gates, never guessed), or the title the headline left out. */
export function choiceDetail(choice: AgentChoice, toolTarget = ''): string {
  if (choice.kind === 'tool_permission') return choice.toolName ? toolTarget : ''
  const headline = describeChoice(choice)
  return choice.title && choice.title !== headline ? choice.title : ''
}

/** An option with no label is named by its kind, Crowbar's own word for it. */
export function optionLabel(option: AgentChoiceOption): string {
  if (option.label) return option.label
  return option.kind.charAt(0).toUpperCase() + option.kind.slice(1)
}

function pickedOptionLabels(choice: AgentChoice): string[] {
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

/** One line for a choice that is no longer pending: what it was about, then
 *  what happened. Each resolution is a different fact for a reader scanning back. */
export function describeResolvedChoice(choice: AgentChoice): string {
  const subject = choice.toolName || describeChoice(choice)
  if (choice.pending) return `${subject} · waiting for answer`
  if (choice.resolution === 'proceeded') return `${subject} · answered at the terminal`
  if (choice.resolution === 'refused') return `${subject} · refused, no answer in time`
  if (choice.resolution === 'abandoned') return `${subject} · left unanswered`
  const picked = pickedOptionLabels(choice)
  const verb = picked.length > 0 ? picked.join(', ') : 'Answered'
  return `${subject} · ${verb}`
}

/** The newest `limit` characters of live text on one line, emphasis runs
 *  stripped: the newest words say what is happening now. */
export function tailOf(text: string, limit: number): string {
  const flat = text
    .replace(/\*{1,3}/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  if (flat.length <= limit) return flat
  return `…${flat.slice(flat.length - limit)}`
}

/** A duration read at a glance; sub-second work in ms, since "0s" reads as nothing. */
export function formatDuration(ms: number | undefined): string {
  if (!ms || ms < 0) return ''
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  const minutes = Math.floor(ms / 60_000)
  const seconds = Math.round((ms % 60_000) / 1000)
  return seconds === 0 ? `${minutes}m` : `${minutes}m ${seconds}s`
}
