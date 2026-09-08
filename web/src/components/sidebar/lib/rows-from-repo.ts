import {
  EMPTY_CHATS,
  EMPTY_FOLDERS,
  type Chat,
  type Repo,
  type Workspace,
} from '@/lib/store/sidebar'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * The id of the `branch` chat that owns the repo/project HOME workspace.
 *
 * The home workspace is never a `Workspace` row (it lives on `repo.defaultWorkspaceId`,
 * outside `repo.workspaces` entirely), so there is no `Workspace.owningChatId` to read it
 * off of the way {@link resolveOwnership} does for every other row below — this has to search
 * `chats` directly, and the `type === 'branch'` filter is load-bearing here in a way it no
 * longer is anywhere else in this file: `RepoChatWireDTO.workspaceId` is stamped on every
 * chat that runs INSIDE a workspace, not just the one that owns it (a thread carries its
 * parent's), so without the filter this could just as easily return an ordinary
 * conversation instead of the one real owner. The daemon mints exactly one `branch`-typed
 * chat per locked branch, repo home and project home (`tree/backfill.go`'s
 * `owningChatType`) and never a second, which is what makes the filter safe here.
 */
export function homeOwningChatId(chats: readonly Chat[], homeId: string): string {
  const owner = chats.find((c) => c.type === 'branch' && c.workspaceId === homeId)
  if (!owner) {
    throw new Error(`rowsFromRepo: home workspace ${homeId} owns no branch row`)
  }
  return owner.id
}

/**
 * Whether `branch` is still the server-generated placeholder a spontaneous
 * worktree create mints when the caller supplies no name of its own
 * (`hierarchy.branch_name.go`'s `provisionalBranchName`: `"chat-" + the first
 * 8 hex chars of a fresh UUID`), collision-checked against real refs on the
 * backend so this exact shape never collides with a name a person typed.
 *
 * There is no separate wire flag for "not yet renamed" (spec §3.4's branch
 * half settles only "when the task is achieved and the agent renames it," in
 * git, and a rename is indistinguishable from any other branch PATCH once it
 * lands) — this pattern IS the signal, and it self-clears the moment a real
 * name replaces it.
 */
const GENERATED_BRANCH_NAME = /^chat-[0-9a-f]{8}$/

function isProvisionalBranchName(branch: string | undefined): boolean {
  return branch !== undefined && GENERATED_BRANCH_NAME.test(branch)
}

/**
 * Who owns what, resolved ONCE, from BOTH directions, before anything is built.
 *
 * A workspace-owning row is assembled from two records that arrive on two
 * INDEPENDENT channels: the `Chat` (`crowbar_chats`, reseeded only when a
 * repo's folder-signal generation moves) and the `Workspace`
 * (`crowbar_workspaces`, its own entity stream). They are routinely skewed —
 * by a frame during a create, and for the entire eight-second countdown during
 * a delete, when the removal tray hides one half and not the other.
 *
 * The bug this type exists to delete: the old resolution asked only
 * `Workspace.owningChatId`, so the WORKSPACE half was load-bearing for the
 * row's very KIND. Lose it for one render and the owning chat stopped being
 * foldable, fell through to `buildSidebarTree`'s chat rule, and drew as a
 * `kind: 'chat'` BUBBLE — a different glyph, a different label, a different id
 * space and a different set of verbs, for a row that had not changed at all.
 * That is what turned a branch row into a conversation mid-delete, and what
 * drew a freshly forked workspace as a bubble until its `WorkspaceDTO` caught
 * up.
 *
 * So ownership is a UNION now, and either half alone is enough to establish it:
 *
 *   - `Workspace.owningChatId` — the direction that always worked, and still
 *     the only one available for a row cached before `Chat.ownsWorktree`
 *     existed;
 *   - `Chat.ownsWorktree` + `Chat.workspaceId` — the same fact carried on the
 *     chat itself, delivered atomically with it (see `ChatDTO.ownsWorktree`).
 *
 * A chat's `workspaceId` alone can NOT stand in for the second clause: a thread
 * carries its parent's, so it names a worktree it does not own. `ownsWorktree`
 * is what picks the one real owner out of however many rows hold the worktree.
 *
 * The consequence that matters downstream: `ownerOfChat` is the complete set of
 * chats that are WORKSPACES, whether or not their `Workspace` record is on hand
 * — so `walk` can always draw one as a `branch` row and degrade only the
 * DECORATION (branch name, diff counts, lock) that genuinely lives on the
 * missing half.
 */
export interface Ownership {
  /** Workspace id -> the chat that owns it. Only workspaces actually present. */
  chatOfWorkspace: Map<string, string>
  /** Chat id -> the workspace it owns, present or not. The authority on "this
   *  chat is a workspace row", and deliberately the wider of the two maps. */
  ownerOfChat: Map<string, string>
}

export function resolveOwnership(
  workspaces: readonly Workspace[],
  chats: readonly Chat[],
): Ownership {
  const chatIds = new Set(chats.map((c) => c.id))
  const chatOfWorkspace = new Map<string, string>()
  const ownerOfChat = new Map<string, string>()
  for (const ws of workspaces) {
    if (!ws.owningChatId || !chatIds.has(ws.owningChatId)) continue
    chatOfWorkspace.set(ws.id, ws.owningChatId)
    ownerOfChat.set(ws.owningChatId, ws.id)
  }
  for (const chat of chats) {
    if (!chat.ownsWorktree || !chat.workspaceId) continue
    // The workspace half may name a DIFFERENT owner for the same workspace
    // (a stale cached `Workspace` from before a re-own). The one that already
    // agreed with a present `Workspace` record wins; this only ever ADDS the
    // pairs that record could not answer for.
    if (chatOfWorkspace.has(chat.workspaceId)) continue
    ownerOfChat.set(chat.id, chat.workspaceId)
  }
  return { chatOfWorkspace, ownerOfChat }
}

/**
 * Fold every workspace-owning chat into the ONE row its workspace already
 * renders as, instead of the two-row split `buildSidebarTree` produces on its
 * own — generalizes the locked-branch/repo-home withhold-and-consume pattern
 * (this file used to reserve for `type: 'branch'` chats only) to every fork
 * and every workspace-owning thread.
 *
 * Runs on the tree `buildSidebarTree` already built from UNFILTERED
 * workspaces and chats — never on its inputs. That matters: the workspace's
 * own `folderId`/`parentId` lineage is the one edge drag-and-drop actually
 * writes (`WorkspacePlacementWrite`), and it is what `buildSidebarTree`'s
 * folder-anchor-compatibility walk, cycle guard and sibling sort all already
 * resolve correctly for a NESTED fork — re-deriving any of that here, off a
 * pre-filtered input, would be exactly the "second pass" this file's own
 * header already argues against for chats generally. An owning chat's own
 * `parentId` is not trusted for the FOLDED row's placement at all: nothing on
 * the wire guarantees it agrees with the workspace's fork lineage (only that
 * the workspace's `Workspace.parentId` is what merge-eligibility and every
 * other placement-writing surface already commit to), so this only ever uses
 * a chat's position to find what hangs off it, never to decide where the
 * folded row itself renders.
 *
 * Two passes:
 *   1. `stripOwners` removes every owning-chat node whose WORKSPACE NODE IS
 *      ALSO IN THIS TREE, from wherever it sits — nested under the workspace
 *      it owns, when it has no thread-parent of its own (the common case, and
 *      the exact shape of the original bug), or under its TRUE thread-parent
 *      elsewhere (rule 8's forked-thread case, already placed correctly by
 *      `buildSidebarTree`'s own `filed` rule) — and records it, real children
 *      included, by id. Its own position is discarded here; pass 2 always
 *      re-attaches its children onto the workspace it owns, so nothing is
 *      spliced back into its old spot.
 *   2. `mergeWorkspaces` relabels every workspace node that has a resolved
 *      owner with that owner's id, and appends the owner's own (already
 *      stripped) children onto its own remaining ones. A workspace with no
 *      resolvable owner is left completely untouched.
 *
 * "WHOSE WORKSPACE NODE IS ALSO IN THIS TREE" is the whole correctness
 * condition, and it used to be assumed rather than checked. An owning chat
 * whose `Workspace` record has not arrived (a fresh fork) or has been hidden
 * out from under it (the removal tray, mid-countdown) has nothing to merge
 * ONTO — so stripping it would delete the row outright, and NOT stripping it
 * left it to `walk` as a `kind: 'chat'` bubble. Neither is right: the row
 * still exists and is still a workspace. It stays exactly where
 * `buildSidebarTree` put it and `walk` draws it as a `branch` row off the
 * chat alone (see `branchRowFromChat`), with only the decoration the absent
 * half carried missing. That is why this takes the whole {@link Ownership}
 * rather than one map: pass 1 needs to know which owners actually have a
 * workspace node to be folded into.
 */
export function foldOwningChats(
  roots: SidebarTreeNode[],
  { chatOfWorkspace }: Ownership,
): SidebarTreeNode[] {
  const foldableOwnerIds = new Set(chatOfWorkspace.values())
  const detached = new Map<string, SidebarTreeNode>()

  const stripOwners = (nodes: SidebarTreeNode[]): SidebarTreeNode[] => {
    const kept: SidebarTreeNode[] = []
    for (const node of nodes) {
      const children = stripOwners(node.children)
      if (node.kind === 'chat' && foldableOwnerIds.has(node.id)) {
        detached.set(node.id, { ...node, children })
        continue
      }
      kept.push({ ...node, children })
    }
    return kept
  }

  const mergeWorkspaces = (nodes: SidebarTreeNode[]): SidebarTreeNode[] =>
    nodes.map((node) => {
      const children = mergeWorkspaces(node.children)
      if (node.kind === 'workspace') {
        const ownerId = chatOfWorkspace.get(node.id)
        if (ownerId) {
          const owner = detached.get(ownerId)
          const ownerChildren = owner ? mergeWorkspaces(owner.children) : []
          return { ...node, id: ownerId, children: [...children, ...ownerChildren] }
        }
      }
      return { ...node, children }
    })

  return mergeWorkspaces(stripOwners(roots))
}

/**
 * Adapts today's Repo/Workspace/Folder/Chat shapes (`lib/store/sidebar.ts`)
 * into the flat `SidebarRow[]` `SidebarTree` renders — the bridge named in
 * Task 4. Deleted in Task 15 once rows arrive pre-shaped over the wire.
 *
 * Reuses `buildSidebarTree`'s placement, ordering and cycle-guard rules rather
 * than re-deriving them: the hierarchy this produces has to agree with the one
 * the tree it replaces already computes and tests. That is why CHATS are woven
 * in by handing them to that builder too, instead of being inserted into its
 * finished output here — a second pass would need its own cycle guard and its
 * own sibling sort, and a level interleaves all three row kinds on one shared
 * `order`, so a post-pass could not honour it without redoing the sort anyway.
 * {@link foldOwningChats} is the one deliberate exception: it runs AFTER that
 * builder, on its finished tree, purely to merge two nodes it built correctly
 * on their own terms into the one row a workspace-owning chat now is.
 *
 * The repo's default (main-worktree) workspace is not a row in
 * `repo.workspaces` — it becomes this tree's one root, exactly as it is the
 * repo header in the tree being retired. Everything `buildSidebarTree` roots
 * (no fork parent, no compatible folder, no resolvable chat edge) nests under
 * it; a repo with no default workspace yet simply has no root row for its own
 * rows to hang off.
 */
export function rowsFromRepo(repo: Repo): SidebarRow[] {
  const rows: SidebarRow[] = []
  const homeId = repo.defaultWorkspaceId ?? null

  // The LAST line of defence against the cross-repo bleed this area has hit
  // repeatedly (tasks 21/22/26/34). `toSidebarRepo` already keeps only this
  // repo's chats when it assembles the tree, but this is the render boundary:
  // a row that reaches here claiming another repo is drawn under this repo's
  // rows, and a chat rendered in the wrong repo is worse than one not drawn.
  const chats = (repo.chats ?? EMPTY_CHATS).filter((c) => c.repoId === repo.id)
  const chatTitleById = new Map(chats.map((c) => [c.id, c.title]))
  const workspaces = repo.workspaces.filter((w) => w.status !== 'deleted')
  const ownership = resolveOwnership(workspaces, chats)
  const { ownerOfChat } = ownership

  const homeRowId = homeId === null ? null : homeOwningChatId(chats, homeId)

  if (homeRowId !== null && homeId !== null) {
    rows.push({
      id: homeRowId,
      kind: 'branch',
      // A repo's own entry may be filed into a project-home folder (never one
      // of its own — see sidebar-drop-policy.ts's repo branch), which is what
      // lets it interleave with the project's home chats/folders instead of
      // always rooting the whole sidebar. '' is the project's home root,
      // normalised to null the same way every other row's does.
      parentId: repo.folderId || null,
      order: repo.order ?? 0,
      label: repo.name,
      // spec §3.4: the branch half of a workspace's provisional naming, not
      // just the chat-title half chat rows already carry below. The home
      // row's own LABEL is the repo's display name rather than its branch
      // (see `branchName` on the line below), so this only actually
      // italicizes anything for a freshly-seeded repo whose default branch
      // is still the server's generated placeholder — an imported repo's
      // real default branch never matches the pattern.
      labelProvisional: isProvisionalBranchName(repo.defaultBranch),
      ownsWorktree: true,
      workspaceId: homeId,
      working: repo.defaultWorking ?? false,
      hasView: false,
      branchName: repo.defaultBranch,
      locked: repo.defaultWorkspaceStatus === 'locked',
      // Only once the repo's owning project has seeded — see the field's own
      // doc on SidebarRow. Its icon route needs both ids.
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
    })
  }

  // UNFILTERED, on purpose, EXCEPT for the home row's own chat: every OTHER
  // workspace and chat goes in exactly as `buildSidebarTree` has always taken
  // them, so it places each one by its own folderId/parentId lineage or
  // parentId/workspaceId ground precisely as it always has — withholding
  // anything else here would be re-deriving placement rules this file already
  // defers to that builder for. The home row IS a real exception: the default
  // workspace this one chat owns is never a member of `repo.workspaces` (it
  // is this tree's one ROOT, pushed above, not a node inside it), so nothing
  // would ever resolve that chat's `ground` — left in, it would root itself a
  // second time, as an orphaned `chat` row floating beside the home row it
  // already IS.
  const roots = buildSidebarTree(
    workspaces,
    repo.folders ?? EMPTY_FOLDERS,
    chats.filter((c) => c.id !== homeRowId),
  )
  const folded = foldOwningChats(roots, ownership)

  walkTreeIntoRows(rows, folded, homeRowId, ownerOfChat, chatTitleById, true)

  return rows
}

/**
 * Walk a built (and, for `rowsFromRepo`, owner-folded) tree into flat
 * `SidebarRow`s, pushed onto `rows` in place.
 *
 * Extracted so `rows-from-home.ts` can draw a project-home tree's chats and
 * folders with the identical row shape a repo's produces — the two trees
 * differ only in what roots them (a repo's default workspace vs. a project's
 * home workspace, pushed by each caller's own home-row logic), never in how a
 * chat, folder or owning-chat-folded workspace becomes a row once the tree is
 * built. `ownerOfChat`/`chatTitleById` come from the caller's own
 * {@link resolveOwnership} / chat list — this function reads them, never
 * resolves them, so a caller with no `Workspace[]` at all (project home can
 * never be forked) can still pass an ownership map derived from an empty one.
 *
 * `foldersCanFork` answers the one thing a folder's own row otherwise can't
 * tell about itself: whether it sits under a real git repo at all. A repo
 * folder's "+" always forks a branch (there is a worktree to clone); a
 * project-home folder's cannot (there is no repo, so no worktree exists to
 * fork) — passed down from each caller's own root call and carried unchanged
 * through every recursive one, since a folder nested inside another folder
 * is still in the same repo-or-home tree its ancestor is.
 *
 * Each row's own `order` is its REAL wire value (`node.chat.order` /
 * `.folder.order` / `.workspace.order`), not `nodes`' own array position —
 * `index` is only a fallback for the rare row with no order field at all.
 * This is load-bearing at project-home's root level, where this function's
 * output is concatenated with a repo's own header row
 * (`rowsFromRepo`/`space-scroller.tsx`'s `SpacePanel`): Task 3 put a repo's
 * `order` on the same server-computed scale as its real home chat/folder
 * siblings, so the two only interleave correctly here if a chat/folder row
 * carries that same real scale through too, rather than a LOCAL 0..n-1
 * index compacted from whatever subset of siblings happened to reach this
 * one `buildSidebarTree` call (which, at the home root, never includes a
 * repo — see `rows-from-home.ts`'s own doc).
 */
export function walkTreeIntoRows(
  rows: SidebarRow[],
  nodes: SidebarTreeNode[],
  parentId: string | null,
  ownerOfChat: ReadonlyMap<string, string>,
  chatTitleById: ReadonlyMap<string, string>,
  foldersCanFork: boolean,
): void {
  nodes.forEach((node, index) => {
    if (node.kind === 'chat') {
      const order = node.chat.order ?? index
      // A chat that OWNS a worktree is a workspace row, and it says so
      // itself — it does not need its `Workspace` record to be on hand to
      // be one. Reaching `walk` still holding that ownership means exactly
      // one thing: `foldOwningChats` found no workspace NODE to merge it
      // onto, because the workspace half is missing right now (a fork whose
      // `WorkspaceDTO` has not landed; a delete holding the workspace hidden
      // for its eight-second countdown). The row is unchanged and still a
      // workspace, so it draws as one — same kind, same glyph, same id, same
      // Fork/Thread verbs — and only the DECORATION that lives on the absent
      // half (branch name, diff counts, lock) is left off until it arrives.
      //
      // Drawing a `chat` bubble here instead is what made a branch row turn
      // into a conversation mid-delete and a fresh fork appear as a bubble.
      const ownedWorkspaceId = ownerOfChat.get(node.id)
      if (ownedWorkspaceId !== undefined) {
        rows.push({
          id: node.id,
          kind: 'branch',
          parentId,
          order,
          label: node.chat.title || UNTITLED_CHAT_LABEL,
          labelProvisional: !node.chat.title,
          ownsWorktree: true,
          workspaceId: ownedWorkspaceId,
          working: false,
          hasView: false,
          // No `branchName`/`added`/`deleted`/`locked`: every one of those
          // is a `Workspace` field, and this row exists precisely because
          // that record is not here. Omitted rather than faked — a row
          // claiming `locked: false` for a branch that is actually locked
          // would offer verbs the daemon then refuses.
        })
        walkTreeIntoRows(rows, node.children, node.id, ownerOfChat, chatTitleById, foldersCanFork)
        return
      }
      rows.push({
        id: node.id,
        kind: 'chat',
        parentId,
        order,
        // A chat is born unnamed and every surface has to call that the same
        // thing (see UNTITLED_CHAT_LABEL) — the tree row and the pane tab
        // disagreeing reads as two different chats.
        label: node.chat.title || UNTITLED_CHAT_LABEL,
        labelProvisional: !node.chat.title,
        // §3.1: a chat NEVER owns a worktree — a worktree chat owns a
        // workspace, which is a different fact and a different row. This is
        // what puts the chat bubble on the row and makes its "+" a thread.
        // Reaching here means the chat asserted no ownership of its own AND
        // no `Workspace` claimed it (see `resolveOwnership`) — a real bubble,
        // not a workspace whose other half is late.
        ownsWorktree: false,
        workspaceId: node.chat.workspaceId ?? null,
        // ALWAYS FALSE HERE, AND NOT AN OVERSIGHT — but no longer the last
        // word. Seeding a real value into the row object is the latch this
        // has always refused to build: the only live path THIS function has
        // is a full repo-scoped reseed, which on the hottest frames in the
        // app (turn_started/turn_stopped) is a request storm, and a value
        // seeded once leaves the flip-dot spinner running on a chat whose
        // turn ended minutes ago.
        //
        // The answer that note always pointed at — "anything else that comes
        // to need real turn state here must subscribe per row the way Recents
        // does" — is now built: `sidebar-tree.tsx`'s `SidebarTreeRow`
        // subscribes each row to `subscribeChatWorking`/`readChatWorking` and
        // overrides this field, exactly as it already did for `hasView`. So
        // this stays an inert default that the render layer always replaces,
        // rather than a second, staler opinion about the same fact.
        //
        // (`sidebar-drop-policy.ts`'s chat branch still calls `isChatWorking`
        // at drag time; it acts outside render and has no row to read.)
        working: false,
        hasView: false,
      })
      walkTreeIntoRows(rows, node.children, node.id, ownerOfChat, chatTitleById, foldersCanFork)
      return
    }
    if (node.kind === 'folder') {
      rows.push({
        id: node.id,
        kind: 'folder',
        parentId,
        order: node.folder.order ?? index,
        label: node.folder.name,
        // A folder's own "+" forks a branch — but only when it sits under a
        // real repo. A project-home folder has no worktree to fork at all
        // (see `foldersCanFork`'s own doc), so its "+" would otherwise offer
        // a verb the daemon has nothing to do with.
        ownsWorktree: foldersCanFork,
        workspaceId: null,
        working: false,
        hasView: false,
      })
    } else {
      // `node.kind === 'workspace'`. `foldOwningChats` relabels `node.id`
      // to the id of the chat that owns this workspace whenever one was
      // resolvable — a locked branch, a repo home (never reached here; see
      // the home row above) and an ordinary fork alike — so `node.id` IS
      // this row's real identity by the time `walk` sees it; the id space
      // the daemon places every create under either way (rule 8's fork
      // button reads it straight off `Workspace.owningChatId`, matching
      // `space-content-actions.ts`'s `handleCreate`). A workspace that
      // folded no owner (an absent `owningChatId`, or one naming a chat
      // this repo has not seeded) is left exactly as `buildSidebarTree`
      // built it: `node.id` is still its own workspace id.
      //
      // `node.workspace.id`, never `node.id`, for the row's OWN
      // `workspaceId` field below — the two can now disagree by design.
      const hasOwner = node.id !== node.workspace.id
      const locked = node.workspace.status === 'locked'
      // Rule 6: a folded row's title IS the chat's, with branch/diff moved
      // to the second line (`branchName`/`added`/`deleted` below) — a
      // locked branch stays branch-labelled (addendum rules 1-4's "Folder
      // mechanism": unchanged, not a chat by another name). A workspace
      // that folded no owner has no chat title to borrow either.
      const chatTitle = hasOwner ? (chatTitleById.get(node.id) ?? '') : undefined
      const label =
        chatTitle !== undefined && !locked
          ? chatTitle || UNTITLED_CHAT_LABEL
          : node.workspace.branch
      const labelProvisional =
        chatTitle !== undefined && !locked
          ? !chatTitle
          : isProvisionalBranchName(node.workspace.branch)
      rows.push({
        id: node.id,
        kind: 'branch',
        parentId,
        order: node.workspace.order ?? index,
        label,
        labelProvisional,
        ownsWorktree: true,
        workspaceId: node.workspace.id,
        working: node.workspace.working ?? false,
        hasView: false,
        branchName: node.workspace.branch,
        added: node.workspace.added,
        deleted: node.workspace.deleted,
        locked,
      })
      // Children hang off the row's OWN id, which is now the owning chat's
      // (when one was resolved) — a thread the daemon filed under the
      // workspace still has to arrive at the row the user can see.
      walkTreeIntoRows(rows, node.children, node.id, ownerOfChat, chatTitleById, foldersCanFork)
      return
    }
    walkTreeIntoRows(rows, node.children, node.id, ownerOfChat, chatTitleById, foldersCanFork)
  })
}
