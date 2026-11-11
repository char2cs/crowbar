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

// Opening a sidebar row: resolving what an id names, and putting its chat in
// a pane (navigating to its workspace or project home first when needed).

export type NavigateFn = ReturnType<typeof useNavigate>

/** What `id` resolves to: its owning repo, and the subject a drag/removal call needs. */
export interface ResolvedRow {
  repo: Repo
  subject: DragSubject
}

// Find `id` among CHAT rows; callers consult this before `resolveRow`, which cannot see a chat.
// Separate so `DragSubject`'s closed `DropKind` union stays chat-free. A branch row shares the
// chat id space but is a WORKSPACE (rename moves the branch, trash removes it), so it is null here.
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

// `wsId` if it names a workspace of THIS repo, else null. A chat moved across repos can keep a
// workspace from the repo it left (§9.2), and /ide/:p/:r/:ws with a ws outside :r resolves nothing.
function isOpenableWorkspaceOfRepo(repo: Repo, wsId: string | null): string | null {
  if (!wsId) return null
  if (wsId === repo.defaultWorkspaceId) return wsId
  return repo.workspaces.some((w) => w.id === wsId) ? wsId : null
}

// Find `id` — a workspace, folder, or repo default workspace — across every visible repo (a
// row's repo is never ambiguous by project). Not a chat resolver: call `resolveChatRow` first.
export function resolveRow(repos: readonly Repo[], id: string): ResolvedRow | null {
  // Translate a branch row's owning-chat id to its workspace once, at the boundary, so the
  // drag matrix, `planRemoval` and `RemovalDraft` never see two id spaces.
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
      // Not a real drag/removal subject — `draftFor` finds no matching row in
      // `repo.workspaces` for this id and returns null, which is what makes
      // trashing the repo-home row a safe no-op below rather than a delete.
      return { repo, subject: { kind: 'workspace', id: rowId, repoId: repo.id } }
    }
  }
  return null
}

// Names a chat and its workspace for `openChatInOwnPane`, which reads only those two fields.
// The rest is type-required filler: never hand this to anything that renders or drags a row.
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

// A click opens a chat in its OWN view (§8.4): reveal, fill an empty pane, or a new pane — never
// the drop-on-active-pane merge, which stays a drag gesture. False unless `workspaceId` is already
// active; other rows go through `navigateThenOpenChat`.
export function openChatInOwnView(chatId: string, workspaceId: string): boolean {
  if (workspaceId !== getActiveWorkspaceId()) return false
  openChatInOwnPane(paneOpenSubject(chatId, workspaceId))
  return true
}

// Navigate to `wsId`, then open `chatId` in its own view right away. Never wait for the workspace
// to become "active": the active PANE's workspace outranks the route (ide-shell.tsx), so that wait
// can deadlock. Panes are window-level, and the subject's `workspaceId` is the resolution hint.
export async function navigateThenOpenChat(
  navigate: NavigateFn,
  params: { projectId: string; repoId: string; wsId: string },
  chatId: string,
): Promise<void> {
  await navigate({ to: '/ide/$projectId/$repoId/$wsId', params })
  openChatInOwnPane(paneOpenSubject(chatId, params.wsId))
}

// The one "project home or repo workspace, then open in its own pane" path for a known chat id,
// navigating first when needed; `handleOpen` and Recents' `focusRecent` both route through it so
// neither forgets project home. Returns whether a route was found.
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

// Opens a row into a pane, or folds a container (folder, repo home, locked branch: §3, never
// checked out into). `repos` is the removal-filtered rendered list; home rows resolve first.
// True only when a chat opened. `fold: false` for the phone, which folds from the disclosure only.
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
    // The drawn row's `workspaceId`, not the raw `Chat.workspaceId`: a bubble with none
    // of its own runs in its nearest ancestor workspace, which only the tree knows.
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
  // A folder has no destination: the row body toggles it.
  if (found.subject.kind === 'folder') {
    toggle()
    return false
  }
  if (!found.repo.projectId) return false
  // §3: the repo home and a locked branch are containers, not checkouts — fold like a folder.
  const isRepoHome = found.subject.id === found.repo.defaultWorkspaceId
  if (isRepoHome || found.subject.locked) {
    toggle()
    return false
  }
  // An unlocked workspace row opens as the chat that owns it (§8.4). Pass `subject.id`, never
  // the row id: a branch row is addressed by its owning chat, and only `resolveRow` maps it back.
  const owningChatId = found.repo.workspaces.find((w) => w.id === found.subject.id)?.owningChatId
  if (owningChatId) {
    return openChatRoute(repos, owningChatId, found.subject.id, navigate)
  }
  // No owner yet means the workspace list is still landing (the daemon mints one on first read);
  // a bare workspace route would spin forever on a chat-keyed explorer.
  const branch = found.repo.workspaces.find((w) => w.id === found.subject.id)?.branch
  toast.error(chatNotLoadedYet('open', branch))
  return false
}

// The project-home sibling of `navigateThenOpenChat` (same no-wait sequencing), for a new home
// thread or a clicked home row. Separate only because `/ide/$projectId/home` has no repo or ws.
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
