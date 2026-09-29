import { useState } from 'react'
import { CaretRightIcon } from '@phosphor-icons/react'
import { occurrenceKeys } from '@/features/agent/lib/occurrence-keys'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentChoice, AgentSubagent, AgentToolCall } from '@/features/agent/api/agent-api'
import { TurnDiffPreview } from '@/features/agent/activity/turn-diff-preview'
import { formatElapsed } from '@/features/agent/activity/lib/shelf-fit'
import { describeResolvedChoice, formatDuration, tailOf } from '@/features/agent/lib/agent-activity'
import {
  FetchIcon,
  FileIcon,
  PencilIcon,
  SearchIcon,
  SubagentIcon,
  TerminalIcon,
  ToolIcon,
} from '@/features/agent/shared/agent-icons'
import { ToolPayloadPanel } from '@/features/agent/transcript/tool-payload-panel'

/** A running tool's output is a progress signal, not a log — one line's worth.
 *  The full output is on the call once it finishes. */
const TOOL_OUTPUT_LIMIT = 120

function ToolKindIcon({ kind }: { kind?: string }) {
  if (!kind) return null
  const Icon =
    kind === 'read'
      ? FileIcon
      : kind === 'edit'
        ? PencilIcon
        : kind === 'execute'
          ? TerminalIcon
          : kind === 'search'
            ? SearchIcon
            : kind === 'fetch'
              ? FetchIcon
              : ToolIcon
  return (
    <span className="kind" title={kind} data-tool-kind={kind}>
      <Icon size={12} />
    </span>
  )
}

function humanizeToolName(name: string): string {
  const words = name
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .trim()
    .toLowerCase()
  return words ? `${words[0]?.toUpperCase()}${words.slice(1)}` : ''
}

function toolVerb(call: AgentToolCall): string {
  const running = call.status === 'running'
  switch (call.kind) {
    case 'read':
      return running ? 'Reading' : 'Read'
    case 'edit':
      return running ? 'Editing' : 'Edited'
    case 'execute':
      return running ? 'Running' : 'Ran'
    case 'search':
      return running ? 'Searching' : 'Searched'
    case 'fetch':
      return running ? 'Fetching' : 'Fetched'
    default:
      return humanizeToolName(call.name) || 'Tool'
  }
}

/** Keep absolute provider paths available in a tooltip without letting them
 * dominate a transcript row. The final three segments are enough to identify
 * a source file in practice and still preserve useful directory context. */
function compactPath(path: string): string {
  const normalized = path.replaceAll('\\', '/').replace(/\/+$/, '')
  if (!normalized.startsWith('/')) return normalized
  const parts = normalized.split('/').filter(Boolean)
  return parts.slice(-3).join('/') || path
}

function toolSubject(call: AgentToolCall): { full: string; display: string } | undefined {
  const location = call.locations?.[0]
  if (location) {
    const line = location.line ? `:${location.line}` : ''
    const extra =
      call.locations && call.locations.length > 1 ? ` +${call.locations.length - 1}` : ''
    return {
      full: `${location.path}${line}${extra}`,
      display: `${compactPath(location.path)}${line}${extra}`,
    }
  }
  if (!call.target) return undefined
  const target = call.target.trim()
  const pathLike =
    (call.kind === 'read' || call.kind === 'edit' || call.kind === 'search') &&
    target.startsWith('/') &&
    !target.includes('\n')
  return { full: target, display: pathLike ? compactPath(target) : target }
}

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
 *  span, since nothing else ever needed one. Exported for NestedSubagentPanel,
 *  which needs the identical span for a subagent with no turn to group under. */
