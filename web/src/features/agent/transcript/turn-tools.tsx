import { useState } from 'react'
import type { AgentChoice, AgentSubagent, AgentToolCall } from '@/features/agent/api/agent-api'
import {
  describeResolvedChoice,
  describeTool,
  formatDuration,
} from '@/features/agent/lib/agent-activity'
import { ToolPayloadPanel } from '@/features/agent/transcript/tool-payload-panel'

/** Rows shown under a reply before the rest collapse into a count — shared by
 *  tool calls, subagents and resolved choices, so a turn with a lot going on
 *  never grows one list far past the others. */
const LIMIT = 6

/** Finished tool calls, grouped by turn and sorted by seq within each turn.
 *  Computed once per activity change (see agent-transcript.tsx), not once
 *  per row — the O(n) filter+sort this used to do inside every row's render
 *  was quadratic in conversation length. */
export function groupToolCallsByTurn(toolCalls: AgentToolCall[]): Map<string, AgentToolCall[]> {
  const byTurn = new Map<string, AgentToolCall[]>()
  for (const call of toolCalls) {
    if (call.status === 'running') continue
    const list = byTurn.get(call.turnId)
    if (list) list.push(call)
    else byTurn.set(call.turnId, [call])
  }
  for (const list of byTurn.values()) {
    list.sort((a, b) => a.seq - b.seq)
  }
  return byTurn
}

/** Ended subagents, grouped by turn — the counterpart of `groupToolCallsByTurn`.
 *  A running one belongs to the live shelf (subagent-shelf.tsx), not here: this
 *  is the record of a turn already answered. */
export function groupSubagentsByTurn(subagents: AgentSubagent[]): Map<string, AgentSubagent[]> {
  const byTurn = new Map<string, AgentSubagent[]>()
  for (const subagent of subagents) {
    if (!subagent.endedAt) continue
    const list = byTurn.get(subagent.turnId)
    if (list) list.push(subagent)
    else byTurn.set(subagent.turnId, [subagent])
  }
  for (const list of byTurn.values()) {
    list.sort((a, b) => a.seq - b.seq)
  }
  return byTurn
}

/** Resolved choices, grouped by turn — the counterpart of `groupToolCallsByTurn`
 *  for a permission, question or elicitation that stopped being pending. A
 *  still-pending one belongs to the composer, not here. */
export function groupChoicesByTurn(choices: AgentChoice[]): Map<string, AgentChoice[]> {
  const byTurn = new Map<string, AgentChoice[]>()
  for (const choice of choices) {
    if (choice.pending) continue
    const list = byTurn.get(choice.turnId)
    if (list) list.push(choice)
    else byTurn.set(choice.turnId, [choice])
  }
  for (const list of byTurn.values()) {
    list.sort((a, b) => a.seq - b.seq)
  }
  return byTurn
}

/** Duration between two timestamps, the same shape `AgentToolCall.durationMs`
 *  already reports — subagents carry only the two timestamps, not a precomputed
 *  span, since nothing else ever needed one. */
function elapsedMs(startedAt: string, endedAt: string): number {
  return Date.parse(endedAt) - Date.parse(startedAt)
}

/**
 * What the agent DID to produce a reply, under the reply.
 *
 * Finished calls only — anything still running belongs to the working line, not
 * to a turn that has already been answered.
 *
 * `wsId`/`chatId` are optional and gate ONE thing: whether a finished call's own
 * request/result bytes can be opened in place. Omitting them (every caller before
 * this existed, and every test) degrades to exactly the old plain row — the
 * payload was always fetchable from the API, just never reachable from here.
 */
export function AgentTurnTools({
  callsByTurn,
  turnId,
  wsId,
  chatId,
}: {
  callsByTurn: Map<string, AgentToolCall[]>
  turnId: string
  wsId?: string
  chatId?: string
}) {
  if (!turnId) return null
  const calls = callsByTurn.get(turnId) ?? []
  if (calls.length === 0) return null

  return (
    <ul className="tools" data-testid="agent-turn-tools">
      {calls.slice(0, LIMIT).map((call) => (
        <ToolRow key={call.id} call={call} wsId={wsId} chatId={chatId} />
      ))}
      {calls.length > LIMIT && <li>+{calls.length - LIMIT} more</li>}
    </ul>
  )
}

