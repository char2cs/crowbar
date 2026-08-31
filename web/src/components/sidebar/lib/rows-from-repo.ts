import { EMPTY_CHATS, EMPTY_FOLDERS, type Repo } from '@/lib/store/sidebar'
import { buildSidebarTree, type SidebarTreeNode } from '@/components/layout/workspace-tree-utils'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

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

  if (homeId) {
    rows.push({
      id: homeId,
      kind: 'branch',
      parentId: null,
      order: repo.order ?? 0,
      label: repo.name,
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
    // The LAST line of defence against the cross-repo bleed this area has hit
    // repeatedly (tasks 21/22/26/34). `toSidebarRepo` already keeps only this
    // repo's chats when it assembles the tree, but this is the render boundary:
    // a row that reaches here claiming another repo is drawn under this repo's
    // rows, and a chat rendered in the wrong repo is worse than one not drawn.
    (repo.chats ?? EMPTY_CHATS).filter((c) => c.repoId === repo.id),
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
        rows.push({
          id: node.id,
          kind: 'branch',
          parentId,
          order,
          label: node.workspace.branch,
          ownsWorktree: true,
          workspaceId: node.id,
          working: node.workspace.working ?? false,
          hasView: false,
          branchName: node.workspace.branch,
        })
      }
      walk(node.children, node.id)
    })
  }
  walk(roots, homeId)

  return rows
}
