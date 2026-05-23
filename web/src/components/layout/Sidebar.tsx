import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { SidebarHeader } from './SidebarHeader'
import { ChatRow, RepoRow, WorkspaceRow, NewRow } from './SidebarRow'

export interface ProjectChat {
  id: string
  title: string
  age: string
}

export interface Workspace {
  id: string
  num?: number
  branch: string
  added?: number
  deleted?: number
  age: string
}

export interface Repo {
  id: string
  name: string
  avatarLabel: string
  avatarColor: string
  workspaces: Workspace[]
}

export interface SidebarProps {
  projectName: string
  userInitials: string
  chats: ProjectChat[]
  repos: Repo[]
  activeChatId?: string
  activeWorkspaceId?: string
  onChatClick?: (id: string) => void
  onWorkspaceClick?: (repoId: string, workspaceId: string) => void
  onNewChat?: () => void
  onNewWorkspace?: () => void
}

export function Sidebar({
  projectName,
  userInitials,
  chats,
  repos,
  activeChatId,
  activeWorkspaceId,
  onChatClick,
  onWorkspaceClick,
  onNewChat,
  onNewWorkspace,
}: SidebarProps) {
  return (
    <div className="flex h-full flex-col overflow-hidden bg-card">
      <SidebarHeader projectName={projectName} userInitials={userInitials} />
      <ScrollArea className="flex-1">
        <div className="py-1">
          {chats.map(chat => (
            <ChatRow
              key={chat.id}
              title={chat.title}
              age={chat.age}
              active={chat.id === activeChatId}
              onClick={() => onChatClick?.(chat.id)}
            />
          ))}
          <NewRow label="New chat" onClick={onNewChat} />

          <Separator className="my-1 mx-3" />

          {repos.map(repo => (
            <div key={repo.id}>
              <RepoRow
                name={repo.name}
                avatarLabel={repo.avatarLabel}
                avatarColor={repo.avatarColor}
              />
              {repo.workspaces.map(ws => (
                <WorkspaceRow
                  key={ws.id}
                  num={ws.num}
                  branch={ws.branch}
                  added={ws.added}
                  deleted={ws.deleted}
                  age={ws.age}
                  active={ws.id === activeWorkspaceId}
                  onClick={() => onWorkspaceClick?.(repo.id, ws.id)}
                />
              ))}
              <NewRow label="New workspace" onClick={onNewWorkspace} />
            </div>
          ))}
        </div>
      </ScrollArea>
    </div>
  )
}
