import { useEffect, useState } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { AgentActivity } from '@/features/agent/api/agent-api'
import {
  blockedOn,
  blocksOnAPerson,
  describeInterruption,
  describeTool,
  pendingChoices,
  runningTools,
} from '@/features/agent/lib/agent-activity'
import { formatElapsed } from '@/features/agent/activity/lib/shelf-fit'
import { VERB_ROTATION_MS, verbAt } from '@/features/agent/activity/lib/verbs'

/** Tool rows shown before the rest collapse into a count. */
const TOOL_LIMIT = 3

interface WorkingLineProps {
  activity: AgentActivity
  working: boolean
  /** When the current turn started, for the elapsed clock. */
  since?: string
  /**
   * Is this chat LIVE mid-compaction right now — from the direct WS push
   * (use-workspace-agent-chats-stream.ts), never from `activity`.
   *
   * The ledger's own interruption record for a compaction is born already
   * resolved (a bare /compact prompt never opens a tracked turn, so
   * commands.Interrupt's idle-chat handling resolves it in the SAME event
   * that creates it), so `blockedOn(activity)` can never observe an open
   * compaction window — reading it for this would be permanently dead code.
   * This prop is the only authoritative "is it happening right now" answer.
   */
  compactingLive?: boolean
  /**
   * What the agent is THINKING right now, if it is telling us.
   *
   * This is the only thing on screen during the long stretch a reasoning model
   * spends before it says anything at all — codex emits nothing but reasoning
   * deltas for most of a hard turn — so without it the whole of that reads as a
   * frozen chat with a spinner on it.
   *
   * Live-only and never recorded (see the backend's turn/reasoning.go and the
   * store's streamingReasoning), which is why it renders HERE, in the
   * what-is-happening-now strip, and not as a transcript row.
   */
  reasoning?: string
  /**
   * The output of the tool that is running right now, and which tool it belongs
   * to. A long build or test run emits nothing else for its whole duration, so
   * without it the tool row sits static with no sign of progress.
   */
  toolOutput?: { id: string; text: string }
  /**
   * The agent's own to-do list for this turn, newest state wholesale.
   *
   * Statuses are CROWBAR'S words — pending / active / done — already translated
   * from the provider's own vocabulary by its descriptor, so nothing here knows
   * how any CLI spells "in progress". An unrecognised status still renders, just
   * unstyled: silently dropping a step would shorten the plan.
   */
  plan?: { text: string; status: string }[]
}

/** How much of the current thought to show. It is a status line, not a document:
 *  the point is to prove the agent is alive and say roughly what it is chewing
 *  on, and an unbounded block would push the transcript around on every token. */
const REASONING_LIMIT = 240

/** Tool output is a progress signal, not a log — one line's worth. The full
 *  output is on the completed tool call. */
const TOOL_OUTPUT_LIMIT = 120

function tailOf(text: string, limit: number): string {
  // Providers head each thinking block with a markdown-bold title (codex emits
  // `**Clarifying ambiguous wording**`). This is one muted italic line, not a
  // rendered document — the markers would read as literal asterisks, and running
  // a markdown pipeline over text that is replaced on every token is not worth
  // the cost. Strip the emphasis runs and collapse the whitespace.
  const flat = text
    .replace(/\*{1,3}/g, '')
    .replace(/\s+/g, ' ')
    .trim()
  if (flat.length <= limit) return flat
  // The TAIL, not the head: the newest thought is the one that says what it is
  // doing now.
  return `…${flat.slice(flat.length - limit)}`
}

/**
 * What the agent is doing, while it is doing it.
 *
 * The verb is CROWBAR'S — no provider reports one, so it is flavour, and it must
 * never wear a provider's name. What is real beside it is the elapsed clock and
 * the running tool calls, both of which come straight off the activity feed.
 *
 * It goes quiet the moment the chat is blocked on a person: a chat waiting for
 * an answer is not working, and saying otherwise is how a blocked agent came to
 * look busy.
 */
