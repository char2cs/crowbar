import { workspaceHandlers } from './workspaces'
import { conversationHandlers } from './conversations'
import { projectHandlers } from './projects'
import { gitHandlers } from './git'
import { fsHandlers } from './fs'
import { terminalHandlers } from './terminal'
import { gitWsHandler } from './ws/git'
import { terminalWsHandler } from './ws/terminal'

export const handlers = [
  ...workspaceHandlers,
  ...conversationHandlers,
  ...projectHandlers,
  ...gitHandlers,
  ...fsHandlers,
  ...terminalHandlers,
  gitWsHandler,
  terminalWsHandler,
]
