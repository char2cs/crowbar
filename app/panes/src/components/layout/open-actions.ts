import type { useNavigate } from '@tanstack/react-router'
import type { Chat, Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import type { DragSubject } from './removal-plan'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { toast } from '@/features/window/stores/toast-store'
import { openChatInOwnPane } from '@/components/sidebar/lib/pane-drop-actions'
import { resolveHomeRowScope } from '@/core/entities/home-rows'
import { renderedProjectRows } from '@/components/sidebar/lib/rendered-rows'
import { chatNotLoadedYet } from '@/components/sidebar/lib/row-actions'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

export type NavigateFn = ReturnType<typeof useNavigate>

export interface ResolvedRow {
  repo: Repo
  subject: DragSubject
}

export function resolveChatRow(
  repos: readonly Repo[],
  id: string,
): { repo: Repo; chat: Chat } | null {
  if (workspaceIdOfBranchRow(repos, id) !== null) return null
  for (const repo of repos) {
    const chat = repo.chats?.find((c) => c.id === id)
    if (chat) return { repo, chat }
  }
  return null
}

function isOpenableWorkspaceOfRepo(repo: Repo, wsId: string | null): string | null {
  if (!wsId) return null
  if (wsId === repo.defaultWorkspaceId) return wsId
  return repo.workspaces.some((w) => w.id === wsId) ? wsId : null
}

export function resolveRow(repos: readonly Repo[], id: string): ResolvedRow | null {
  const rowId = workspaceIdOfBranchRow(repos, id) ?? id
  for (const repo of repos) {
    const ws = repo.workspaces.find((w) => w.id === rowId)
    if (ws) {
      return {
        repo,
        subject: {
          kind: 'workspace',
          id: rowId,
          repoId: repo.id,
          locked: ws.status === 'locked',
          parentId: ws.parentId,
        },
      }
    }
    const folder = repo.folders?.find((f) => f.id === id)
    if (folder) {
      return { repo, subject: { kind: 'folder', id, repoId: repo.id, parentId: folder.parentId } }
    }
    if (repo.defaultWorkspaceId === rowId) {
      return { repo, subject: { kind: 'workspace', id: rowId, repoId: repo.id } }
    }
  }
  return null
}

function paneOpenSubject(chatId: string, workspaceId: string): SidebarRowType {
  return {
    id: chatId,
    kind: 'chat',
    parentId: null,
    order: 0,
    label: '',
    ownsWorktree: false,
    workspaceId,
    working: false,
    hasView: false,
  }
}

export function openChatInOwnView(chatId: string, workspaceId: string): boolean {
  if (workspaceId !== getActiveWorkspaceId()) return false
  openChatInOwnPane(paneOpenSubject(chatId, workspaceId))
  return true
}

export async function navigateThenOpenChat(
  navigate: NavigateFn,
  params: { projectId: string; repoId: string; wsId: string },
  chatId: string,
): Promise<void> {
  await navigate({ to: '/ide/$projectId/$repoId/$wsId', params })
  openChatInOwnPane(paneOpenSubject(chatId, params.wsId))
}

export function openChatRoute(
  repos: readonly Repo[],
  chatId: string,
  workspaceId: string,
  navigate: NavigateFn,
): boolean {
  const homeRow = resolveHomeRowScope(chatId)
  if (homeRow) {
    void openHomeChat(homeRow.projectId, homeRow.homeWorkspaceId, chatId, navigate)
    return true
  }
  const found = resolveRow(repos, workspaceId)
  if (!found?.repo.projectId) return false
  if (openChatInOwnView(chatId, workspaceId)) return true
  void navigateThenOpenChat(
    navigate,
    { projectId: found.repo.projectId, repoId: found.repo.id, wsId: workspaceId },
    chatId,
  )
  return true
}

export function handleOpen(
  id: string,
  repos: readonly Repo[],
  navigate: NavigateFn,
  { fold = true }: { fold?: boolean } = {},
): boolean {
  const toggle = () => {
    if (fold) treeUiStore.getState().toggleChatRow(id)
  }
  const homeRow = resolveHomeRowScope(id)
  if (homeRow) {
    if (homeRow.kind === 'folder') {
      toggle()
      return false
    }
    return openChatRoute(repos, id, homeRow.homeWorkspaceId, navigate)
  }

  const chatRow = resolveChatRow(repos, id)
  if (chatRow) {
    const { projectId } = chatRow.repo
    const rowWsId = projectId
      ? (renderedProjectRows(projectId, repos).find((r) => r.id === id)?.workspaceId ?? null)
      : null
    const wsId = isOpenableWorkspaceOfRepo(chatRow.repo, rowWsId)
    if (!wsId || !chatRow.repo.projectId) {
      toggle()
      return false
    }
    return openChatRoute(repos, id, wsId, navigate)
  }

  const found = resolveRow(repos, id)
  if (!found) return false
  if (found.subject.kind === 'folder') {
    toggle()
    return false
  }
  if (!found.repo.projectId) return false
  const isRepoHome = found.subject.id === found.repo.defaultWorkspaceId
  if (isRepoHome || found.subject.locked) {
    toggle()
    return false
  }
  const owningChatId = found.repo.workspaces.find((w) => w.id === found.subject.id)?.owningChatId
  if (owningChatId) {
    return openChatRoute(repos, owningChatId, found.subject.id, navigate)
  }
  const branch = found.repo.workspaces.find((w) => w.id === found.subject.id)?.branch
  toast.error(chatNotLoadedYet('open', branch))
  return false
}

export async function openHomeChat(
  projectId: string,
  homeWorkspaceId: string,
  chatId: string,
  navigate: NavigateFn,
): Promise<void> {
  if (openChatInOwnView(chatId, homeWorkspaceId)) return
  await navigate({ to: '/ide/$projectId/home', params: { projectId } })
  openChatInOwnPane(paneOpenSubject(chatId, homeWorkspaceId))
}
