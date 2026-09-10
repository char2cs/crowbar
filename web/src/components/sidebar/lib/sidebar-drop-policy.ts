import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  NO_MODES,
  REORDER_MODES,
  resolvesToFirstChild,
  type AllowedModes,
  type DropPolicy,
} from '@/components/tree-dnd/drop-core'
import { isWorkspaceLockedInSidebar, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { isChatWorking } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { resolveHomeRowScope } from '@/lib/store/home-tree'
import { foldWorkspaceOwners, resolveOwnerChats } from '@/components/sidebar/lib/rows-from-repo'
import { buildSidebarTree, indexSidebarTree } from '@/components/layout/workspace-tree-utils'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * What a drag may do to a row in the unified sidebar tree, and where the
 * indicator goes — the one matrix that replaces the workspace tree's
 * `SIDEBAR_DROP_POLICY` (`components/layout/drop-rules.ts`) and the chats
 * tree's `CHAT_DROP_POLICY` (`features/agent/tree/lib/chat-drop.ts`) now that
 * both trees are one `SidebarRow` forest (Task 4/8).
 *
 * A `SidebarRow` carries no `repoId`/`projectId` of its own — Task 4 kept it
 * to only what the tree draws. Scope is resolved the same way
 * `sidebar/lib/row-actions.ts` already resolves a row id against the live
 * tree (`performCreateFolder`, `performRenameRow`): a row's id is matched
 * against a repo's three id spaces (its default/home workspace, a tree
 * workspace, or a folder). This runs from pointer-driven code (a drag, never
 * a render), so reading `getState()` here follows the store's own usage rule.
 */

export interface RowScope {
  repoId: string
  projectId: string | undefined
}

/**
 * A row's owning repo/project, or null if nothing in the live store claims
 * this id — a race, or a row kind (`workflow`) with no producer feeding
 * `repos` yet. Every rule below refuses rather than guesses when this comes
 * back null, same posture the old policies took on an unrecognised selection.
 *
 * NOT a chat resolver, deliberately, even though `Repo.chats` now exists: a
 * chat is not repo-scoped for DRAG purposes (spec §8.3 makes cross-repo drag
 * legal precisely for a chat that owns no worktree), so resolving one here
 * would only feed it to the same-repo rule it is exempt from. Chat rows are
 * handled by their own branch in `allowedModes` before this is reached — see
 * there.
 *
 * Takes `repos` rather than reading the store itself so one drag call reads
 * `getState()` once, not once per subject. `drop-dom.ts`'s own design note
 * says this runs on every pointermove once a live drag wires it up (Task 21)
 * — the remaining per-row linear scan is still real cost that task should
 * consider caching per-gesture rather than per-frame; not fixed here, since
 * this file only owns the matrix, not the drag loop.
 *
 * Exported so `drop-actions.ts`'s `onDrop` implementation (Task 33) resolves
 * a row's repo/project the same way this matrix does, rather than a second
 * copy of the same id-space walk.
 */
export function resolveRowRepo(repos: readonly Repo[], rowId: string): RowScope | null {
  // A BRANCH row is addressed by the id of the chat that owns its workspace
  // (`rows-from-repo.ts`), so it matches none of the three id spaces below on
  // its own. Left untranslated, the repo-home row and every locked branch
  // resolved to null and a drop onto them became a silent no-op — the drag
  // indicator promising a move that fired no request at all.
  const id = workspaceIdOfBranchRow(repos, rowId) ?? rowId
  const repo = repos.find(
    (r) =>
      r.defaultWorkspaceId === id ||
      r.workspaces.some((w) => w.id === id) ||
      r.folders?.some((f) => f.id === id),
  )
  return repo ? { repoId: repo.id, projectId: repo.projectId } : null
}

/**
 * A CHAT's owning repo/project — the one lookup `resolveRowRepo` deliberately
 * skips (see its own doc: resolving a DRAGGED chat there would defeat spec
 * §8.3's cross-repo exemption). Used only for a chat as a drop TARGET, never
 * as a subject — by the time `allowedModes` reaches here the subject can
 * never be a chat itself (that kind returns earlier, unconditionally).
 */
