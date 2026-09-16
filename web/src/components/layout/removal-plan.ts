import { buildSidebarTree, indexSidebarTree } from './workspace-tree-utils'
import {
  getPostDeleteNavigationTarget,
  EMPTY_CHATS,
  EMPTY_FOLDERS,
  type Chat,
  type Folder,
  type Repo,
} from '@/lib/store/sidebar'
import type { RemovalDraft, RemovalEntry } from '@/lib/store/sidebar-removal'
import type { DragSubjectBase } from '@/components/tree-dnd/drop-core'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { resolveHomeRowScope, getHomeTree } from '@/lib/store/home-tree'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/** The little a removal needs to know about a project: which one, and its label. */
export interface ProjectRow {
  id: string
  name: string
}

/** The five movable classes. They do not mix. `chat` is additive (addendum
 *  §2) — the original four `workspace | folder | repo | project` are
 *  unchanged. */
export type DropKind = 'workspace' | 'folder' | 'repo' | 'project' | 'chat'

/**
 * A row as a removal/drag subject — which class of thing it is, which one it
 * is, and enough of its placement to resolve its owning repo.
 *
 * Formerly `components/layout/drop-rules.ts`'s type (that module's policy
 * logic went with the unified sidebar's `sidebar-drop-policy.ts`, but this
 * shape lives on: `space-content-actions.ts`'s `resolveRow` still builds one
 * per row and hands it here to plan a removal).
 */
export interface DragSubject extends DragSubjectBase {
  kind: DropKind
  /** Repo scope, for the same-repo rule. Absent on repos and projects. */
  repoId?: string
  /** A protected branch: reorders among its own siblings and nothing else. */
  locked?: boolean
  /** Its current parent, for the locked same-parent rule. */
  parentId?: string
}

/**
 * What a removal means, worked out before anything is hidden.
 *
 * Pure, like `drop-plan.ts` next door and for the same reason: the whole of it —
 * which rows go and which rows go WITH them — is decided here and can be tested
 * without a pointer.
 *
 * The one rule worth stating: a hold is not a delete. Everything below computes
 * what to HIDE, and hiding is undone by putting the ids back. The destructive
 * half is one API call per entry, and it happens only when a hold is committed.
 */

/**
 * The rows a removal hides, and the rows that go with them.
 *
 * Three kinds, three answers, and they differ because the daemon differs:
 *
 * - **A workspace** takes its whole subtree: the delete cascades server-side, so
 *   hiding only the row would promise less than is about to happen.
 * - **A folder** takes nothing. It holds no worktree, and deleting one reparents
 *   its children to the folder's own parent — which is why a folder needs no
 *   confirmation where a repo does.
 * - **A repo** takes every worktree under it, which is exactly why it waits on
 *   an answer rather than on a clock.
 * - **A project** takes every repo, and therefore every worktree under every one
 *   of them. It waits on an answer for the same reason, and on a modal after
 *   that.
 */
export function planRemoval(
  subjects: readonly DragSubject[],
  repos: readonly Repo[],
  projects: readonly ProjectRow[] = [],
): RemovalDraft[] {
  const drafts: RemovalDraft[] = []
  const claimed = new Set<string>()

  for (const subject of subjects) {
    const draft = draftFor(subject, repos, projects)
    // A row already inside another subject's subtree is not a second removal —
    // it is part of the first one, and holding it twice would put two rows in
    // the tray for one disappearance.
    if (!draft || claimed.has(draft.id)) continue
    drafts.push(draft)
    for (const id of draft.hiddenIds) claimed.add(id)
  }

  return drafts
}

/** Every chat hanging off `id`, transitively — the same subtree a
 *  `parentId` walk of the sidebar's own lightweight `Chat[]` already
 *  resolves for other purposes (`chat-rows.ts`'s tree), inlined here rather
 *  than imported since this only ever needs ids, not a rendered tree. */
