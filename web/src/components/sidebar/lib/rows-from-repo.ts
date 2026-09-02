import { EMPTY_CHATS, EMPTY_FOLDERS, type Chat, type Repo } from '@/lib/store/sidebar'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * Each workspace id to the id of the `branch` row that owns it.
 *
 * `branch` and nothing else, because it is the only owner a client can name
 * without guessing. The daemon mints one per locked branch, repo home and
 * project home (`tree/backfill.go`'s `owningChatType`) and never a second, and
 * its own tiebreak says a branch row always wins. Every OTHER chat carries a
 * workspace id too — `MintChat` stamps it on every conversation ever started
 * inside a worktree — so "some chat names this workspace" is a question with N
 * answers and no way to order them here: `ChatDTO` carries no `createdAt`,
 * which is the key the daemon's own `preferred()` sorts on.
 */
function branchRowIds(chats: readonly Chat[]): Map<string, string> {
  const owners = new Map<string, string>()
  for (const chat of chats) {
    if (chat.type === 'branch' && chat.workspaceId) owners.set(chat.workspaceId, chat.id)
  }
  return owners
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
  const ownerOf = branchRowIds(chats)
  // A branch-owning row's IDENTITY, which is not the same thing as the
  // workspace it draws. The daemon addresses every placement by CHAT id — "+"
  // on a locked branch or the repo home resolves its parent as a chat row — so
  // a row id'd from the `Workspace` named something the daemon has never heard
  // of. Only `id` moves: `branchName`, `working`, `ownsWorktree` and `repoIcon`
  // are facts about the WORKSPACE and still come from that record.
  //
  // Asked ONLY of a workspace the daemon guarantees a `branch` row for — the
  // repo home, and a locked branch — and there the absence of one is a real
  // break, not a shape to tolerate: the caller does not reach this function
  // until the repo's chat seed has landed (SidebarTreeSurface's
  // `seededRepoIds` gate), so "not yet" is already ruled out by the time the
  // question is asked.
  const branchRowIdFor = (workspaceId: string): string => {
    const chatId = ownerOf.get(workspaceId)
    if (!chatId) {
      throw new Error(`rowsFromRepo: workspace ${workspaceId} owns no branch row`)
    }
    return chatId
  }

  const homeRowId = homeId === null ? null : branchRowIdFor(homeId)

  if (homeRowId !== null) {
    rows.push({
      id: homeRowId,
      kind: 'branch',
      parentId: null,
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

  const roots = buildSidebarTree(
    repo.workspaces.filter((w) => w.status !== 'deleted'),
    repo.folders ?? EMPTY_FOLDERS,
    // Branch rows are withheld: one has already been consumed above as a
    // workspace row's identity, and weaving it in again would draw the SAME id
    // twice — once as the branch, once as a nameless chat nested inside it.
    chats.filter((c) => c.type !== 'branch'),
  )

  const walk = (nodes: SidebarTreeNode[], parentId: string | null) => {
    nodes.forEach((node, order) => {
      if (node.kind === 'chat') {
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
          ownsWorktree: false,
          workspaceId: node.chat.workspaceId ?? null,
          // ALWAYS FALSE, AND NOT AN OVERSIGHT. The tree answers "does this
          // exist" and Recents answers "what is up right now" (spec §5.7), and
          // a chat row has no cheap per-turn push to ride: the only live path
          // is a full repo-scoped reseed, which on the hottest frames in the
          // app (turn_started/turn_stopped) is a request storm. A value seeded
          // once would instead latch the flip-dot spinner on a chat whose turn
          // ended minutes ago, which is worse than not drawing one.
          //
          // The one place that genuinely needs the live answer asks for it
          // itself: `sidebar-drop-policy.ts`'s chat branch calls
          // `isChatWorking` at drag time, so refusing to drag a working chat
          // does NOT depend on this field. Anything else that comes to need
          // real turn state here must subscribe per row the way Recents does
          // (`recents-band.tsx`'s RecentsMemberRow) — never seed it into the
          // row object, which is the latch described above.
          working: false,
          hasView: false,
        })
        walk(node.children, node.id)
        return
      }
      if (node.kind === 'folder') {
        rows.push({
          id: node.id,
          kind: 'folder',
          parentId,
          order,
          label: node.folder.name,
          // A folder groups workspaces, not chats — its own "+" always forks a
          // branch, matching the tree being retired.
          ownsWorktree: true,
          workspaceId: null,
          working: false,
          hasView: false,
        })
      } else {
        // A LOCKED branch is a `branch` row and is addressed by the chat that
        // owns it. A regular fork is not: the daemon deliberately gives it a
        // `chat`-typed owner instead (`owningChatType`), and that row is
        // already drawn on its own beneath this one — taking its id here would
        // put the same id on two rows, one of them its own parent. So a regular
        // fork keeps the workspace id, and that is a statement about what the
        // row IS, not a stand-in for data that has not arrived.
        //
        // `node.workspace.owningChatId` names that conversation and is NOT
        // read here on purpose. Sourcing the row's id from it needs the chat
        // WITHHELD from the weave above to avoid the duplicate — which deletes
        // a real conversation row, the merge this file's own header defers to
        // the task that replaces it — and would drop the row into `chat` id
        // space, where `resolveChatRow` claims it (it passes everything that is
        // not `type: 'branch'`) and `workspaceIdOfBranchRow` does not: the "+"
        // goes inert, trash deletes the conversation, rename retitles it. The
        // one caller that needs the placement id reads it off the WORKSPACE
        // instead (`space-content-actions.ts`'s `handleCreate`).
        const rowId = node.workspace.status === 'locked' ? branchRowIdFor(node.id) : node.id
        rows.push({
          id: rowId,
          kind: 'branch',
          parentId,
          order,
          label: node.workspace.branch,
          // spec §3.4: here the row's LABEL *is* the branch name, so a
          // still-generated one renders italic directly — settles the
          // instant a real rename replaces it, same as the chat-title half
          // below.
          labelProvisional: isProvisionalBranchName(node.workspace.branch),
          ownsWorktree: true,
          workspaceId: node.id,
          working: node.workspace.working ?? false,
          hasView: false,
          branchName: node.workspace.branch,
        })
        // Children hang off the row's OWN id, which is now the owning chat's —
        // a thread the daemon filed under the workspace still has to arrive at
        // the row the user can see.
        walk(node.children, rowId)
        return
      }
      walk(node.children, node.id)
    })
  }
  walk(roots, homeRowId)

  return rows
}
