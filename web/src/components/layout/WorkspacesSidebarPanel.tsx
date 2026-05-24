import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { ChatRow, RepoRow, WorkspaceRow, NewRow } from './SidebarRow'
import type { ProjectChat, Repo } from '@/lib/store/sidebar'

interface WorkspacesSidebarPanelProps {
  chats: ProjectChat[]
  repos: Repo[]
  collapsedRepos?: Set<string>
  activeChatId?: string
  activeWorkspaceId?: string
  onChatClick?: (id: string) => void
  onWorkspaceClick?: (repoId: string, workspaceId: string) => void
  onNewChat?: () => void
  onNewWorkspace?: () => void
  onDeleteChat?: (id: string) => void
  onDeleteWorkspace?: (wsId: string) => void
  onRepoToggle?: (repoId: string) => void
}

const EMPTY_SET = new Set<string>()

export function WorkspacesSidebarPanel({
  chats, repos, collapsedRepos = EMPTY_SET,
  activeChatId, activeWorkspaceId,
  onChatClick, onWorkspaceClick, onNewChat, onNewWorkspace,
  onDeleteChat, onDeleteWorkspace, onRepoToggle,
}: WorkspacesSidebarPanelProps) {
  return (
    <ScrollArea className="flex-1">
      <div className="py-1">
        {chats.map(chat => (
          <ChatRow
            key={chat.id}
            title={chat.title}
            age={chat.age}
            active={chat.id === activeChatId}
            onClick={() => onChatClick?.(chat.id)}
            onDelete={onDeleteChat ? () => onDeleteChat(chat.id) : undefined}
          />
        ))}
        <NewRow label="New chat" onClick={onNewChat} />
        <Separator className="my-1 mx-3" />
        {repos.map(repo => {
          const collapsed = collapsedRepos.has(repo.id)
          return (
            <div key={repo.id}>
              <RepoRow
                name={repo.name}
                avatarLabel={repo.avatarLabel}
                avatarColor={repo.avatarColor}
                collapsed={collapsed}
                onClick={() => onRepoToggle?.(repo.id)}
              />
              {!collapsed && (
                <>
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
                      onDelete={onDeleteWorkspace ? () => onDeleteWorkspace(ws.id) : undefined}
                    />
                  ))}
                  <NewRow label="New workspace" onClick={onNewWorkspace} />
                </>
              )}
            </div>
          )
        })}
      </div>
    </ScrollArea>
  )
}