function chatDescendantsOf(chats: readonly Chat[], id: string): string[] {
  const childrenOf = new Map<string, string[]>()
  for (const c of chats) {
    if (!c.parentId) continue
    const siblings = childrenOf.get(c.parentId)
    if (siblings) siblings.push(c.id)
    else childrenOf.set(c.parentId, [c.id])
  }
  const out: string[] = []
  const walk = (parentId: string) => {
    for (const childId of childrenOf.get(parentId) ?? []) {
      out.push(childId)
      walk(childId)
    }
  }
  walk(id)
  return out
}

function draftFor(
  subject: DragSubject,
  repos: readonly Repo[],
  projects: readonly ProjectRow[],
): RemovalDraft | null {
  if (subject.kind === 'project') {
    const project = projects.find((p) => p.id === subject.id)
    if (!project) return null
    const owned = repos.filter((r) => r.projectId === project.id)
    return {
      kind: 'project',
      id: project.id,
      label: project.name,
      projectId: project.id,
      // A project spans every repo under it, so there is no single owning one.
      repoId: '',
      // Sidebar rows are not workspace-scoped — `wsId`/`providerIcon` are
      // vestiges of the Chats panel's own drafts, gone with it (Task 22).
      wsId: '',
      providerIcon: '',
      // The project's own row AND every repo row inside it: the delete cascades
      // server-side, so hiding only the header would leave its repos on screen
      // with nothing above them.
      hiddenIds: [project.id, ...owned.map((r) => r.id)],
      extra: owned.reduce((n, r) => n + 1 + r.workspaces.length, 0),
      fallbackWsId: null,
    }
  }

  if (subject.kind === 'repo') {
    const repo = repos.find((r) => r.id === subject.id)
    if (!repo?.projectId) return null
    return {
      kind: 'repo',
      id: repo.id,
      label: repo.name,
      projectId: repo.projectId,
      repoId: repo.id,
      wsId: '',
      providerIcon: '',
      hiddenIds: [repo.id],
      extra: repo.workspaces.length,
      fallbackWsId: null,
    }
  }

  // A project-home chat or folder rides no repo at all (`resolveHomeRowScope`'s
  // own doc) — checked BEFORE the repo lookup below, which would otherwise just
  // find nothing for `subject.repoId` (never set for a home row) and return
  // null, reported live as "Can't delete X yet" rather than an actual removal.
  if (subject.kind === 'chat' || subject.kind === 'folder') {
    const homeRow = resolveHomeRowScope(subject.id)
    if (homeRow) {
      const tree = getHomeTree(homeRow.projectId)
      if (subject.kind === 'chat') {
        const chat = tree.chats.find((c) => c.id === subject.id)
        if (!chat) return null
        // Same subtree rule as a repo chat's own branch below: reparenting
        // and deleting both take every thread hanging off this one with it.
        const descendants = chatDescendantsOf(tree.chats, chat.id)
        return {
          kind: 'chat',
          id: chat.id,
          label: chat.title || UNTITLED_CHAT_LABEL,
          projectId: homeRow.projectId,
          // '' — no owning repo; `wsId` (the DELETE route's own scope, see
          // `deleteChat`'s contract) is the project's home workspace instead.
          repoId: '',
          wsId: homeRow.homeWorkspaceId,
          providerIcon: '',
          hiddenIds: [chat.id, ...descendants],
          extra: descendants.length,
          fallbackWsId: null,
        }
      }
      const folder = tree.folders.find((f) => f.id === subject.id)
      if (!folder) return null
      return {
        kind: 'folder',
        id: folder.id,
        label: folder.name,
        projectId: homeRow.projectId,
        repoId: '',
        wsId: '',
        providerIcon: '',
        hiddenIds: [folder.id],
        extra: 0,
        fallbackWsId: null,
      }
    }
  }

  const repo = repos.find((r) => r.id === subject.repoId)
  if (!repo?.projectId) return null

  if (subject.kind === 'chat') {
    // NOT a branch row (`resolveChatRow`'s own rule — such a row is a
    // WORKSPACE, addressed by `workspaceIdOfBranchRow` elsewhere, and has no
    // business reaching this branch at all). Generalized the same way that
    // rule is: a folded, non-locked fork's row lives in this same chat id
    // space now too, not just a locked branch or a repo home.
    if (workspaceIdOfBranchRow(repos, subject.id) !== null) return null
    const chat = repo.chats?.find((c) => c.id === subject.id)
    if (!chat) return null
    // The DELETE route is repo-scoped (`deleteChat`'s own contract) — any
    // workspace of this repo resolves the URL, same as
    // `row-actions.ts`'s `scopedWorkspaceIdOf`.
    const wsId = repo.defaultWorkspaceId ?? repo.workspaces[0]?.id
    if (!wsId) return null
    const descendants = chatDescendantsOf(repo.chats ?? [], chat.id)
    return {
      kind: 'chat',
      id: chat.id,
      label: chat.title || UNTITLED_CHAT_LABEL,
      projectId: repo.projectId,
      repoId: repo.id,
      wsId,
      providerIcon: '',
      // Reparenting and deleting both take the whole subtree (spec §8.3) —
      // every thread hanging off this chat goes with it.
      hiddenIds: [chat.id, ...descendants],
      extra: descendants.length,
      fallbackWsId: null,
    }
  }

  if (subject.kind === 'folder') {
    const folder = (repo.folders ?? EMPTY_FOLDERS).find((f) => f.id === subject.id)
    if (!folder) return null
    return {
      kind: 'folder',
      id: folder.id,
      label: folder.name,
      projectId: repo.projectId,
      repoId: repo.id,
      wsId: '',
      providerIcon: '',
      hiddenIds: [folder.id],
      extra: 0,
      fallbackWsId: null,
    }
  }

  const workspace = repo.workspaces.find((w) => w.id === subject.id)
  // A protected branch keeps its worktree; the daemon refuses the delete, so the
  // tray must never accept one and promise otherwise.
  if (!workspace || workspace.status === 'locked') return null

  const tree = indexSidebarTree(
    buildSidebarTree(repo.workspaces, repo.folders ?? EMPTY_FOLDERS),
    repo.id,
  )
  const descendants = tree.index.descendantsOf(workspace.id)
  // BOTH HALVES OF EVERY ROW THAT GOES, NOT JUST THE WORKSPACE HALF.
  //
  // A workspace row IS its owning chat (`rows-from-repo.ts`) and the delete is
  // literally `deleteChat` — so hiding the `Workspace` records alone left every
  // one of those owning chats visible for the whole eight-second countdown.
  // With its workspace gone from under it, such a chat has no workspace node to
  // fold onto, and the row the user just deleted came straight back as a CHAT
  // BUBBLE — a different glyph, a different label, a different set of verbs —
  // which is exactly the "the branch row transformed into a conversation" the
  // hold is supposed to make impossible.
  //
  // Threads go too, for the same reason `chatDescendantsOf` takes them on a
  // chat removal: the daemon's delete cascades the whole subtree, and hiding
  // less than what is about to go promises less than happens.
  const goingWorkspaceIds = new Set([workspace.id, ...descendants])
  const hiddenChatIds: string[] = []
  for (const chat of repo.chats ?? EMPTY_CHATS) {
    if (!chat.workspaceId || !goingWorkspaceIds.has(chat.workspaceId)) continue
    hiddenChatIds.push(chat.id, ...chatDescendantsOf(repo.chats ?? EMPTY_CHATS, chat.id))
  }
  return {
    kind: 'workspace',
    id: workspace.id,
    // The row this held workspace actually renders as is keyed by its OWNING
    // CHAT's id, not the raw workspace id above — `descendantHiddenIds` needs
    // this to keep that chat un-hidden too (see this field's own doc), or the
    // fold has nothing left to fold onto and the row re-keys the instant it's
    // held, losing its DOM identity for no reason. Falls back to the
    // workspace id itself for the rare beat where a workspace has landed
    // with no owning chat resolved yet — nothing in `hiddenChatIds` would
    // name that id anyway, so the fallback never hides anything real.
    primaryRowId: workspace.owningChatId ?? workspace.id,
    label: workspace.branch,
    projectId: repo.projectId,
    repoId: repo.id,
    wsId: '',
    providerIcon: '',
    hiddenIds: [workspace.id, ...descendants, ...new Set(hiddenChatIds)],
    // Counts WORKSPACES only, unchanged: the tray row says how many more
    // worktrees go with this one, and the chat ids above are the same rows
    // counted once, not extra ones.
    extra: descendants.length,
    // Resolved now, against a tree that still has the row in it.
    fallbackWsId: getPostDeleteNavigationTarget(repos as Repo[], workspace.id),
  }
}