function resolveChatRepo(repos: readonly Repo[], chatId: string): RowScope | null {
  const repo = repos.find((r) => r.chats?.some((c) => c.id === chatId))
  return repo ? { repoId: repo.id, projectId: repo.projectId } : null
}

/**
 * The nearest workspace-kind row's id in `id`'s ancestor chain — `id`'s own
 * row included — mirroring the backend's own golden-rule "context" walk
 * (`nearestWorkspaceAnchor`, usecases/chat/internal/tree/validate.go): a
 * folder may reorder freely within whatever branch's (or the bare repo
 * root's) subtree it already sits in, but never jump to a different one —
 * even though the coarser same-repo check above would allow it (same repo,
 * or both the bare root). `""` is the bare repo root, a real answer, not
 * "not found".
 *
 * Built from the SAME folded tree `rows-from-repo.ts` renders (`buildSidebarTree`
 * + `foldWorkspaceOwners`), so a folder's anchor here always agrees with the
 * row the user actually sees it nested under — never re-derived off raw
 * `Workspace`/`Chat` lineage, which is what let a chat's own `workspaceId`
 * (every chat carries one, folder or not) stand in for an anchor it never
 * earned; see `nearestWorkspaceAnchor`'s own doc for the exact bug that
 * caused live.
 */
function nearestBranchAnchor(repo: Repo, id: string): string {
  const ownerChats = resolveOwnerChats(repo.workspaces, repo.chats ?? [])
  const roots = buildSidebarTree(repo.workspaces, repo.folders ?? [], repo.chats ?? [])
  const { nodeById, parentById } = indexSidebarTree(foldWorkspaceOwners(roots, ownerChats), '')

  const seen = new Set<string>()
  let cursor = id
  while (cursor !== '' && !seen.has(cursor)) {
    seen.add(cursor)
    if (nodeById.get(cursor)?.kind === 'workspace') return cursor
    cursor = parentById.get(cursor) ?? ''
  }
  return ''
}

/**
 * Which of before/after/into this drag may do to this target.
 *
 * Same-project rule generalizes `drop-rules.ts`'s same-repo rule (its
 * `allowedModes`, ~line 80: `subjects.some((s) => s.repoId !== target.repoId)`):
 * rows now span one project's whole forest rather than one repo's workspaces
 * (`rows-for-project.ts`), so the boundary a drag may not cross moved up one
 * level. Cross-repo, within-project drags stay legal, but only for a row that
 * owns no worktree (design spec §8.3, "cross-repo drag is legal only for a
 * chat with no worktree") — reparenting a fork across repos makes no sense,
 * but a bubble chat pinned to nothing on disk can move freely.
 *
 * Working-row refusal (spec §8.3) reads `SidebarRow.working`, refusing every
 * mode when any dragged row is working — the UI mirror of the backend plan's
 * `guardNotWorking`, duplicated here deliberately since a drag needs to
 * refuse before a network round trip, not after.
 *
 * A protected branch (`drop-rules.ts`'s old locked rule, ~line 82-93) owns a
 * worktree pinned to its parent, so it may reorder among its own siblings and
 * nothing else — including not "after" an expanded sibling, since that would
 * nest it. Lock state isn't a `SidebarRow` field; it's read the same way
 * `file-explorer-tree.tsx` already reads it, via `isWorkspaceLockedInSidebar`
 * against the row's `workspaceId`.
 *
 * A CHAT row is exempt from all of that — see the branch below.
 */