export function elapsedMs(startedAt: string, endedAt: string): number {
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

/** One stable activity row per tool invocation in the virtualized transcript. */
export function AgentToolCallEntry({
  call,
  output,
  diff,
  wsId,
  chatId,
  parentId,
}: {
  call: AgentToolCall
  output?: string
  diff?: string
  wsId?: string
  chatId?: string
  parentId?: string
}) {
  return (
    <article
      className="row activity-tool-row"
      data-testid="agent-activity-tool"
      data-tool-id={call.id}
      data-parent-id={parentId}
      data-nested={call.subagentId || (parentId && parentId !== call.turnId) ? '' : undefined}
    >
      <div className="assistant">
        <ul className="tools">
          <ToolRow call={call} output={output} diff={diff} wsId={wsId} chatId={chatId} />
        </ul>
      </div>
    </article>
  )
}

/** A subagent is a chronological row; its own transcript stays nested inside it. */
export function AgentSubagentEntry({
  subagent,
  parentId,
}: {
  subagent: AgentSubagent
  parentId?: string
}) {
  const running = !subagent.endedAt
  const messageKeys = occurrenceKeys(
    subagent.messages ?? [],
    (message) => `${message.at}:${message.text}`,
  )
  const elapsed = subagent.endedAt
    ? Math.max(0, Math.round(elapsedMs(subagent.startedAt, subagent.endedAt) / 1000))
    : Math.max(0, Math.round((Date.now() - Date.parse(subagent.startedAt)) / 1000))
  return (
    <article
      className="row"
      data-testid="agent-activity-subagent"
      data-subagent-id={subagent.id}
      data-parent-id={parentId}
      data-nested={parentId && parentId !== subagent.turnId ? '' : undefined}
      data-status={running ? 'running' : 'completed'}
    >
      <div className="assistant">
        <div className="subbar">
          <span className="subhd">
            <SubagentIcon size={12} />
            <b>{subagent.agentType || 'Subagent'}</b>
          </span>
          <span className="subline">
            <span className={`tok${running ? '' : ' done'}`}>
              <i />
              <b>{formatElapsed(elapsed)}</b>
            </span>
          </span>
        </div>
        {subagent.messages && subagent.messages.length > 0 && (
          <details className="subagent-transcript">
            <summary>Show subagent transcript ({subagent.messages.length})</summary>
            <div>
              {subagent.messages.map((message, index) => (
                <p key={messageKeys[index]}>{message.text}</p>
              ))}
            </div>
          </details>
        )}
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
      {calls.map((call) => (
        <ToolRow
          key={call.id}
          call={call}
          output={output?.id === call.id ? output.text : undefined}
          wsId={wsId}
          chatId={chatId}
        />
      ))}
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
  diff,
  wsId,
  chatId,
}: {
  call: AgentToolCall
  output?: string
  diff?: string
  wsId?: string
  chatId?: string
}) {
  const [open, setOpen] = useState(false)
  const running = call.status === 'running'
  const subject = toolSubject(call)
  const summary = (
    <span className="tool-call-summary">
      {/* A row with no duration and no marker reads exactly like a finished
          one, so the one thing a reader needs from a live list — which of
          these is still going — would be the thing it did not say. */}
      {running && <FlickerSpinner className="size-3" />}
      <ToolKindIcon kind={call.kind} />
      <span className="tool-call-verb">{toolVerb(call)}</span>
      {subject && (
        <span
          className="tool-call-subject"
          data-tool-location={call.locations?.length ? '' : undefined}
          title={subject.full}
        >
          {subject.display}
        </span>
      )}
      {call.error && <span className="err">{call.error}</span>}
      {call.durationMs !== undefined && (
        <span className="tool-call-duration">{formatDuration(call.durationMs)}</span>
      )}
      {running && output && (
        <span className="out" data-testid="agent-tool-output">
          {tailOf(output, TOOL_OUTPUT_LIMIT)}
        </span>
      )}
    </span>
  )
  const expandable = wsId && chatId && (call.hasRequest || call.hasResult)
  const patch = call.diff || diff
  const diffPreview = call.kind === 'edit' && patch && (
    <div className="turn-diff tool-turn-diff" data-testid="agent-tool-diff">
      <TurnDiffPreview diff={patch} turnId={call.id} wsId={wsId} />
    </div>
  )
  if (!expandable)
    return (
      <li data-status={call.status}>
        {summary}
        {diffPreview}
      </li>
    )
  return (
    <li data-status={call.status}>
      <div className="tool-row">
        {summary}
        <button
          type="button"
          className="tool-row-toggle"
          aria-label="Show tool details"
          aria-expanded={open}
          onClick={() => setOpen((o) => !o)}
        >
          <CaretRightIcon size={12} />
        </button>
      </div>
      {open && (
        <ToolPayloadPanel
          wsId={wsId}
          chatId={chatId}
          toolId={call.id}
          hasRequest={call.hasRequest}
          hasResult={call.hasResult}
        />
      )}
      {diffPreview}
    </li>
  )
}

/** Ended subagents this turn ran, under the reply — the counterpart of
 *  `AgentTurnTools` for the OTHER kind of work a turn does. Without it a
 *  subagent that finished had no record anywhere once the live shelf stopped
 *  counting it: it simply stopped existing.
 *
 *  Same `.subbar`/`.subhd`/`.tok` chip the live shelf (SubagentShelf) draws
 *  for a RUNNING subagent — a finished one is the same fact, just no longer
 *  moving, not a different kind of thing that needs its own look. `.tok done`
 *  freezes the dot instead of pulsing it: pulsing reads as "still working",
 *  which a finished row would be lying about. */
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
    <div className="subbar" data-testid="agent-turn-subagents">
      <span className="subhd">
        <SubagentIcon size={12} />
        <b>{subagents.length}</b>&nbsp;{subagents.length === 1 ? 'subagent' : 'subagents'}
      </span>
      <span className="subline">
        {subagents.map((subagent) => (
          <span className="tok done" key={subagent.id}>
            <i />
            {subagent.agentType && <span className="ty">{subagent.agentType}</span>}
            <b>
              {formatElapsed(
                Math.round(elapsedMs(subagent.startedAt, subagent.endedAt as string) / 1000),
              )}
            </b>
          </span>
        ))}
      </span>
    </div>
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
      {choices.map((choice) => (
        <li key={choice.id} data-resolution={choice.resolution}>
          <span>{describeResolvedChoice(choice)}</span>
        </li>
      ))}
    </ul>
  )
}