/**
 * The sidebar as it reads with the tray's rows taken out.
 *
 * A held row is hidden, never deleted, so this is a projection over the repos
 * the store already holds — which is what makes Cancel a matter of dropping an
 * id rather than putting a subtree back together.
 *
 * A held FOLDER is the one case that rewrites rather than filters: deleting a
 * folder reparents its children to the folder's own parent, so the preview has
 * to show them there. Filtering the folder alone would re-root them, which is a
 * different place and a promise the commit would not keep.
 */
export function applyPendingRemovals(
  repos: readonly Repo[],
  hiddenIds: ReadonlySet<string>,
): Repo[] {
  if (hiddenIds.size === 0) return repos as Repo[]

  const out: Repo[] = []
  for (const repo of repos) {
    if (hiddenIds.has(repo.id)) continue

    const folders = repo.folders ?? EMPTY_FOLDERS
    const heldFolders = folders.filter((f) => hiddenIds.has(f.id))
    const workspaces = repo.workspaces.filter((w) => !hiddenIds.has(w.id))
    // A held CHAT (addendum §2's drag-to-trash) is never re-homed the way a
    // held folder's children are — its own descendants are already part of
    // this same hold (`chatDescendantsOf`, removal-plan.ts's `draftFor`), so
    // there is never a survivor left under it to reparent. A plain filter is
    // the whole story. `repo.chats` is optional (older frames simply omit
    // it), so an untouched repo with none stays `undefined`, not `[]`.
    const chats = repo.chats?.some((c) => hiddenIds.has(c.id))
      ? repo.chats.filter((c) => !hiddenIds.has(c.id))
      : repo.chats
    if (
      heldFolders.length === 0 &&
      workspaces.length === repo.workspaces.length &&
      chats === repo.chats
    ) {
      out.push(repo)
      continue
    }

    if (heldFolders.length === 0) {
      out.push({ ...repo, workspaces, chats })
      continue
    }

    // Walk each held folder's own parent up past any held ancestor, so a hold
    // that takes a folder and its parent folder still lands the survivors at the
    // outermost place that is actually still on screen.
    const byId = new Map(folders.map((f) => [f.id, f]))
    const survivingParentOf = (folderId: string): string => {
      let cursor = byId.get(folderId)?.parentId ?? ''
      const seen = new Set<string>([folderId])
      while (cursor && hiddenIds.has(cursor) && !seen.has(cursor)) {
        seen.add(cursor)
        cursor = byId.get(cursor)?.parentId ?? ''
      }
      return cursor
    }
    const rehomed = new Map(heldFolders.map((f) => [f.id, survivingParentOf(f.id)]))

    const survivors: Folder[] = []
    for (const folder of folders) {
      if (hiddenIds.has(folder.id)) continue
      survivors.push(
        folder.parentId && rehomed.has(folder.parentId)
          ? { ...folder, parentId: rehomed.get(folder.parentId) }
          : folder,
      )
    }

    out.push({
      ...repo,
      workspaces: workspaces.map((w) =>
        w.folderId && rehomed.has(w.folderId) ? { ...w, folderId: rehomed.get(w.folderId) } : w,
      ),
      folders: survivors,
      chats,
    })
  }

  return out
}

