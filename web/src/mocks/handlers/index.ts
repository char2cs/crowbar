import { workspaceHandlers } from './workspaces'
import { flowHandlers } from './flows'
import { conversationHandlers } from './conversations'
import { projectHandlers } from './projects'
import { gitHandlers } from './git'
import { fsHandlers } from './fs'
import { terminalHandlers } from './terminal'

export const handlers = [
  ...workspaceHandlers,
  ...flowHandlers,
  ...conversationHandlers,
  ...projectHandlers,
  ...gitHandlers,
  ...fsHandlers,
  ...terminalHandlers,
]
