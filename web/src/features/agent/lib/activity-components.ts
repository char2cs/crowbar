import type {
  AgentActivity,
  AgentChoice,
  AgentSubagent,
  AgentToolCall,
} from '@/features/agent/api/agent-api'

/** Provider-neutral activity vocabulary shared by the rich activity surface. */
type ActivityComponentKind =
  | 'user_message'
  | 'assistant_message'
  | 'reasoning'
  | 'plan'
  | 'tool_call'
  | 'tool_output'
  | 'diff'
  | 'permission_request'
  | 'user_input_request'
  | 'subagent'
  | 'citation'
  | 'attachment'
  | 'context_usage'
  | 'rate_limits'
  | 'compaction'
  | 'status_notice'
  | 'connection_notice'

type ActivityComponentStatus =
  'pending' | 'active' | 'completed' | 'failed' | 'declined' | 'interrupted' | 'abandoned'

export interface ActivityComponent {
  id: string
  turnId: string
  parentId?: string
  seq: number
  kind: ActivityComponentKind
  status: ActivityComponentStatus
  createdAt: string
  updatedAt: string
  completedAt?: string
  payload: Record<string, unknown>
  updates?: ActivityComponentUpdate[]
}

interface ActivityComponentUpdate {
  id: string
  seq: number
  kind: string
  at: string
  status: ActivityComponentStatus
  payload: Record<string, unknown>
}

function toolStatus(tool: AgentToolCall): ActivityComponentStatus {
  if (tool.status === 'running') return 'active'
  if (tool.status === 'error') return 'failed'
  if (tool.status === 'declined') return 'declined'
  if (tool.status === 'abandoned') return 'abandoned'
  return 'completed'
}

function choiceStatus(choice: AgentChoice): ActivityComponentStatus {
  if (choice.pending) return 'pending'
  if (choice.resolution === 'abandoned') return 'abandoned'
  const options = [
    ...(choice.options ?? []),
    ...(choice.questions ?? []).flatMap((question) => question.options ?? []),
  ]
  if (
    choice.kind === 'tool_permission' &&
    choice.resolution === 'answered' &&
    choice.answeredOptionIds?.some((id) =>
      options.some((option) => option.id === id && option.kind === 'deny'),
    )
  ) {
    return 'declined'
  }
  return 'completed'
}

function choiceKind(choice: AgentChoice): ActivityComponentKind {
  return choice.kind === 'tool_permission' ? 'permission_request' : 'user_input_request'
}

function subagentStatus(subagent: AgentSubagent): ActivityComponentStatus {
  return subagent.endedAt ? 'completed' : 'active'
}

/**
 * Project the current activity ledger into the stable, ordered component shape.
 * This is intentionally derived from the existing API so Claude and older
 * daemons remain compatible while the durable component endpoint is introduced.
 */
export function activityComponents(activity: AgentActivity): ActivityComponent[] {
  const components: ActivityComponent[] = []
  const subagentTurnById = new Map<string, string>()
  for (const subagent of activity.subagents) {
    if (subagent.turnId) subagentTurnById.set(subagent.id, subagent.turnId)
  }

  for (const tool of activity.toolCalls) {
    const turnId = tool.turnId || subagentTurnById.get(tool.subagentId ?? '') || ''
    const status = toolStatus(tool)
    components.push({
      id: tool.id,
      turnId,
      parentId: tool.subagentId,
      seq: tool.seq,
      kind: 'tool_call',
      status,
      createdAt: tool.startedAt,
      updatedAt: tool.endedAt ?? tool.startedAt,
      completedAt: tool.endedAt,
      payload: {
        name: tool.name,
        kind: tool.kind,
        locations: tool.locations,
        target: tool.target,
        error: tool.error,
        durationMs: tool.durationMs,
        hasRequest: tool.hasRequest,
        hasResult: tool.hasResult,
      },
    })
    if (tool.hasResult) {
      const id = `${tool.id}:output`
      const updatedAt = tool.endedAt ?? tool.startedAt
      const payload = { toolCallId: tool.id, side: 'result' }
      components.push({
        id,
        turnId,
        parentId: tool.id,
        seq: tool.seq,
        kind: 'tool_output',
        status,
        createdAt: tool.startedAt,
        updatedAt,
        completedAt: tool.endedAt,
        payload,
        updates: [
          {
            id: `${id}:1`,
            seq: 1,
            kind: 'available',
            at: updatedAt,
            status,
            payload,
          },
        ],
      })
    }
  }

  for (const choice of activity.choices) {
    components.push({
      id: choice.id,
      turnId: choice.turnId,
      seq: choice.seq,
      kind: choiceKind(choice),
      status: choiceStatus(choice),
      createdAt: choice.at,
      updatedAt: choice.resolvedAt ?? choice.at,
      completedAt: choice.resolvedAt,
      payload: { choice },
    })
  }

  for (const subagent of activity.subagents) {
    components.push({
      id: subagent.id,
      turnId: subagent.turnId,
      seq: subagent.seq,
      kind: 'subagent',
      status: subagentStatus(subagent),
      createdAt: subagent.startedAt,
      updatedAt: subagent.endedAt ?? subagent.startedAt,
      completedAt: subagent.endedAt,
      payload: { agentType: subagent.agentType, messages: subagent.messages ?? [] },
    })
  }

  for (const interruption of activity.interruptions) {
    const kind = interruption.kind === 'compaction' ? 'compaction' : 'status_notice'
    components.push({
      id: interruption.id,
      turnId: interruption.turnId,
      seq: interruption.seq,
      kind,
      status: interruption.resolvedAt ? 'completed' : 'active',
      createdAt: interruption.at,
      updatedAt: interruption.resolvedAt ?? interruption.at,
      completedAt: interruption.resolvedAt,
      payload: { interruption },
    })
  }

  return components.sort((a, b) => {
    if (a.seq !== b.seq) return a.seq - b.seq
    if (a.id === b.parentId) return -1
    if (b.id === a.parentId) return 1
    return a.id.localeCompare(b.id)
  })
}
