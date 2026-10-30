import type { Repo } from '@/lib/store/sidebar'
import { EMPTY_CHATS, EMPTY_FOLDERS } from '@/lib/store/repo-tree'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { Problem } from '@/lib/errors/problem'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { workspaceIconFields } from '@/components/sidebar/lib/chat-icon-index'
import {
  foldWorkspaceOwners,
  resolveHomeOwnerId,
  resolveOwnerChats,
  resolveOwnerOfChat,
} from '@/components/sidebar/lib/workspace-owners'

/** The newest failure among the ids a row names (its workspace, its owning chat),
 *  looked up by the caller: the walk reads no store. */
export type ProblemLookup = (...entityIds: string[]) => Problem | null
const NO_PROBLEMS: ProblemLookup = () => null

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
 * `repo.workspaces` — enforced below, not assumed — it becomes this tree's one
 * root, exactly as it is the
 * repo header in the tree being retired. Everything `buildSidebarTree` roots
 * (no fork parent, no compatible folder, no resolvable chat edge) nests under
 * it; a repo with no default workspace yet simply has no root row for its own
 * rows to hang off.
 */
export function rowsFromRepo(repo: Repo, problemOf: ProblemLookup = NO_PROBLEMS): SidebarRow[] {
  const rows: SidebarRow[] = []
  const homeId = repo.defaultWorkspaceId ?? null

  // The LAST line of defence against the cross-repo bleed this area has hit
  // repeatedly (tasks 21/22/26/34). `toSidebarRepo` already keeps only this
  // repo's chats when it assembles the tree, but this is the render boundary:
  // a row that reaches here claiming another repo is drawn under this repo's
  // rows, and a chat rendered in the wrong repo is worse than one not drawn.
  const chats = (repo.chats ?? EMPTY_CHATS).filter((c) => c.repoId === repo.id)
  const chatTitleById = new Map(chats.map((c) => [c.id, c.title]))
  // The same last line of defence for the WORKSPACE half, and for the same
  // reason. "The default workspace is not a member of `repo.workspaces`" (see
  // this function's own doc, and the home-row exception below) is an invariant
  // nothing here used to enforce: a payload that does carry it — an import
  // mid-flight, before `defaultBranch` resolves — reached `walkTreeIntoRows`
  // as a second `branch` row beside the home row this function already pushed,
  // unlabeled, because the home chat is excluded from the tree so no owner can
  // fold onto it. Live-reported on an imported repo.
  const workspaces = repo.workspaces.filter((w) => w.status !== 'deleted' && w.id !== homeId)
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
      status: repo.defaultWorkspaceStatus,
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

  walkTreeIntoRows(
    rows,
    folded,
    homeRowId,
    ownerOfChat,
    chatTitleById,
    true,
    homeId,
    repo.defaultBranch,
    problemOf,
  )

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
 * button to run in (`create-actions.ts`'s `handleCreate` reads a
 * folder row's own `workspaceId` for exactly this — see that field's stamp
 * below). Starts as each caller's tree root (`rowsFromRepo`: the repo's real
 * default workspace id; `rows-from-home.ts`: the project's home workspace
 * id) and is updated only when the walk descends into a row that owns a
 * REAL workspace of its own (a `branch` row, folded owner included) — a
 * `chat` bubble or a `folder` passes it through unchanged, exactly like
 * `foldersCanFork`, since neither introduces a worktree of its own for a
 * nested folder to belong to instead.
 *
 * `ownDefaultBranch` is the THIRD fact of that kind: which branch this tree's
 * repo already has checked out in its own main folder, so a worktree-less row
 * on that branch reads as the repo's own checkout rather than as a failed
 * provision (`placeholderKind`). Undefined from `rows-from-home.ts`, whose
 * tree rides no repo and therefore owns no checkout to be held by.
 */
