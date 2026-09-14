import { useMemo, useState } from 'react'
import type { AgentActivity, AgentSubagent } from '@/features/agent/api/agent-api'
import { formatDuration } from '@/features/agent/lib/agent-activity'
import { elapsedMs, ToolList } from '@/features/agent/transcript/turn-tools'

/**
 * Subagents with NO top-level turn of their own to group under — a provider
 * (codex) whose subagent is a whole SECOND conversation, opened from a tool
 * call completing rather than a dedicated subagent_pre/subagent_post pair
 * mid-turn (see the backend's turn/observation.go, openNestedSubagent).
 *
 * `AgentTurnSubagents` (transcript/turn-tools.tsx) can never show these: it
 * groups by `turnId`, and these carry none. Without this list a finished one
 * had no record anywhere the instant it stopped running — the live shelf
 * only ever counts what is STILL running.
 */
export function finishedNestedSubagents(activity: AgentActivity): AgentSubagent[] {
  return activity.subagents
    .filter((subagent) => subagent.endedAt && !subagent.turnId)
    .sort((a, b) => a.seq - b.seq)
}

/**
 * The durable record of every nested subagent this chat has run — its own
 * tool calls, and its own final reply, both already part of the activity
 * payload (unlike a tool call's own request/result bytes, which are
 * content-addressed and fetched on demand: a subagent's nested activity is
 * small enough, and asked-for often enough, to ship with the rest of it).
 *
 * A STILL RUNNING nested subagent is not drawn here — it is counted in
 * SubagentShelf's own live strip, the same as any other running subagent,
 * and gains its expandable row here only once it is done.
 */
export function NestedSubagentPanel({ activity }: { activity: AgentActivity }) {
  const subagents = useMemo(() => finishedNestedSubagents(activity), [activity])
  if (subagents.length === 0) return null

  return (
    <ul className="tools" data-testid="agent-nested-subagents">
      {subagents.map((subagent) => (
        <NestedSubagentRow key={subagent.id} subagent={subagent} activity={activity} />
      ))}
    </ul>
  )
}

function NestedSubagentRow({
  subagent,
  activity,
}: {
  subagent: AgentSubagent
  activity: AgentActivity
}) {
  const [open, setOpen] = useState(false)
  const tools = useMemo(
    () => activity.toolCalls.filter((call) => call.subagentId === subagent.id),
    [activity.toolCalls, subagent.id],
  )
  const reply = subagent.messages?.at(-1)?.text
  const label = subagent.agentType ? `Subagent · ${subagent.agentType}` : 'Subagent'
  const summary = (
    <>
      <span>{label}</span>
      <span>{formatDuration(elapsedMs(subagent.startedAt, subagent.endedAt as string))}</span>
    </>
  )

  if (tools.length === 0 && !reply) {
    return <li data-status="ok">{summary}</li>
  }
  return (
    <li data-status="ok">
      <details open={open}>
        <summary
          onClick={(e) => {
            e.preventDefault()
            setOpen((o) => !o)
          }}
        >
          {summary}
        </summary>
        {open && (
          <>
            {tools.length > 0 && <ToolList calls={tools} testId="agent-nested-subagent-tools" />}
            {reply && <p className="reply">{reply}</p>}
          </>
        )}
      </details>
    </li>
  )
}
