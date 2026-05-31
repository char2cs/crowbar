import { workspaceHandlers } from './workspaces'
import { flowHandlers } from './flows'
import { conversationHandlers } from './conversations'
import { projectHandlers } from './projects'
import { gitHandlers } from './git'
import { fsHandlers } from './fs'
import { terminalHandlers } from './terminal'
import { markdownChatHandlers } from './markdown-chat'
import { gitWsHandler } from './ws/git'
import { chatWsHandler } from './ws/chat'
import { terminalWsHandler } from './ws/terminal'

export const handlers = [
  ...workspaceHandlers,
  ...flowHandlers,
  ...conversationHandlers,
  ...projectHandlers,
  ...gitHandlers,
  ...fsHandlers,
  ...terminalHandlers,
  ...markdownChatHandlers,
  gitWsHandler,
  chatWsHandler,
  terminalWsHandler,
]
