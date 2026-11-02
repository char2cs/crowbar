import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { ChatType } from '@/lib/types'

/** A chat as the projects scope carries it: the chat routes' row, with the project
 *  and repo it is listed under (repoId '' for a project home). */
export interface ChatEntity extends AgentChat {
  projectId: string
  repoId: string
  type?: ChatType
}
