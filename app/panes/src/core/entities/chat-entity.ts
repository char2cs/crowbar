import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { ChatType } from '@/lib/types'

export interface ChatEntity extends AgentChat {
  projectId: string
  repoId: string
  type?: ChatType
}
