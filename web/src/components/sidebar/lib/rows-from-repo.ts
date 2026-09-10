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
 * The chat that owns a repo/project HOME workspace's worktree, straight off
 * `defaultOwningChatId` — the direct field `rows-from-repo.ts`/`toSidebarRepo`
 * lift from `WorkspaceDTO.owningChatId` for exactly this row, since the home
 * workspace is never a `Workspace` row (it lives on `repo.defaultWorkspaceId`,
 * outside `repo.workspaces` entirely) for {@link resolveOwnerChats} to read
 * directly the way it does for every other row.
 *
 * `defaultOwningChatId` is absent only for a caller with no `Repo` to lift it
 * from at all (`rows-from-home.ts`'s project home, which has no `Repository`
 * either) or a frame that predates the field — both fall back to the same
 * `ownsWorktree` signal {@link resolveOwnerChats} uses for every other row's
 * degrade case. Never a `Chat.type` check: Task 9 stopped minting/retyping a
 * `'branch'`-typed chat for this row, so nothing here may depend on it.
 *
 * Never throws. A workspace whose owning chat has not resolved by either
 * channel yet degrades exactly like any other unfolded workspace-owning row
 * below: left as its own raw id rather than dropped, so a row the user can
 * see stays visible while the daemon catches up.
 */