/**
 * What `applyPendingRemovals` should actually strip, now that a held row
 * transforms IN PLACE (sidebar-row.tsx's `RemovingSidebarRow`) instead of
 * vanishing into a separate tray: every hidden id MINUS each entry's own
 * PRIMARY id, which stays on screen so it has a row left to transform.
 * `RemovalEntry.hiddenIds` always lists the primary id first, then its
 * cascade (`chatDescendantsOf`/the workspace-subtree walk above) — those
 * descendants still disappear outright, same as before, since the primary
 * row already accounts for them (the "+N goes with it" count).
 *
 * 'repo'/'project' are the exception: `attachRemovalState`'s own doc notes
 * neither ever matches a row here, so there is no in-place row for their
 * primary id to stay visible FOR — excluding it would just stop the whole
 * repo/project from being hidden at all (their `hiddenIds` IS the primary,
 * for a repo). Those two kinds keep their full `hiddenIds`, primary
 * included; only a kind `attachRemovalState` can actually transform gets
 * its own id held back.
 *
 * A 'workspace' entry holds back TWO ids, not one: `entry.id` (the raw
 * Workspace id, so the record survives for `rows-from-repo.ts` to read) and
 * `entry.primaryRowId` (the owning chat's id, so the fold has a chat left to
 * fold onto and the rendered row keeps the same key it always had — see that
 * field's own doc on `RemovalEntry`). Every other kind's `id` already IS its
 * `primaryRowId`.
 */
