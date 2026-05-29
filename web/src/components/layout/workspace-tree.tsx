import { useNavigate, useRouterState } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import { useSidebarStore } from '@/lib/store/sidebar'
import { createMockChat } from '@/lib/mock/chats'
import { cn } from '@/lib/utils'
import { WorkspaceTreeItem } from './workspace-tree-item'
import { ChatRow, NewRow } from './SidebarRow'
import type { Workspace } from '@/lib/store/sidebar'

export interface WorkspaceTreeNode {
  workspace: Workspace
  children: WorkspaceTreeNode[]
}

export function buildWorkspaceTree(workspaces: Workspace[]): WorkspaceTreeNode[] {
  const nodeMap = new Map<string, WorkspaceTreeNode>()
  for (const ws of workspaces) {
    nodeMap.set(ws.id, { workspace: ws, children: [] })
  }

  const roots: WorkspaceTreeNode[] = []
  for (const ws of workspaces) {
    const node = nodeMap.get(ws.id)!
    const parent = ws.parentId ? nodeMap.get(ws.parentId) : undefined

    if (!parent || parent === node) {
      roots.push(node)
    } else {
      let cursor: WorkspaceTreeNode | undefined = parent
      let cycle = false
      while (cursor) {
        if (cursor.workspace.id === ws.id) { cycle = true; break }
        cursor = cursor.workspace.parentId ? nodeMap.get(cursor.workspace.parentId) : undefined
      }
      if (cycle) {
        roots.push(node)
      } else {
        parent.children.push(node)
      }
    }
  }
  return roots
}

export function WorkspaceTree() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const chats = useSidebarStore(s => s.chats)
  const repos = useSidebarStore(s => s.repos)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1] ?? ''
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1] ?? ''

  function handleWorkspaceClick(wsId: string) {
    void navigate({ to: '/workspaces/$wsId', params: { wsId } })
  }

  function handleChatClick(id: string) {
    void navigate({ to: '/chat/$chatId', params: { chatId: id } })
  }

  function handleNewChat() {
    const chat = createMockChat()
    useSidebarStore.getState().addChat({ id: chat.id, title: chat.title, age: 'just now' })
    void navigate({ to: '/chat/$chatId', params: { chatId: chat.id } })
  }

  function handleDeleteChat(id: string) {
    useSidebarStore.getState().deleteChat(id)
  }

  return (
    <ScrollArea className="flex-1">
      <div className="py-1">
        {chats.map(chat => (
          <ChatRow
            key={chat.id}
            title={chat.title}
            age={chat.age}
            active={chat.id === activeChatId}
            onClick={() => handleChatClick(chat.id)}
            onDelete={() => handleDeleteChat(chat.id)}
          />
        ))}
        <NewRow label="New chat" onClick={handleNewChat} />
        <Separator className="my-1 mx-3" />
        {repos.map(repo => {
          const roots = buildWorkspaceTree(repo.workspaces)
          return (
            <div key={repo.id} className="mb-2">
              <div className="flex items-center gap-1.5 px-2 py-1 mt-1">
                <span className={cn('rounded px-1.5 py-0.5 text-[10px] font-bold text-primary-foreground', repo.avatarColor)}>
                  {repo.avatarLabel}
                </span>
                <span className="text-[10px] tracking-widest text-muted-foreground/40 uppercase">
                  {repo.name}
                </span>
              </div>
              <div className="px-1">
                {roots.map(node => (
                  <WorkspaceTreeItem
                    key={node.workspace.id}
                    node={node}
                    depth={0}
                    activeWorkspaceId={activeWorkspaceId}
                    onWorkspaceClick={handleWorkspaceClick}
                  />
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </ScrollArea>
  )
}
