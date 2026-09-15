import type { AgentActivity, AgentSubagent } from '@/features/agent/api/agent-api'

/**
 * Subagents with NO top-level turn of their own — a provider (codex) whose
 * subagent is a whole SECOND conversation, opened from a tool call completing
 * rather than a dedicated subagent_pre/subagent_post pair mid-turn (see the
 * backend's turn/observation.go, openNestedSubagent).
 *
 * `groupSubagentsByTurn` (transcript/turn-tools.tsx) can never show these: it
 * groups by `turnId`, and these carry none. `agent-transcript.tsx` folds this
 * list into the last turn instead, so a finished one still gets a record
 * somewhere once the live shelf stops counting it.
 */
export function finishedNestedSubagents(activity: AgentActivity): AgentSubagent[] {
  return activity.subagents
    .filter((subagent) => subagent.endedAt && !subagent.turnId)
    .sort((a, b) => a.seq - b.seq)
}