export function walkTreeIntoRows(
  rows: SidebarRow[],
  nodes: SidebarTreeNode[],
  parentId: string | null,
  ownerOfChat: ReadonlyMap<string, string>,
  chatTitleById: ReadonlyMap<string, string>,
  foldersCanFork: boolean,
  ancestorWorkspaceId: string | null,
  ownDefaultBranch: string | undefined,
  problemOf: ProblemLookup = NO_PROBLEMS,
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
          createdAt: node.chat.createdAt,
          label: node.chat.title || UNTITLED_CHAT_LABEL,
          labelProvisional: !node.chat.title,
          ownsWorktree: true,
          workspaceId: ownedWorkspaceId,
          working: false,
          hasView: false,
          // No `branchName`/`added`/`deleted`/`locked`: those are `Workspace`
          // fields, and that record is not here; faking `locked: false` would
          // offer verbs the daemon then refuses.
        })
        walkTreeIntoRows(
          rows,
          node.children,
          node.id,
          ownerOfChat,
          chatTitleById,
          foldersCanFork,
          ownedWorkspaceId,
          ownDefaultBranch,
          problemOf,
        )
        return
      }
      rows.push({
        id: node.id,
        kind: 'chat',
        parentId,
        order,
        createdAt: node.chat.createdAt,
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
        // Falls back to the nearest real ancestor workspace (the SAME value
        // a folder row's own `workspaceId` already gets — see
        // `ancestorWorkspaceId`'s own doc above) when the chat carries none
        // of its own. Without this, a bubble with no `Workspace` record of
        // its own — the overwhelmingly common case, e.g. `foldWorkspaceOwners`
        // resolves NO ownership for it — had a `workspaceId` of `null`, and
        // `open-actions.ts`'s `handleOpen` (which reads
        // exactly this field) then had nothing to open it into: a click just
        // toggled its (childless, so invisible) fold instead of ever landing
        // in a pane. Live-reported: "clicking on a not opened row... simply
        // anything happens... It should create a view on its own." A bubble
        // still asserts no ownership (`ownsWorktree: false` — that fact is
        // unchanged), it just runs the daemon's chats endpoint under the
        // ground workspace it has always conceptually "borrowed" (§3.2),
        // exactly as `handleCreate`'s Thread button already resolves for it.
        workspaceId: node.chat.workspaceId ?? ancestorWorkspaceId,
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
        ownDefaultBranch,
        problemOf,
      )
      return
    }
    if (node.kind === 'folder') {
      rows.push({
        id: node.id,
        kind: 'folder',
        parentId,
        order: node.folder.order ?? index,
        createdAt: node.folder.createdAt,
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
      // `create-actions.ts`'s `handleCreate`). A workspace with no
      // `owningChatId` at all is left exactly as `buildSidebarTree` built
      // it: `node.id` is still its own workspace id.
      //
      // `node.workspace.id`, never `node.id`, for the row's OWN
      // `workspaceId` field below — the two can now disagree by design.
      const hasOwner = node.id !== node.workspace.id
      const locked = node.workspace.status === 'locked'
      // Rule 6: a folded row's title IS the chat's, with branch/diff moved
      // to the second line (`branchName`/`added`/`deleted` below) — a
      // locked branch stays branch-labelled (addendum rules 1-4's "Folder
      // mechanism": unchanged, not a chat by another name). An owner whose
      // chat row has not seeded yet (undefined, not '') degrades to the
      // branch name rather than "Untitled chat".
      const chatTitle = hasOwner ? chatTitleById.get(node.id) : undefined
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
        createdAt: node.workspace.createdAt,
        label,
        labelProvisional,
        ownsWorktree: true,
        workspaceId: node.workspace.id,
        working: node.workspace.working ?? false,
        hasView: false,
        ...workspaceIconFields(
          node.workspace,
          ownDefaultBranch,
          problemOf(node.workspace.id, node.id),
        ),
      })
      // Children hang off the row's own id (its owning chat's) so they reach the
      // visible row; the RAW workspace id is the ancestor `createChat` posts to.
      walkTreeIntoRows(
        rows,
        node.children,
        node.id,
        ownerOfChat,
        chatTitleById,
        foldersCanFork,
        node.workspace.id,
        ownDefaultBranch,
        problemOf,
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
      ownDefaultBranch,
      problemOf,
    )
  })
}
