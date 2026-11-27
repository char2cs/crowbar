export interface AgentChoiceOption {
  id: string
  kind: string
  label?: string
  description?: string
}

export interface AgentChoiceQuestion {
  id: string
  title?: string
  text?: string
  multi?: boolean
  options: AgentChoiceOption[]
}

export interface AgentChoice {
  id: string
  turnId: string
  seq: number
  kind: string
  toolName?: string
  title?: string
  question?: string
  mode?: string
  multi?: boolean
  options: AgentChoiceOption[]
  questions?: AgentChoiceQuestion[]
  schema?: string
  pending: boolean
  answerable: boolean
  at: string
  resolvedAt?: string
  resolution?: string
  autoApproved?: boolean
  answeredOptionIds?: string[]
}

export function mapChoice(c: AgentChoice): AgentChoice {
  return {
    ...c,
    multi: c.multi ?? false,
    options: c.options ?? [],
    pending: c.pending ?? false,
    answerable: c.answerable ?? false,
  }
}
