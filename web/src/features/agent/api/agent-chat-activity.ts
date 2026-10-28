import { API_BASE, apiFetch } from '@/lib/api/fetch'
import { chatBase } from '@/features/agent/api/agent-chat-reads'
import {
  activityComponents,
  type ActivityComponent,
} from '@/features/agent/lib/activity-components'

type ToolCallStatus = 'running' | 'ok' | 'error' | 'declined' | 'abandoned'

/** One tool the agent ran. The payloads are NOT here: a coding agent produces
 *  hundreds of KB per turn and almost none of it is ever opened, so each side is
 *  fetched on demand by `getToolPayload`. */
export interface AgentToolCall {
  id: string
  turnId: string
  seq: number
  name: string
  kind?: 'read' | 'edit' | 'execute' | 'search' | 'fetch' | 'other' | string
  locations?: Array<{ path: string; line?: number }>
  /** The file, command or URL the tool acted on, when the provider reports one.
   *  Absent is legible; a guess would be wrong. */
  target?: string
  status: ToolCallStatus
  /** A short caption for a FAILED call — absent on every other status. The full
   *  failure text is the result payload, fetched on demand like any other side
   *  (see `getToolPayload`); this is the one line worth showing without asking. */
  error?: string
  durationMs?: number
  hasRequest: boolean
  hasResult: boolean
  /** Unified diff of this edit call's own changes, when the provider reports
   *  file diffs, patch hunks, or replaced and replacement text. */
  diff?: string
  startedAt: string
  endedAt?: string
  /** Set instead of a meaningful `turnId` when this call belongs to a
   *  SUBAGENT's own nested activity (its id) rather than the chat's own
   *  top-level turn — see `AgentSubagent`. */
  subagentId?: string
}

/** One closed turn of a subagent's own nested conversation. */
interface AgentSubagentMessage {
  text: string
  at: string
}

export interface AgentSubagent {
  id: string
  turnId: string
  seq: number
  agentType?: string
  startedAt: string
  endedAt?: string
  /** The subagent's own reply history, in arrival order — present only for a
   *  provider whose subagent is a whole nested conversation with its own
   *  tool calls and turns (see `AgentToolCall.subagentId`), not the flat
   *  marker a native subagent (Claude's Task tool) still is. */
  messages?: AgentSubagentMessage[]
}

type InterruptionKind =
  | 'permission'
  | 'notification'
  | 'elicitation'
  | 'compaction'
  | 'stopped'
  | 'provider_switched'
  | 'model_changed'
  | 'effort_changed'
  /** Crowbar's own guess, not a person's Stop click or a provider report: a
   *  turn's assistant reply went quiet with nothing open to explain it, and
   *  the daemon closed it on a timeout. See turn.AbandonMessageInferredInterrupt. */
  | 'inferred'

/** The agent blocked on, or interrupted by, something outside the turn. These
 *  are what make an apparently frozen agent legible. */
export interface AgentInterruption {
  id: string
  turnId: string
  seq: number
  /** Inherits the turn it was opened against — see AgentChatMessage.displayOrder
   *  and domain.ActivityInterruption. Falls back to `seq` when absent. */
  displayOrder?: number
  kind: InterruptionKind
  detail?: string
  at: string
  resolvedAt?: string
}

/** One answer a prompt will accept.
 *
 *  A client renders from `kind`, not from `label`: allow and deny are Crowbar's
 *  OWN words, because no provider enumerates the two answers a permission has by
 *  construction. It is a plain string rather than a union so a vocabulary the
 *  backend grows later degrades to "an option" instead of failing to compile —
 *  the four in use today are `answer`, `allow`, `deny` and `suggestion`. */
export interface AgentChoiceOption {
  id: string
  kind: string
  label?: string
  description?: string
}

/** ONE question inside a prompt, with the options that answer it.
 *
 *  `multi` is per QUESTION and not per prompt, because the provider says so:
 *  claude carries multiSelect on each entry of its questions array, so one prompt
 *  can ask "pick one" and "pick any" in the same card.
 *
 *  Option ids are unique across the WHOLE prompt, not merely within this question,
 *  because an answer names its picks in one flat list. */
export interface AgentChoiceQuestion {
  id: string
  title?: string
  text?: string
  multi?: boolean
  options: AgentChoiceOption[]
}

/**
 * The agent waiting on a HUMAN DECISION — a tool permission, a question it asked,
 * an MCP server's elicitation.
 *
 * `pending` and `answerable` are two DIFFERENT facts and both matter. `pending`
 * says the CLI is still asking. `answerable` says a relay is holding that CLI's
 * gate open right now, so an answer sent from here will actually reach it. A
 * prompt that is pending and NOT answerable is still worth drawing — the CLI is
 * genuinely asking, at its own terminal — but it must offer no buttons, because
 * they would reach nobody.
 */
