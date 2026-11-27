import type {
  AgentChoice,
  AgentChoiceOption,
  AgentChoiceQuestion,
} from '@/features/agent/api/agent-chat-activity'
import type { ToolFamily } from '@/features/agent/transcript/lib/tool-family'

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

const ASKED: Partial<Record<ToolFamily, string>> = {
  execute: 'Run a command',
  edit: 'Edit a file',
  read: 'Read a file',
  fetch: 'Fetch a web page',
  search: 'Search',
  plan: 'Approve this plan',
  mcp: 'Use a tool',
}

export function describeChoice(choice: AgentChoice, family?: ToolFamily): string {
  switch (choice.kind) {
    case 'tool_permission':
      if (family && ASKED[family]) return ASKED[family]
      return choice.toolName ? `Run ${choice.toolName}` : 'The agent is asking for permission'
    case 'question': {
      if (choice.question || choice.title) return choice.question || choice.title || ''
      const asked = choice.questions?.length ?? 0
      return asked > 1 ? `The agent has ${asked} questions` : 'The agent has a question'
    }
    case 'elicitation':
      return choice.question || 'A tool is asking for some details'
    default:
      return choice.question || choice.title || 'The agent is waiting for your answer'
  }
}

export function choiceDetail(choice: AgentChoice, toolTarget = ''): string {
  if (choice.kind === 'tool_permission') return choice.toolName ? toolTarget : ''
  const headline = describeChoice(choice)
  return choice.title && choice.title !== headline ? choice.title : ''
}

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

export function askNote(choice: AgentChoice): string {
  if (choice.resolution === 'proceeded') return 'Allowed in terminal'
  if (choice.resolution === 'refused') return 'Timed out'
  if (choice.resolution === 'abandoned') return 'Not answered'
  const ids = new Set(choice.answeredOptionIds)
  const picked = choice.options.filter((option) => ids.has(option.id))
  if (picked.some((option) => option.kind === 'deny')) return 'Denied'
  const allowed = picked.find((option) => option.kind === 'allow')
  const extra = allowed?.label && !/^allow( once)?$/i.test(allowed.label) ? allowed.label : ''
  if (allowed) return extra ? `Allowed · ${extra}` : 'Allowed'
  const labels = pickedOptionLabels(choice)
  return labels.length > 0 ? labels.join(', ') : 'Answered'
}
