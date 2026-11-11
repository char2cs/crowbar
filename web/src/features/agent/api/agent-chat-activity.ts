// The decision a chat is waiting on, as the composer and the answer API use it.

/** One answer a prompt accepts. Rendered from `kind` (Crowbar's own words for
 *  allow/deny); a plain string so a kind the daemon adds degrades to "an option". */
export interface AgentChoiceOption {
  id: string
  kind: string
  label?: string
  description?: string
  /** Answering with it sends the person's reason to the agent. */
  reason?: boolean
}

/** One question inside a prompt. `multi` is per question because the provider
 *  says so; option ids are unique across the whole prompt, since an answer
 *  names its picks in one flat list. */
export interface AgentChoiceQuestion {
  id: string
  title?: string
  text?: string
  multi?: boolean
  options: AgentChoiceOption[]
}

/** The agent waiting on a person (permission, question, elicitation). `pending`: the CLI still
 *  asks; `answerable`: an answer from here reaches it. Pending + unanswerable: no buttons. */
export interface AgentChoice {
  id: string
  turnId: string
  seq: number
  /** `tool_permission` | `question` | `elicitation`, open like an option's kind. */
  kind: string
  toolName?: string
  title?: string
  question?: string
  /** The provider's own word for how it expects an answer, never interpreted. */
  mode?: string
  multi?: boolean
  options: AgentChoiceOption[]
  /** Absent (not empty) for a record from before questions were modelled: then
   *  `question` and `options` are the one question. A partial answer is refused. */
  questions?: AgentChoiceQuestion[]
  /** An elicitation's input schema, the provider's JSON verbatim: answered with a form. */
  schema?: string
  pending: boolean
  answerable: boolean
  at: string
  resolvedAt?: string
  /** `answered`, `proceeded` (at the terminal), `refused` (no answer in time), `abandoned`. */
  resolution?: string
  autoApproved?: boolean
  /** The picks of an ask answered through Crowbar; absent for any other resolution. */
  answeredOptionIds?: string[]
}

// `answerable` defaults false: buttons for a gate nobody holds would reach nobody.
export function mapChoice(c: AgentChoice): AgentChoice {
  return {
    ...c,
    multi: c.multi ?? false,
    options: c.options ?? [],
    pending: c.pending ?? false,
    answerable: c.answerable ?? false,
  }
}
