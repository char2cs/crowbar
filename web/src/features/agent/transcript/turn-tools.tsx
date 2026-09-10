import { useState } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentChoice, AgentSubagent, AgentToolCall } from '@/features/agent/api/agent-api'
import {
  describeResolvedChoice,
  describeTool,
  formatDuration,
  tailOf,
} from '@/features/agent/lib/agent-activity'
import { ToolPayloadPanel } from '@/features/agent/transcript/tool-payload-panel'

/** Rows shown under a reply before the rest collapse into a count — shared by
 *  tool calls, subagents and resolved choices, so a turn with a lot going on
 *  never grows one list far past the others. */
const LIMIT = 6

/** A running tool's output is a progress signal, not a log — one line's worth.
 *  The full output is on the call once it finishes. */
const TOOL_OUTPUT_LIMIT = 120

/** Finished tool calls, grouped by turn and sorted by seq within each turn.
 *  Computed once per activity change (see agent-transcript.tsx), not once
 *  per row — the O(n) filter+sort this used to do inside every row's render
 *  was quadratic in conversation length.
 *
 *  A still-running call is left out because it belongs to the turn IN FLIGHT,
 *  which AgentLiveTurnTools draws instead; by the time a turn has a reply to
 *  group under, the ledger has already ended every call it opened. */
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
 * Finished calls only — anything still running belongs to the turn in flight
 * (AgentLiveTurnTools), not to a turn that has already been answered.
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

  return <ToolList calls={calls} testId="agent-turn-tools" wsId={wsId} chatId={chatId} />
}

/**
 * The work of the turn happening RIGHT NOW, as a transcript row of its own.
 *
 * The record and the live view are the same list, drawn by the same component,
 * because they are the same facts: a call lands here the moment the ledger has
 * it, and moves under the reply (AgentTurnTools) the moment that reply exists to
 * hold it. Without this a call was durable in the ledger and invisible on screen
 * for the whole rest of its turn — measured live at 13-26s per call, all of them
 * appearing at once when the turn ended.
 *
 * Wrapped in the same `row`/`assistant` boxes a settled reply uses so the rows do
 * not move when the turn closes and they change parents.
 */
export function AgentLiveTurnTools({
  calls,
  toolOutput,
  wsId,
  chatId,
}: {
  calls: AgentToolCall[]
  /** The running call's newest output, and which call it belongs to. */
  toolOutput?: { id: string; text: string }
  wsId?: string
  chatId?: string
}) {
  if (calls.length === 0) return null
  return (
    <article className="row" data-testid="agent-live-turn">
      <div className="assistant">
        <ToolList
          calls={calls}
          testId="agent-live-turn-tools"
          output={toolOutput}
          wsId={wsId}
          chatId={chatId}
        />
      </div>
    </article>
  )
}

/** The list both of the above draw — one definition of the cap, the ordering and
 *  the row, so the live view and the record can never disagree about a call. */
function ToolList({
  calls,
  testId,
  output,
  wsId,
  chatId,
}: {
  calls: AgentToolCall[]
  testId: string
  output?: { id: string; text: string }
  wsId?: string
  chatId?: string
}) {
  return (
    <ul className="tools" data-testid={testId}>
      {calls.slice(0, LIMIT).map((call) => (
        <ToolRow
          key={call.id}
          call={call}
          output={output?.id === call.id ? output.text : undefined}
          wsId={wsId}
          chatId={chatId}
        />
      ))}
      {calls.length > LIMIT && <li>+{calls.length - LIMIT} more</li>}
    </ul>
  )
}

/** One call's row. Split out from the list because it is the only piece that
 *  ever needs state — WHETHER its own payload is open — and a turn with a
 *  hundred rows must not carry that for every one of them.
 *
 *  The payload is fetched only once actually opened, never on mount: it is
 *  content-addressed and can run to hundreds of KB per turn (see
 *  `AgentToolCall`'s own doc), so a `<details>` that fetched eagerly the
 *  instant its turn scrolled into view would be the opposite of "on demand". */
function ToolRow({
  call,
  output,
  wsId,
  chatId,
}: {
  call: AgentToolCall
  output?: string
  wsId?: string
  chatId?: string
}) {
  const [open, setOpen] = useState(false)
  const running = call.status === 'running'
  const summary = (
    <>
      {/* A row with no duration and no marker reads exactly like a finished
          one, so the one thing a reader needs from a live list — which of
          these is still going — would be the thing it did not say. */}
      {running && <FlickerSpinner className="size-3" />}
      <span>{describeTool(call)}</span>
      {call.error && <span className="err">{call.error}</span>}
      {call.durationMs !== undefined && <span>{formatDuration(call.durationMs)}</span>}
      {running && output && (
        <span className="out" data-testid="agent-tool-output">
          {tailOf(output, TOOL_OUTPUT_LIMIT)}
        </span>
      )}
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