/** One finished call's row. Split out from `AgentTurnTools` because it is the
 *  only piece that ever needs state — WHETHER its own payload is open — and a
 *  turn with a hundred rows must not carry that for every one of them.
 *
 *  The payload is fetched only once actually opened, never on mount: it is
 *  content-addressed and can run to hundreds of KB per turn (see
 *  `AgentToolCall`'s own doc), so a `<details>` that fetched eagerly the
 *  instant its turn scrolled into view would be the opposite of "on demand". */
function ToolRow({
  call,
  wsId,
  chatId,
}: {
  call: AgentToolCall
  wsId?: string
  chatId?: string
}) {
  const [open, setOpen] = useState(false)
  const summary = (
    <>
      <span>{describeTool(call)}</span>
      {call.error && <span className="err">{call.error}</span>}
      {call.durationMs !== undefined && <span>{formatDuration(call.durationMs)}</span>}
    </>
  )
  const expandable = wsId && chatId && (call.hasRequest || call.hasResult)
  if (!expandable) {
    return <li data-status={call.status}>{summary}</li>
  }
  return (
    <li data-status={call.status}>
      <details open={open}>
        {/* preventDefault + explicit state, not the browser's own toggle: the
            fetch below must fire exactly once, on the click that actually
            opens this row, never on every re-render. */}
        <summary
          onClick={(e) => {
            e.preventDefault()
            setOpen((o) => !o)
          }}
        >
          {summary}
        </summary>
        {open && (
          <ToolPayloadPanel
            wsId={wsId}
            chatId={chatId}
            toolId={call.id}
            hasRequest={call.hasRequest}
            hasResult={call.hasResult}
          />
        )}
      </details>
    </li>
  )
}

/** Ended subagents this turn ran, under the reply — the counterpart of
 *  `AgentTurnTools` for the OTHER kind of work a turn does. Without it a
 *  subagent that finished had no record anywhere once the live shelf stopped
 *  counting it: it simply stopped existing. */
export function AgentTurnSubagents({
  subagentsByTurn,
  turnId,
}: {
  subagentsByTurn: Map<string, AgentSubagent[]>
  turnId: string
}) {
  if (!turnId) return null
  const subagents = subagentsByTurn.get(turnId) ?? []
  if (subagents.length === 0) return null

  return (
    <ul className="tools" data-testid="agent-turn-subagents">
      {subagents.slice(0, LIMIT).map((subagent) => (
        <li key={subagent.id}>
          <span>{subagent.agentType ? `Subagent · ${subagent.agentType}` : 'Subagent'}</span>
          <span>{formatDuration(elapsedMs(subagent.startedAt, subagent.endedAt as string))}</span>
        </li>
      ))}
      {subagents.length > LIMIT && <li>+{subagents.length - LIMIT} more</li>}
    </ul>
  )
}

/** Resolved choices this turn opened, under the reply — the transcript's only
 *  record of a permission, question or elicitation once it stops blocking
 *  anyone. Without it, a person who approved something ten messages back has
 *  no way to see it happened, let alone what they chose. */
export function AgentTurnChoices({
  choicesByTurn,
  turnId,
}: {
  choicesByTurn: Map<string, AgentChoice[]>
  turnId: string
}) {
  if (!turnId) return null
  const choices = choicesByTurn.get(turnId) ?? []
  if (choices.length === 0) return null

  return (
    <ul className="tools" data-testid="agent-turn-choices">
      {choices.slice(0, LIMIT).map((choice) => (
        <li key={choice.id} data-resolution={choice.resolution}>
          <span>{describeResolvedChoice(choice)}</span>
        </li>
      ))}
      {choices.length > LIMIT && <li>+{choices.length - LIMIT} more</li>}
    </ul>
  )
}