export function WorkingLine({
  activity,
  working,
  since,
  compactingLive,
  reasoning,
  toolOutput,
  plan,
}: WorkingLineProps) {
  const [tick, setTick] = useState(0)
  const [elapsed, setElapsed] = useState(0)

  // Compaction is the CLI's own housekeeping, not a wait on a PERSON, and a
  // specific, known operation rather than the vague busywork the rotating
  // verb list exists for. Fixed at "Compacting" instead.
  //
  // Sourced from the live push, NEVER from `activity`: a compaction's ledger
  // record is born already resolved (see this prop's own doc comment), so
  // `blockedOn(activity)?.kind === 'compaction'` can never be true — it would
  // be permanently dead code, not a fallback.
  const interruption = blockedOn(activity)
  const compacting = compactingLive === true

  useEffect(() => {
    if (!working || compacting) {
      setTick(0)
      return
    }
    const timer = window.setInterval(() => setTick((n) => n + 1), VERB_ROTATION_MS)
    return () => window.clearInterval(timer)
  }, [working, compacting])

  useEffect(() => {
    if (!working || !since) {
      setElapsed(0)
      return
    }
    const startedAt = Date.parse(since)
    if (Number.isNaN(startedAt)) return
    const read = () => setElapsed(Math.max(0, Math.round((Date.now() - startedAt) / 1000)))
    read()
    const timer = window.setInterval(read, 1000)
    return () => window.clearInterval(timer)
  }, [working, since])

  // A CHAT WAITING ON A PERSON IS NOT WORKING, so it must never wear the spinner
  // and the rotating verb — that is exactly how a blocked agent came to look busy.
  //
  // But going SILENT for it was the opposite failure, and a worse one. The
  // composer only ever renders pendingChoices[0], and toDividerTag draws nothing
  // at all for the permission, notification and elicitation kinds, so a block with
  // no pending choice behind it — a prompt that never opened durably, a second
  // simultaneous prompt, an interruption the backend never resolved — erased the
  // spinner and put nothing in its place. The chat simply looked dead, with no
  // explanation anywhere on screen. That is the "we lose track of whether it is
  // working" report.
  //
  // So: quiet only when something ELSE is already speaking for the block, which is
  // what a pending choice in the composer means. Otherwise say what is being
  // waited on, without claiming to be working.
  const blockedByPerson = !compacting && blocksOnAPerson(interruption)
  const answeringInTheBar = pendingChoices(activity).length > 0
  // `working` is turn-dispatch state and stays FALSE for a /compact — it never
  // opens a tracked turn (see compactingLive's own doc comment) — so gating
  // on `working` alone would hide this branch for the exact case it exists
  // to cover. Compacting is its own "there is something to show" condition,
  // independent of whether a turn happens to be open too.
  if ((!working && !compacting) || answeringInTheBar) return null

  // Not a turn in flight: no spinner, no verb, no elapsed clock. Just the reason.
  if (blockedByPerson && interruption) {
    return (
      <div className="activity" data-testid="agent-activity-strip" data-blocked="true">
        <div className="hd">
          <span className="dim">{describeInterruption(interruption)}</span>
        </div>
      </div>
    )
  }

  // Compaction isn't a tool call — nothing to enumerate under it.
  const tools = compacting ? [] : runningTools(activity)

  return (
    <div className="activity" data-testid="agent-activity-strip">
      <div className="hd">
        <FlickerSpinner className="size-4" />
        <span>
          <b className="verb">{compacting ? 'Compacting' : verbAt(tick)}…</b>
          {elapsed > 0 && <span className="dim"> · {formatElapsed(elapsed)}</span>}
        </span>
      </div>
      {reasoning && !compacting && (
        <p className="thinking" data-testid="agent-reasoning">
          {tailOf(reasoning, REASONING_LIMIT)}
        </p>
      )}
      {plan && plan.length > 0 && !compacting && (
        <ol className="plan" data-testid="agent-plan">
          {/* PlanStep (Go's domain.PlanStep) has no id, only text/status, so
              there is no natural stable identity to key on. The index is
              combined WITH text, not used alone: a genuinely reordered/
              changed step still gets a fresh key (forcing the correct
              re-render), and two steps sharing identical text — the only
              case this degrades to a plain index key — is not a real shape
              for a task list. */}
          {plan.map((step, i) => (
            // react-doctor-disable-next-line no-array-index-as-key -- see comment above, index is combined with text
            <li key={`${i}-${step.text}`} data-status={step.status}>
              {step.text}
            </li>
          ))}
        </ol>
      )}
      {tools.length > 0 && (
        <ul>
          {tools.slice(0, TOOL_LIMIT).map((call) => (
            <li key={call.id}>
              {describeTool(call)}
              {/* Only under the row it belongs to: a build's output under the
                  build, never under whatever else happens to be running. */}
              {toolOutput?.id === call.id && toolOutput.text && (
                <span className="out" data-testid="agent-tool-output">
                  {tailOf(toolOutput.text, TOOL_OUTPUT_LIMIT)}
                </span>
              )}
            </li>
          ))}
          {tools.length > TOOL_LIMIT && <li>+{tools.length - TOOL_LIMIT} more</li>}
        </ul>
      )}
    </div>
  )
}
