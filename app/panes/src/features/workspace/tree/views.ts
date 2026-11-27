import type { Chat, Repo, Workspace, WorkspaceStatus } from '@/core/entities/repo-types'
import type { Problem } from '@/lib/errors/problem'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { workspaceIconFields, type ChatIconFields } from '@/components/sidebar/lib/chat-icon-index'
import {
  resolveHomeOwnerId,
  resolveOwnerChats,
  resolveOwnerOfChat,
} from '@/components/sidebar/lib/workspace-owners'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { repoMembers, type LegacyHome } from './legacy-tree-index'

export type ProblemLookup = (...entityIds: string[]) => Problem | null
const NO_PROBLEMS: ProblemLookup = () => null

interface ChatView {
  label: string
  labelProvisional: boolean
  createdAt?: string
  workspaceId?: string
  ownedWorkspaceId?: string
}

interface WorkspaceView {
  rowId: string
  label: string
  labelProvisional: boolean
  createdAt?: string
  working: boolean
  icon: Omit<ChatIconFields, 'kind' | 'ownsWorktree'>
}

interface RepoView {
  rowId: string | null
  workspaceId: string | null
  label: string
  labelProvisional: boolean
  working: boolean
  branchName?: string
  locked: boolean
  status?: WorkspaceStatus
  repoIcon?: SidebarRow['repoIcon']
}

export interface RowViews {
  chat(id: string): ChatView | undefined
  workspace(id: string): WorkspaceView | undefined
  repo(id: string): RepoView | undefined
}

const GENERATED_BRANCH_NAME = /^chat-[0-9a-f]{8}$/

function isProvisionalBranchName(branch: string | undefined): boolean {
  return branch !== undefined && GENERATED_BRANCH_NAME.test(branch)
}

function chatView(chat: Chat, ownedWorkspaceId: string | undefined): ChatView {
  return {
    label: chat.title || UNTITLED_CHAT_LABEL,
    labelProvisional: !chat.title,
    createdAt: chat.createdAt,
    workspaceId: chat.workspaceId,
    ...(ownedWorkspaceId === undefined ? {} : { ownedWorkspaceId }),
  }
}

function workspaceView(
  ws: Workspace,
  ownerId: string | undefined,
  ownerTitle: string | undefined,
  repo: Repo,
  problemOf: ProblemLookup,
): WorkspaceView {
  const rowId = ownerId ?? ws.id
  const byChat = ownerTitle !== undefined && ws.status !== 'locked'
  return {
    rowId,
    label: byChat ? ownerTitle || UNTITLED_CHAT_LABEL : ws.branch,
    labelProvisional: byChat ? !ownerTitle : isProvisionalBranchName(ws.branch),
    createdAt: ws.createdAt,
    working: ws.working ?? false,
    icon: workspaceIconFields(ws, repo.defaultBranch, problemOf(ws.id, rowId)),
  }
}

function repoView(repo: Repo, chats: readonly Chat[]): RepoView {
  const homeId = repo.defaultWorkspaceId ?? null
  return {
    rowId: homeId === null ? null : resolveHomeOwnerId(homeId, repo.defaultOwningChatId, chats),
    workspaceId: homeId,
    label: repo.name,
    labelProvisional: isProvisionalBranchName(repo.defaultBranch),
    working: repo.defaultWorking ?? false,
    branchName: repo.defaultBranch,
    locked: repo.defaultWorkspaceStatus === 'locked',
    status: repo.defaultWorkspaceStatus,
    repoIcon: repo.projectId
      ? {
          repoId: repo.id,
          projectId: repo.projectId,
          name: repo.name,
          avatarLabel: repo.avatarLabel,
          avatarColor: repo.avatarColor,
          avatarURL: repo.avatarURL,
        }
      : undefined,
  }
}

export function viewsFromStores(
  repos: readonly Repo[],
  home: LegacyHome | null,
  problemOf: ProblemLookup = NO_PROBLEMS,
): RowViews {
  const chats = new Map<string, ChatView>()
  const workspaces = new Map<string, WorkspaceView>()
  const repoViews = new Map<string, RepoView>()
  for (const repo of repos) {
    const members = repoMembers(repo)
    const ownerChats = resolveOwnerChats(members.workspaces)
    const ownerOfChat = resolveOwnerOfChat(ownerChats, members.chats)
    const titles = new Map(members.chats.map((c) => [c.id, c.title]))
    for (const chat of members.chats) chats.set(chat.id, chatView(chat, ownerOfChat.get(chat.id)))
    for (const ws of members.workspaces) {
      const ownerId = ownerChats.get(ws.id)
      const title = ownerId === undefined ? undefined : titles.get(ownerId)
      workspaces.set(ws.id, workspaceView(ws, ownerId, title, repo, problemOf))
    }
    repoViews.set(repo.id, repoView(repo, members.chats))
  }
  const homeOwners = resolveOwnerOfChat(new Map(), home?.chats ?? [])
  for (const chat of home?.chats ?? []) chats.set(chat.id, chatView(chat, homeOwners.get(chat.id)))
  return {
    chat: (id) => chats.get(id),
    workspace: (id) => workspaces.get(id),
    repo: (id) => repoViews.get(id),
  }
}