export interface AgentChoice {
  id: string
  turnId: string
  seq: number
  /** `tool_permission` | `question` | `elicitation`, open for the same reason
   *  an option's kind is. */
  kind: string
  /** The tool a permission gates, when there is one. */
  toolName?: string
  title?: string
  question?: string
  /** The provider's own word for how it expects to be answered, carried verbatim
   *  and never interpreted here either. */
  mode?: string
  /** Describes `options`. A question carries its own `multi`. */
  multi?: boolean
  options: AgentChoiceOption[]
  /** The whole of what a question-kind prompt is asking, one entry per question.
   *
   *  ABSENT is meaningful and is NOT the same as empty: it says the record predates
   *  questions being modelled, and such a prompt is a single question described by
   *  `question` and `options` — render those instead of an empty card.
   *
   *  An answer covering only SOME of these is refused with 400, and that refusal is
   *  the point: a partial answer hands the CLI an input covering part of what it
   *  asked, and it goes on waiting for the rest with nothing able to send it. */
  questions?: AgentChoiceQuestion[]
  /** An elicitation's requested-input schema: the provider's own JSON, verbatim.
   *  A prompt with a schema and no options is answered with a FORM. */
  schema?: string
  pending: boolean
  answerable: boolean
  at: string
  resolvedAt?: string
  /** How it stopped pending: `answered` here, `proceeded` at the terminal,
   *  `refused` for want of an answer in time, or `abandoned` with the turn. */
  resolution?: string
  /** Who answered it when `resolution` is `answered`: policy (`true`) or a
   *  human's own click (`false`). */
  autoApproved?: boolean
  /** Which of `options` (or a question's own options) was actually picked, when
   *  `resolution` is `answered` through Crowbar. Absent for one that proceeded
   *  at the provider's own terminal or was abandoned with its turn — those
   *  genuinely have no such answer to report, not merely an unrecorded one. */
  answeredOptionIds?: string[]
}

export interface AgentActivity {
  toolCalls: AgentToolCall[]
  subagents: AgentSubagent[]
  interruptions: AgentInterruption[]
  /** The prompts this chat has opened. They ride the activity payload rather than
   *  a poll of their own: a blocked agent is a state of the same turn the timeline
   *  describes, and a second loop would ask the same daemon the same question. */
  choices: AgentChoice[]
  /** Stable ordered projection used by rich activity renderers. Older daemons
   * may omit this because it is derived client-side from the existing ledger. */
  components?: ActivityComponent[]
}

export interface ListActivityOptions {
  after?: number
  limit?: number
  signal?: AbortSignal
}

/** Read what the agent DID — tool calls, subagents, interruptions — as distinct
 *  from what it said. Separate from the messages because the conversation is read
 *  on every render and this is read when a timeline is opened. */
export async function listChatActivity(
  wsId: string,
  id: string,
  options: ListActivityOptions = {},
): Promise<AgentActivity> {
  const query = new URLSearchParams()
  if (options.after !== undefined) query.set('after', String(options.after))
  if (options.limit !== undefined) query.set('limit', String(options.limit))
  const raw = await apiFetch<AgentActivity>(
    `${chatBase(wsId)}/${encodeURIComponent(id)}/activity?${query}`,
    { signal: options.signal },
  )
  const activity: AgentActivity = {
    toolCalls: raw?.toolCalls ?? [],
    subagents: raw?.subagents ?? [],
    interruptions: raw?.interruptions ?? [],
    choices: (raw?.choices ?? []).map(mapChoice),
  }
  return { ...activity, components: raw?.components ?? activityComponents(activity) }
}

// A prompt's three decision-bearing fields are grounded here so nothing
// downstream has to treat an absent key and a false one as two different things.
// `answerable` in particular defaults FALSE: a daemon that does not send it is
// one with no answer channel at all, and defaulting it true would draw buttons
// that reach nobody — the single failure this field exists to prevent.
export function mapChoice(c: AgentChoice): AgentChoice {
  return {
    ...c,
    multi: c.multi ?? false,
    options: c.options ?? [],
    pending: c.pending ?? false,
    answerable: c.answerable ?? false,
  }
}

/** Fetch one tool call's arguments or output. Returns null when retention has
 *  swept it, which is an ordinary outcome to render rather than an error. */
export async function getToolPayload(
  wsId: string,
  id: string,
  toolId: string,
  side: 'request' | 'result',
  signal?: AbortSignal,
): Promise<string | null> {
  const url =
    `${chatBase(wsId)}/${encodeURIComponent(id)}` +
    `/activity/${encodeURIComponent(toolId)}/payload?side=${side}`
  const response = await fetch(`${API_BASE}${url}`, { signal })
  if (response.status === 404) return null
  if (!response.ok) throw new Error(`tool payload: ${response.status}`)
  return response.text()
}