export function allowedModes(subjects: readonly SidebarRow[], target: SidebarRow): AllowedModes {
  if (subjects.length === 0) return NO_MODES
  if (subjects.some((s) => s.working)) return NO_MODES
  // Never drop a row onto itself.
  if (subjects.some((s) => s.id === target.id)) return NO_MODES
  // A mixed-kind selection is not a thing the sidebar can express; refuse
  // rather than guess which class wins (carried over from both old policies).
  const kind = subjects[0].kind
  if (subjects.some((s) => s.kind !== kind)) return NO_MODES

  // A CHAT IS NOT REPO-SCOPED, so the repo/project walk below does not apply to
  // it — and applying it anyway is what silently refused every single
  // Recents-entry drag: `resolveRowRepo` resolved null for every chat subject
  // AND every chat target, taking the `!scope` refusal below, which made Task
  // 33's `planChatDrop` — and with it spec §8.1's whole "middle of a Recents
  // entry / above-below a Recents entry" row of the target table — unreachable
  // dead code. `Repo` does carry chats now, and `resolveRowRepo` still declines
  // to look at them ON PURPOSE (see its own note): resolving a chat there would
  // hand it to the same-repo rule §8.3 exempts it from.
  //
  // The scope a chat DOES have is its workspace: its placement lives on the
  // `AgentChat` aggregate and is written by `setChatPlacement`, which is
  // workspace-scoped. Cross-workspace is deliberately still ALLOWED here rather
  // than refused: spec §8.3 makes cross-repo drag legal precisely for "a chat
  // with no worktree", and `planChatDrop` answers the one case the backend has
  // no endpoint for with an explicit toast (`UNSUPPORTED`) — a real explanation,
  // where a silent policy refusal would be none until §8.3's refusal affordance
  // (still unbuilt, parked in Task 21) exists to say why.
  //
  // A branch is not something a chat can become a thread of — that direction
  // still refuses. A FOLDER is legal now: `kind: 'folder'` is one aggregate
  // in the current unified row model (rows-from-repo.ts's own folder push is
  // fed straight off the wire's `AgentChatFolder`/`ChatsFolderWireDTO`, the
  // same one `planChatDrop`'s target resolves against), not the two separate
  // "repo folder" / "chat folder" concepts an earlier, pre-unification
  // version of this comment worried about mixing. Refusing a folder target
  // here used to be the literal "can't drag a chat into a folder" gap, caught
  // live — `planChatDrop` accepts one the same way now; refusing here too
  // means the indicator never promises a move that would then do nothing.
  // Only when the DRAGGED row is a chat — `|| target.kind === 'chat'` used to
  // sit here too, thinking it needed to also catch "something dropped onto a
  // chat", but that dragged something is a FOLDER (or a branch) the exact
  // same amount whether its sibling happens to be a chat or a folder, and
  // belongs in whichever block below already resolves ITS kind's scope. With
  // it, a FOLDER dragged onto a CHAT target hit this block instead, found
  // `kind !== 'chat'` true, and returned NO_MODES before ever reaching the
  // home/repo scope logic that would have allowed it — caught live as a
  // project-home folder that could reorder past another folder but never
  // past a chat, "stuck" with no explanation (folders and chats share one
  // sibling order space; nothing about them not stacking blocks a reorder).
  if (kind === 'chat') {
    if (target.kind !== 'chat' && target.kind !== 'folder' && target.kind !== 'branch') {
      return NO_MODES
    }
    // THE WORKING REFUSAL, ASKED AGAIN — because `s.working` above cannot
    // answer it for a chat drawn in the TREE.
    //
    // A Recents chat row carries live turn state (its component subscribes per
    // chat), so the blanket check above already refuses it. A TREE chat row is
    // built by `rows-from-repo.ts` from the repo's reseeded chat list, which
    // has no per-turn push to ride and therefore reports `working: false`
    // always — deliberately, since a value seeded once would latch the spinner
    // on a chat whose turn ended long ago. The cost was that the SAME chat
    // dragged from the tree skipped this client-side refusal and learned it
    // only from a rejected round trip, as a raw error toast.
    //
    // `isChatWorking` is the very map Recents reads (`agentChats.working`,
    // scanned across mounted workspace stores), asked here rather than carried
    // on the row: this runs from pointer-driven code, so it reads the truth at
    // the instant of the drag and cannot go stale between renders. A chat whose
    // workspace is not mounted answers false — the same answer the row already
    // gave, and the server still refuses it.
    if (subjects.some((s) => isChatWorking(s.id))) return NO_MODES
    // A BRANCH row — a repo's own header, a locked branch, or an ordinary
    // fork folded to look like one (`rows-from-repo.ts`'s own doc: `kind:
    // 'branch'` either way) — is never a container a chat can thread INTO
    // (see this function's own doc above: "A branch is not something a
    // chat can become a thread of"), but it IS a real sibling a chat may
    // reorder PAST: chats and branches share one dense order space at
    // every level this drag reaches — project home (placeRepoAmongHome-
    // Siblings, for a repo's own header row) and a repo's own root or a
    // locked branch's own row (writeHomeNode, 2026-09-09's reparent fix).
    // Caught live as "can't put a chat right at the bottom of the list"
    // whenever a branch row happened to sit there — refusing before/after
    // here, unconditionally, is what made every one of those the literal
    // end of the list a chat could never reach. The finer same-workspace/
    // same-repo check stays the backend's own (checkChatMove/
    // checkChatContainer) — this is only the client-side pre-filter, and
    // "into" stays refused exactly as it already was.
    if (target.kind === 'branch') return REORDER_MODES
    return ALL_MODES
  }

  // A FOLDER can also be a project-home one (rows-from-home.ts) — home rides
  // no repo at all, so `resolveRowRepo` below can never see it (the literal
  // "can't drag/group a home folder" gap, caught live). Home never forks, so
  // nothing but a folder ever reaches here for it. Its scope question is
  // simpler than a repo's: a project has exactly one home workspace, so "the
  // same project" is the only thing that has to agree — no lock/fork lineage
  // to protect. A drag that mixes a home folder with anything repo-scoped is
  // refused (there is no shared container either side could land in).
  if (kind === 'folder') {
    const subjectHomeScopes = subjects.map((s) => resolveHomeRowScope(s.id))
    if (subjectHomeScopes.some((s) => s !== null)) {
      if (subjectHomeScopes.some((s) => s === null)) return NO_MODES
      const targetHome = resolveHomeRowScope(target.id)
      if (!targetHome) return NO_MODES
      if (subjectHomeScopes.some((s) => s!.projectId !== targetHome.projectId)) return NO_MODES
      return ALL_MODES
    }
  }

  // A BRANCH row can also be a REPO's own header row (rows-from-repo.ts's
  // `repoIcon`, set only on that one row) — the repo's own placement lives on
  // a different aggregate entirely (domain.Repository, not Workspace/Chat),
  // so it does not answer to the same-repo walk every ordinary branch below
  // is scoped by; it has no repo of its own to BE scoped by, and the walk
  // would refuse it outright (`resolveRowRepo` never resolves a repo's own id
  // to anything). Its one legal destination is project home: another repo's
  // header (reorder only — a repo is not a container), a home chat (reorder
  // only — nothing for a repo to thread under), or a home folder (reorder or
  // nest) — always within the SAME project, never a repo-internal folder or
  // branch. Only ever dragged alone: a multi-row selection mixing a repo
  // header with anything else already refused above (mixed kinds are
  // impossible here since every subject shares `kind`, but a multi-REPO
  // selection is not a thing this drag supports).
  if (kind === 'branch' && subjects.length === 1 && subjects[0].repoIcon) {
    const repoIcon = subjects[0].repoIcon
    if (target.kind === 'branch') {
      // Another repo's own header row always sits at a container ('' or a
      // home folder) a repo may legally land in — that is its OWN placement
      // invariant, enforced the identical way when IT was filed there — so
      // no further container check is needed here.
      return target.repoIcon && target.repoIcon.projectId === repoIcon.projectId
        ? REORDER_MODES
        : NO_MODES
    }
    const targetHome = resolveHomeRowScope(target.id)
    if (!targetHome || targetHome.projectId !== repoIcon.projectId) return NO_MODES
    // Reordering before/after TARGET lands the repo in target's OWN
    // container (target.parentId), which has to itself be legal for a repo —
    // project-home root, or another home FOLDER, never a CHAT's own thread
    // space — even when target itself is a folder. Caught live: "New
    // folder" nested inside a home chat (a legal place for a FOLDER to sit)
    // still let a repo reorder "past" it, which would have filed the repo
    // under that chat — refused server-side, but the drag indicator had
    // already promised a move nothing here should have offered. Nesting
    // INTO target is a different question (target itself, not its
    // container) and stays gated on target.kind alone, below.
    const containerID = target.parentId ?? ''
    const containerIsRepoSafe =
      containerID === '' || resolveHomeRowScope(containerID)?.kind === 'folder'
    return {
      before: containerIsRepoSafe,
      after: containerIsRepoSafe,
      into: target.kind === 'folder',
    }
  }

  const repos = useSidebarStore.getState().repos

  // `resolveRowRepo` is deliberately chat-blind (its own doc: resolving a
  // chat there would hand a DRAGGED chat the same-repo rule §8.3 exempts it
  // from) — but the SUBJECT here is never a chat (the branch above already
  // returned for that kind), so that exemption does not apply to the TARGET.
  // A folder/branch reordering past a plain repo chat sibling needs that
  // chat's repo resolved same as any other target would be, or it hits the
  // exact "stuck, no explanation" gap the home-scope block above was already
  // fixed for — caught live, the repo-scoped half of the same bug.
  const targetScope =
    resolveRowRepo(repos, target.id) ??
    (target.kind === 'chat' ? resolveChatRepo(repos, target.id) : null)
  if (!targetScope) return NO_MODES

  for (const subject of subjects) {
    const subjectScope = resolveRowRepo(repos, subject.id)
    if (!subjectScope) return NO_MODES
    if (subjectScope.projectId !== targetScope.projectId) return NO_MODES
    if (subjectScope.repoId !== targetScope.repoId && subject.ownsWorktree) return NO_MODES
  }

  // The golden rule's finer grain (spec §2.6): a FOLDER may reorder freely
  // among siblings sharing its own branch/root context, but never cross into
  // a different one — the repo-scope check above alone would allow that (same
  // repo, or both the bare root). Scoped to `kind === 'folder'` only: a
  // BRANCH row's placement is fork lineage (`Workspace.parentId`), a
  // different edge entirely, with no "context" of its own to protect.
  if (kind === 'folder') {
    const repo = repos.find((r) => r.id === targetScope.repoId)
    if (repo) {
      const subjectAnchor = nearestBranchAnchor(repo, subjects[0].id)
      if (subjects.some((s) => nearestBranchAnchor(repo, s.id) !== subjectAnchor)) return NO_MODES
      const reorderAnchor = nearestBranchAnchor(repo, target.parentId ?? '')
      const intoAnchor = nearestBranchAnchor(repo, target.id)
      return {
        before: subjectAnchor === reorderAnchor,
        after: subjectAnchor === reorderAnchor,
        into: subjectAnchor === intoAnchor,
      }
    }
  }

  const hasLocked = subjects.some((s) => isWorkspaceLockedInSidebar(repos, s.workspaceId))
  if (hasLocked) {
    const sameParent = subjects.every((s) => s.parentId === target.parentId)
    if (!sameParent) return NO_MODES
    return resolvesToFirstChild(target, 'after')
      ? { before: true, after: false, into: false }
      : REORDER_MODES
  }

  return ALL_MODES
}

/**
 * The outer band of a row reorders; the middle nests. A folder gets the
 * container band — filing into one is cheap and common. Everything else gets
 * the heavy band: nesting under a branch re-parents a fork, nesting under a
 * chat makes the row one of its threads, and nesting into an as-yet-unwired
 * `workflow` row is assumed the same weight until one exists to say
 * otherwise — all heavier moves that deserve a harder-to-hit target. Matches
 * `drop-rules.ts`'s own `edgeBandFor` verbatim, generalized off `SidebarRowKind`.
 */
export function edgeBandFor(kind: string): number {
  return kind === 'folder' ? EDGE_BAND_CONTAINER : EDGE_BAND_HEAVY
}

export const SIDEBAR_DROP_POLICY: DropPolicy<SidebarRow, SidebarRow> = {
  allowedModes,
  edgeBandFor,
}