export function descendantHiddenIds(entries: readonly RemovalEntry[]): Set<string> {
  const out = new Set<string>()
  for (const entry of entries) {
    const transformsInPlace = entry.kind !== 'repo' && entry.kind !== 'project'
    const primaryRowId = entry.primaryRowId ?? entry.id
    for (const id of entry.hiddenIds) {
      if (transformsInPlace && (id === entry.id || id === primaryRowId)) continue
      out.add(id)
    }
  }
  return out
}

/**
 * Marks the row(s) matching a held entry with `row.removal`, so
 * `sidebar-row.tsx` can render it transformed in place.
 *
 * A 'workspace'-kind entry's own `id` is the RAW workspace id — but the
 * branch row it corresponds to is rendered/looked-up by its OWNING CHAT's
 * id (`rows-from-repo.ts`'s fold), never the raw one. Matching on `row.id`
 * for that kind would silently find nothing. `row.workspaceId` is the one
 * field every branch row still carries the raw id on, so that is what a
 * 'workspace' entry matches against; a 'chat'/'folder' entry's `id` already
 * IS the rendered row's own id, no translation needed. 'repo'/'project'
 * entries never match anything here — a removal that takes a whole repo or
 * project has no single row of its own in this tree to transform (its own
 * gesture is the space header's trash, not a row).
 */
export function attachRemovalState(
  rows: readonly SidebarRow[],
  entries: readonly RemovalEntry[],
): SidebarRow[] {
  const byWorkspaceId = new Map<string, RemovalEntry>()
  const byRowId = new Map<string, RemovalEntry>()
  for (const entry of entries) {
    if (entry.kind === 'workspace') byWorkspaceId.set(entry.id, entry)
    else if (entry.kind === 'chat' || entry.kind === 'folder') byRowId.set(entry.id, entry)
  }
  if (byWorkspaceId.size === 0 && byRowId.size === 0) return rows as SidebarRow[]

  return rows.map((row) => {
    const entry =
      row.kind === 'branch' && row.workspaceId
        ? byWorkspaceId.get(row.workspaceId)
        : byRowId.get(row.id)
    if (!entry) return row
    return {
      ...row,
      removal: { entryId: entry.entryId, deadlineAt: entry.deadlineAt, extra: entry.extra },
    }
  })
}