export function resolveHomeOwnerId(
  homeId: string,
  defaultOwningChatId: string | undefined,
  chats: readonly Chat[],
): string {
  if (defaultOwningChatId) return defaultOwningChatId
  const owner = chats.find((c) => c.ownsWorktree && c.workspaceId === homeId)
  return owner?.id ?? homeId
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
 * Every workspace's owning chat, keyed by workspace id — CONFIRMED pairs
 * only, straight off the direct field (2026-09-08 sidebar-placement-
 * unification Task 7/9): every workspace mints its owning chat chat-first,
 * atomically, at creation (`MintOwningChat`/`AttachOwningWorkspace`), and the
 * daemon resolves `Workspace.owningChatId` off it on every `WorkspaceDTO` it
 * sends (`domain.ResolveOwningChat`). No boot backfill races this any more,
 * and the chat is never retyped to mark it (`ChatType` narrows to
 * `'chat'|'workflow'` — see that type's own doc), so this never compares
 * `Chat.type`.
 *
 * "Confirmed" means the WORKSPACE side is present — this is
 * {@link foldWorkspaceOwners}'s whole input, and it deliberately excludes a
 * chat's own unconfirmed `ownsWorktree` claim (see
 * {@link resolveOwnerOfChat}): folding requires a real workspace NODE to fold
 * onto, and a workspace this repo has not seeded has none. A chat whose
 * workspace is merely absent from `workspaces` (deleted, or its own
 * `WorkspaceDTO` has not landed yet) must NOT be treated as foldable, or
 * `foldWorkspaceOwners` strips it from the tree with nowhere to put it back —
 * the exact regression `walkTreeIntoRows`'s own chat branch exists to render
 * instead (see `removal-countdown-rows.test.ts`'s "still draws a branch row
 * off the chat alone").
 */
export function resolveOwnerChats(
  workspaces: readonly Workspace[],
  chats: readonly Chat[],
): Map<string, string> {
  const chatIds = new Set(chats.map((c) => c.id))
  const ownerChats = new Map<string, string>()
  for (const ws of workspaces) {
    if (ws.owningChatId && chatIds.has(ws.owningChatId)) ownerChats.set(ws.id, ws.owningChatId)
  }
  return ownerChats
}

/**
 * Chat id -> the workspace it owns, present or not — the WIDER set
 * {@link resolveOwnerChats} deliberately excludes. The authority
 * `walkTreeIntoRows` draws a `branch` row off even when that workspace's own
 * `Workspace` record has not arrived yet (a fresh fork) or is hidden
 * mid-delete: a chat this map names IS a workspace row, full stop, and only
 * its DECORATION (branch name, diff counts, lock) depends on the missing
 * half.
 *
 * Starts from {@link resolveOwnerChats}'s CONFIRMED pairs (reversed), then
 * adds `Chat.ownsWorktree` + `Chat.workspaceId` for whatever workspace that
 * left unconfirmed — delivered atomically WITH the chat, on a separately
 * streamed channel (`crowbar_chats` reseeds on its own folder-signal bump,
 * `crowbar_workspaces` on its own entity stream), so a row still renders as a
 * workspace even when its `WorkspaceDTO` is the half running late. A chat's
 * `workspaceId` alone can NOT stand in for `ownsWorktree`: a thread carries
 * its parent's, so it names a worktree it does not own.
 */
export function resolveOwnerOfChat(
  ownerChats: ReadonlyMap<string, string>,
  chats: readonly Chat[],
): Map<string, string> {
  const ownerOfChat = new Map<string, string>()
  for (const [wsId, chatId] of ownerChats) ownerOfChat.set(chatId, wsId)
  for (const chat of chats) {
    if (!chat.ownsWorktree || !chat.workspaceId || ownerChats.has(chat.workspaceId)) continue
    ownerOfChat.set(chat.id, chat.workspaceId)
  }
  return ownerOfChat
}

/**
 * Fold every workspace-owning chat into the ONE row its workspace already
 * renders as, instead of the two-row split `buildSidebarTree` produces on its
 * own (product rule 6: "a chat with a workspace" is ONE row, not two).
 *
 * Runs on the tree `buildSidebarTree` already built from UNFILTERED
 * workspaces and chats — never on its inputs. That matters: a workspace's own
 * `folderId`/`parentId` lineage is the one edge drag-and-drop actually writes
 * (`WorkspacePlacementWrite`), and `buildSidebarTree`'s folder-anchor
 * compatibility walk, cycle guard and sibling sort already resolve it
 * correctly for a NESTED fork — an owning chat's own `parentId` is used only
 * to find what hangs off it (a real thread), never to decide where the
 * folded row itself renders.
 *
 * Two passes:
 *   1. `stripOwners` removes every owning-chat node whose workspace is ALSO
 *      in this tree, from wherever it sits, and records it (real children
 *      included) by id. Pass 2 always re-attaches its children onto the
 *      workspace it owns, so nothing is spliced back into its old spot.
 *   2. `mergeWorkspaces` relabels every workspace node with a resolved owner
 *      to that owner's id, appending the owner's own (stripped) children
 *      onto its own. A workspace with no resolvable owner — its owning chat
 *      has not arrived, or names a chat this repo has not seeded — is left
 *      completely untouched, exactly as `buildSidebarTree` built it: still a
 *      `branch` row (see `walkTreeIntoRows`'s workspace branch), just missing
 *      the title/decoration only the chat half carries.
 */
export function foldWorkspaceOwners(
  roots: SidebarTreeNode[],
  ownerChats: ReadonlyMap<string, string>,
): SidebarTreeNode[] {
  const foldableOwnerIds = new Set(ownerChats.values())
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
        const ownerId = ownerChats.get(node.id)
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
 * {@link foldWorkspaceOwners} is the one deliberate exception: it runs AFTER that
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
  const ownerChats = resolveOwnerChats(workspaces, chats)
  const ownerOfChat = resolveOwnerOfChat(ownerChats, chats)

  const homeRowId =
    homeId === null ? null : resolveHomeOwnerId(homeId, repo.defaultOwningChatId, chats)

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
  const folded = foldWorkspaceOwners(roots, ownerChats)

  walkTreeIntoRows(rows, folded, homeRowId, ownerOfChat, chatTitleById, true, homeId)

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
 * built. `ownerOfChat`/`chatTitleById` come from the caller's own owner
 * resolution / chat list — this function reads them, never resolves them, so
 * a caller with no `Workspace[]` at all (project home can never be forked)
 * can still pass an owner map derived from an empty one.
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
 *
 * `ancestorWorkspaceId` is the OTHER thing a folder's own row can't tell
 * about itself: which real workspace it actually sits inside, for a Thread
 * button to run in (`space-content-actions.ts`'s `handleCreate` reads a
 * folder row's own `workspaceId` for exactly this — see that field's stamp
 * below). Starts as each caller's tree root (`rowsFromRepo`: the repo's real
 * default workspace id; `rows-from-home.ts`: the project's home workspace
 * id) and is updated only when the walk descends into a row that owns a
 * REAL workspace of its own (a `branch` row, folded owner included) — a
 * `chat` bubble or a `folder` passes it through unchanged, exactly like
 * `foldersCanFork`, since neither introduces a worktree of its own for a
 * nested folder to belong to instead.
 */
export function walkTreeIntoRows(
  rows: SidebarRow[],
  nodes: SidebarTreeNode[],
  parentId: string | null,
  ownerOfChat: ReadonlyMap<string, string>,
  chatTitleById: ReadonlyMap<string, string>,
  foldersCanFork: boolean,
  ancestorWorkspaceId: string | null,
): void {
  nodes.forEach((node, index) => {
    if (node.kind === 'chat') {
      const order = node.chat.order ?? index
      // A chat that OWNS a worktree is a workspace row, and it says so
      // itself — it does not need its `Workspace` record to be on hand to
      // be one. Reaching `walk` still holding that ownership means exactly
      // one thing: `foldWorkspaceOwners` found no workspace NODE to merge it
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
        walkTreeIntoRows(
          rows,
          node.children,
          node.id,
          ownerOfChat,
          chatTitleById,
          foldersCanFork,
          ownedWorkspaceId,
        )
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
        // no `Workspace` claimed it (see `resolveOwnerChats`) — a real bubble,
        // not a workspace whose other half is late.
        ownsWorktree: false,
        workspaceId: node.chat.workspaceId ?? null,
        // `foldersCanFork` says the SAME thing for a chat's Fork button that
        // it already says for a folder's: whether this tree sits under a
        // real repo at all. A repo-scoped bubble always does (`true` here);
        // `rows-from-home.ts` passes `false` for a project-home one, which
        // has no worktree for Fork to clone regardless of what its ground
        // workspace resolves to.
        canFork: foldersCanFork,
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
      walkTreeIntoRows(
        rows,
        node.children,
        node.id,
        ownerOfChat,
        chatTitleById,
        foldersCanFork,
        ancestorWorkspaceId,
      )
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
        // The real workspace this folder sits inside — `handleCreate`'s
        // Thread branch reads this straight off the row rather than
        // re-walking the tree itself. Always resolvable: even a root-level
        // folder inherits its tree's own root (the repo's default workspace,
        // or the project's home workspace), never null in practice for a
        // folder that reaches here at all.
        workspaceId: ancestorWorkspaceId,
        working: false,
        hasView: false,
      })
    } else {
      // `node.kind === 'workspace'`. `foldWorkspaceOwners` relabels `node.id`
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
      //
      // `node.workspace.id` (the RAW workspace id), not `node.id` — a
      // folder nested under a locked/folded branch still has to resolve to
      // the real workspace `createChat` posts to, not the owning chat's id.
      walkTreeIntoRows(
        rows,
        node.children,
        node.id,
        ownerOfChat,
        chatTitleById,
        foldersCanFork,
        node.workspace.id,
      )
      return
    }
    walkTreeIntoRows(
      rows,
      node.children,
      node.id,
      ownerOfChat,
      chatTitleById,
      foldersCanFork,
      ancestorWorkspaceId,
    )
  })
}
