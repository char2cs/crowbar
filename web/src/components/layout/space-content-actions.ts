import type { useNavigate } from '@tanstack/react-router'
import { useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { planRemoval, type DragSubject } from './removal-plan'
import { createChat, createChatWithOwnWorktree, deleteChat } from '@/features/agent/api/agent-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { scopedWorkspaceIdOf } from '@/components/sidebar/lib/row-actions'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { toast } from '@/features/window/stores/toast-store'

/** What `id` resolves to: its owning repo, and the subject a drag/removal call needs. */
export interface ResolvedRow {
  repo: Repo
  subject: DragSubject
}

type NavigateFn = ReturnType<typeof useNavigate>

/**
 * Find `id` among the repo's CHAT rows.
 *
 * Deliberately its own resolver rather than a fourth branch inside
 * `resolveRow` below: that one answers with a `DragSubject`, whose `kind` is
 * the closed `DropKind` union (`workspace | folder | repo | project`) that the
 * drag matrix, `planRemoval` and `RemovalDraft` all switch on exhaustively.
 * Widening that union to admit a chat would change four surfaces that have
 * nothing to do with clicking a row, so a chat answers here instead and
 * `resolveRow` keeps exactly the contract it had.
 *
 * Callers must consult THIS FIRST. `resolveRow` cannot see a chat, and every
 * one of its callers treats "not found" as "do nothing" — which is how a chat
 * row came to look like every other row while silently doing nothing.
 *
 * AND it deliberately does NOT match a `branch` row, even though one lives in
 * the chat id space: `rows-from-repo.ts` gives a locked branch and a repo home
 * the id of the chat that owns their workspace, because that is what the daemon
 * places by. Such a row is a WORKSPACE — `resolveRow` answers for it, via
 * `workspaceIdOfBranchRow`. Matching it here sent every verb down the chat
 * path, which is not a hypothetical: it made a locked branch's "+" silently
 * inert, pointed its trash at `deleteChat`, and turned renaming the repo-home
 * row into retitling a conversation. A new caller must handle both clauses.
 */
export function resolveChatRow(
  repos: readonly Repo[],
  id: string,
): { repo: Repo; chat: Chat } | null {
  for (const repo of repos) {
    // A `branch` row lives in the chat id space but is NOT a chat row: it is
    // how a locked branch or a repo home is drawn, and `rows-from-repo.ts`
    // gives that workspace's row this id precisely so the daemon can place
    // under it. Matching it here would send every verb down the chat path —
    // "+" would go silently inert, trash would delete the branch's own row,
    // and rename would retitle it instead of moving the git branch.
    const chat = repo.chats?.find((c) => c.id === id && c.type !== 'branch')
    if (chat) return { repo, chat }
  }
  return null
}

/**
 * The workspace a chat row OPENS, or null when it opens none.
 *
 * A worktree chat (§3.1) names the workspace it owns and navigating to it is
 * exactly what clicking a branch row does. A bubble names none — it borrows an
 * ancestor's ground — and there is nothing to navigate to.
 *
 * The named workspace must belong to THIS repo. Spec §9.2 makes a repo's chats
 * an open set (a bubble moved across repos keeps ancestors in the repo it
 * left), so a `workspaceId` pointing outside is an ordinary state, not
 * corruption — and routing to /ide/:p/:r/:ws with a ws that is not under :r
 * would be a URL nothing resolves.
 */
function openableWorkspaceOf(repo: Repo, chat: Chat): string | null {
  const wsId = chat.workspaceId
  if (!wsId) return null
  if (wsId === repo.defaultWorkspaceId) return wsId
  return repo.workspaces.some((w) => w.id === wsId) ? wsId : null
}

/**
 * Find `id` — a workspace, a folder, or a repo's own default workspace —
 * across every visible repo, regardless of which project's space panel
 * rendered it. Extracted verbatim from sidebar-tree-panel.tsx (Task 8/29): a
 * row's owning repo is never ambiguous by project, so this needs no
 * per-project variant.
 *
 * NOT a chat resolver — see `resolveChatRow` above for why, and call that
 * first.
 */
export function resolveRow(repos: readonly Repo[], id: string): ResolvedRow | null {
  // A branch row is addressed by its owning chat's id, but everything downstream
  // of here — the drag matrix, `planRemoval`, `RemovalDraft` — is about the
  // WORKSPACE. Translate once, at the boundary, so none of them has to know two
  // id spaces exist.
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

/**
 * Opens a row: navigates into a workspace, or toggles a fold.
 *
 * `repos` is the caller's removal-tray-filtered list (what is actually
 * rendered), not the raw store — a row already hidden pending removal must
 * not be openable.
 *
 * A CHAT row answers this the same two ways every other row does, chosen by
 * the one fact §3.1 distinguishes chats on:
 *
 *   - a WORKTREE chat owns a workspace, so it navigates there — the same thing
 *     clicking the branch row for that workspace does;
 *   - a BUBBLE owns none, so it folds, like every other container that has no
 *     destination of its own.
 *
 * Neither opens the chat INTO A PANE, which is the thing a reader will expect
 * to find here. That needs a chatId->workspace resolution the render path does
 * not have for a non-active workspace — `drop-actions.ts`'s `openChatIntoPane`
 * documents the failure (a pane that renders permanently blank and, because
 * pane state is persisted, survives reload). Folding is not a placeholder for
 * it; it is what a row with no destination has always done.
 */
export function handleOpen(id: string, repos: readonly Repo[], navigate: NavigateFn): void {
  const chatRow = resolveChatRow(repos, id)
  if (chatRow) {
    const wsId = openableWorkspaceOf(chatRow.repo, chatRow.chat)
    const projectId = chatRow.repo.projectId
    if (!wsId || !projectId) {
      useSidebarStore.getState().toggleChatRow(id)
      return
    }
    void navigate({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId, repoId: chatRow.repo.id, wsId },
    })
    return
  }

  const found = resolveRow(repos, id)
  if (!found) return
  // A folder has no destination — the row body toggles it, same as the
  // tree it replaces.
  if (found.subject.kind === 'folder') {
    useSidebarStore.getState().toggleChatRow(id)
    return
  }
  if (!found.repo.projectId) return
  void navigate({
    to: '/ide/$projectId/$repoId/$wsId',
    // `subject.id`, never the row's — a branch row is addressed by its owning
    // chat, and only `resolveRow` knows which workspace that names.
    params: { projectId: found.repo.projectId, repoId: found.repo.id, wsId: found.subject.id },
  })
}

/**
 * Trashes a row. A chat's delete is a direct `deleteChat` call, never a
 * removal-tray draft — the tray's `planRemoval`/`RemovalDraft` has no chat
 * subject to hold one with (`resolveChatRow`'s own doc explains why that
 * union stays closed). Every other row still goes through the tray, reading
 * the store's raw, current repos (not a caller-supplied filtered snapshot):
 * the true current state, not the UI's already-hidden-pending-removal
 * overlay.
 *
 * Returns whether anything was actually held/deleted. `false` covers: a chat
 * whose repo has no workspace at all to scope the DELETE request through
 * (`scopedWorkspaceIdOf`), a repo-home id (resolves to a `workspace` subject
 * naming no row in `repo.workspaces` — repo deletion gets its own
 * confirmation flow in Part H and is not reachable from a row's trash button
 * yet), and a user-locked, non-home workspace (`planRemoval`'s `draftFor`
 * refuses one — the daemon would refuse the delete too, so the tray must
 * never accept one and promise otherwise).
 */
export function handleTrash(id: string): boolean {
  const currentRepos = useSidebarStore.getState().repos
  // Checked BEFORE resolveRow, which cannot see a chat at all
  // (`resolveChatRow`'s own doc: "callers must consult THIS FIRST").
  const chatRow = resolveChatRow(currentRepos, id)
  if (chatRow) {
    // The chat's OWN repo, never `chat.workspaceId` — the DELETE route is
    // repo-scoped, so any workspace of this repo resolves the URL; a bubble
    // has none of its own to name.
    const wsId = scopedWorkspaceIdOf(chatRow.repo)
    if (!wsId) return false
    deleteChat(wsId, id).catch((err: unknown) => {
      toast.error(err instanceof Error ? err.message : 'Failed to delete chat')
    })
    return true
  }
  const found = resolveRow(currentRepos, id)
  if (!found) return false
  const projects = dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS
  const drafts = planRemoval([found.subject], currentRepos, projects)
  if (drafts.length === 0) return false
  useRemovalTrayStore.getState().hold(drafts)
  return true
}

/**
 * Trashes a whole PROJECT via the same removal tray a row's trash uses —
 * spec §9: "every row that owns something carries a trash: chats,
 * workspaces, folders, repos, and the space header for the project."
 *
 * Deliberately not routed through `DeleteConfirmDialog` the way a row's
 * trash is: `planRemoval`'s project draft already hides the project's row
 * AND every repo under it, and `RemovalTray` pops `RemovalConfirmDialog`
 * for exactly the two cascading kinds (`repo`, `project`) before it commits
 * — so a project already gets the "confirm names what goes" step, from the
 * surface that owns it, and a second dialog in front would ask twice.
 *
 * Returns whether anything was actually held, so the caller can say
 * something rather than silently doing nothing (`draftFor` returns null for
 * a project id no loaded project claims).
 */
export function handleTrashProject(projectId: string): boolean {
  const currentRepos = useSidebarStore.getState().repos
  const projects = dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS
  const drafts = planRemoval([{ kind: 'project', id: projectId }], currentRepos, projects)
  if (drafts.length === 0) return false
  useRemovalTrayStore.getState().hold(drafts)
  return true
}

/** Creates a workspace (fork) or a thread (chat) under `parentId`. */
export function handleCreate(parentId: string, kind: 'workspace' | 'thread'): void {
  const currentRepos = useSidebarStore.getState().repos
  // A chat row's "+" is inert for now, and it must be SILENTLY inert: falling
  // through to resolveRow's miss is what it did before chat rows existed, but
  // now that `resolveChatRow` can see one, the thread branch below would reach
  // its "a folder has none to run it in" refusal — a sentence that is simply
  // untrue of a chat, which is precisely the row a thread hangs off. Wiring it
  // needs the cwd walk (§3.2) to find a bubble's ground workspace; until then
  // an affordance that does nothing beats one that explains itself wrongly.
  if (resolveChatRow(currentRepos, parentId)) return
  const found = resolveRow(currentRepos, parentId)
  if (!found) return
  const { repo, subject } = found
  const { projectId } = repo
  if (!projectId) return

  if (kind === 'workspace') {
    // Task 8: mints the workspace AND its first chat in ONE call (POST
    // .../chats {ownWorktree: true} — backend Task 7) instead of the old
    // chat-less postWorkspace, which produced a bare branch row now and a
    // separate child chat row only once something else later started a
    // conversation in it. The parent named below is the clicked row's own
    // fork parent, same as the old mapping (a folder is the one exception the
    // old `placement` distinguished — that split has no counterpart on this
    // endpoint's single `parentId`, so a folder click also just names
    // itself here) — resolved to the chat that owns it, see below.
    //
    // A generated branch name is no longer minted client-side either: the
    // server names it the same way Promote's spontaneous create already
    // does (model spec §4.1), since this is the same "nothing of its own
    // to name the branch" shape.
    const provider = useAgentProvidersStore.getState().providers.find((p) => p.enabled)
    if (!provider) return
    // The daemon places by CHAT id, and the clicked row's own id is only that
    // id for a branch row (a locked branch, the repo home — `rows-from-repo.ts`
    // draws those AS their owning `branch` chat). A REGULAR fork's row is id'd
    // from its `Workspace`, because its owner is an ordinary conversation
    // already drawn beside it, so its owning chat has to be read off the
    // workspace record instead. Falls back to the clicked row for the ids that
    // name no workspace of this repo (the repo home, a folder) and for a frame
    // that carries no owner yet.
    const owningChatId = repo.workspaces.find((w) => w.id === subject.id)?.owningChatId
    createChatWithOwnWorktree(projectId, repo.id, provider.id, owningChatId || parentId).catch(
      (err: unknown) => {
        toast.error(err instanceof Error ? err.message : 'Failed to create workspace')
      },
    )
    return
  }

  // A thread needs a real workspace to run in — a folder names none. Reachable
  // only from an empty folder's affordance dropdown (its own "+" always makes
  // a workspace, see above); the dropdown itself has no folder-vs-workspace
  // split to hide this option behind, so say why instead of swallowing the
  // click.
  if (subject.kind !== 'workspace') {
    toast.error('Start a thread from a workspace row — a folder has none to run it in')
    return
  }
  // `subject.id`, not the clicked row's — this one needs the WORKSPACE (it
  // opens that workspace's store and posts to its chats mount), and a branch
  // row's own id is the chat that owns it.
  const wsId = subject.id
  const store = getOrCreateWorkspaceStore(wsId)
  const provider = store.getState().agentChats.providers.find((p) => p.enabled)
  if (!provider) return
  createChat(wsId, provider.id).catch((err: unknown) => {
    toast.error(err instanceof Error ? err.message : 'Failed to start chat')
  })
}
