import { indexFromParents, type TreeIndex } from './keep-set'
import type { Workspace } from '@/lib/store/sidebar'

/**
 * A folder as the sidebar needs it. Organisation only — no branch, no git
 * status, no worktree. Structurally compatible with the store's folder type;
 * declared here so the tree builder does not depend on the store.
 */
export interface SidebarFolder {
  id: string
  name: string
  /** A workspace id, another folder id, or undefined for the repo root. */
  parentId?: string
  order?: number
}

/**
 * The two fields the redesign adds to a workspace. Written as an intersection
 * rather than assumed on `Workspace` so this compiles whether or not the store
 * has caught up, and so a frame that predates them is simply undefined here.
 */
export type PlacedWorkspace = Workspace & { folderId?: string; order?: number }

/**
 * A chat as the sidebar tree needs it — design spec §3.1's `chat` row.
 *
 * ONE edge, unlike a workspace's two: a chat has no branch, so it has no fork
 * lineage a folder edge could split, and `parentId` is the whole answer (§3.2).
 * `workspaceId` is not a second edge either — it is the row's GROUND, and it
 * only ever places the row when `parentId` names nothing, which is what "'' is
 * the workspace root" means on the wire.
 *
 * Declared here rather than imported so the tree builder does not depend on the
 * store, exactly as `SidebarFolder` above is. `lib/store/sidebar.ts`'s `Chat`
 * satisfies it structurally.
 */
export interface SidebarChat {
  id: string
  /** Another chat (this row is a thread of it), or a folder. */
  parentId?: string
  /** The workspace this chat owns; absent on a bubble, which owns none. */
  workspaceId?: string
  title: string
  order?: number
}

export type SidebarTreeNode =
  | { kind: 'workspace'; id: string; workspace: PlacedWorkspace; children: SidebarTreeNode[] }
  | { kind: 'folder'; id: string; folder: SidebarFolder; children: SidebarTreeNode[] }
  | { kind: 'chat'; id: string; chat: SidebarChat; children: SidebarTreeNode[] }

/** Missing order sorts last, and ties keep arrival order via the index tiebreak. */
const NO_ORDER = Number.MAX_SAFE_INTEGER

/**
 * Build the sidebar tree from a repo's workspaces, folders and chats.
 *
 * `chats` is optional and defaults to none, so the two callers that place only
 * branches and folders (`drop-actions.ts`, `removal-plan.ts`) are unchanged.
 *
 * A CHAT is placed by one rule, and it is deliberately not the workspace rule
 * below: a chat carries no branch, so it has no fork lineage for a folder edge
 * to split and nothing to check anchors for.
 *
 *   1. `parentId` — another chat (this row is a thread of it) or a folder.
 *   2. `workspaceId` — its GROUND, used only when `parentId` names nothing.
 *      This is what "'' is the workspace root" means on the wire: a chat filed
 *      nowhere sits at the root of the workspace it owns.
 *   3. otherwise the repo root — which is where a bubble whose ancestry lives in
 *      another repo lands (spec §9.2: a repo's chats are not a closed set, so an
 *      edge that resolves to nothing here is ordinary, not corrupt). Rendered at
 *      the root beats dropped: a row the user can see is recoverable.
 *
 * A WORKSPACE's placement follows one rule, in this order:
 *
 *   1. `folderId` — for a fork ROOT, always; for a forked child, when that
 *      folder lives in the same visible fork-parent space. A child of `develop`
 *      may therefore sit in a folder that also hangs off `develop`, while
 *      `parentId` still records its real lineage. A fork root has no lineage to
 *      protect, so it goes wherever it is filed.
 *   2. `parentId` — the FORK parent. An incompatible or stale folder edge is
 *      ignored rather than letting organisation visually split a fork chain.
 *   3. otherwise the repo root.
 *
 * Siblings sort by `order`, then by arrival, so an un-ordered repo (nothing
 * dragged yet) keeps exactly the order the backend sent.
 *
 * Any edge that would close a cycle is dropped and the node is re-rooted, so a
 * corrupt parent chain degrades to a flat list rather than hanging the render.
 */
export function buildSidebarTree(
  workspaces: PlacedWorkspace[],
  folders: SidebarFolder[] = [],
  chats: SidebarChat[] = [],
): SidebarTreeNode[] {
  const nodes = new Map<string, SidebarTreeNode>()
  // Arrival index per id, so the sibling sort has a stable tiebreak that does
  // not depend on Array.prototype.sort's stability guarantee.
  const arrival = new Map<string, number>()
  // The resolved parent of each node, used for cycle detection below.
  const parentOf = new Map<string, string | undefined>()

  let seen = 0
  for (const folder of folders) {
    nodes.set(folder.id, { kind: 'folder', id: folder.id, folder, children: [] })
    arrival.set(folder.id, seen++)
  }
  for (const ws of workspaces) {
    nodes.set(ws.id, { kind: 'workspace', id: ws.id, workspace: ws, children: [] })
    arrival.set(ws.id, seen++)
  }
  // Chats arrive LAST, so an untouched level — every row still holding order 0,
  // which is the whole tree on a daemon that has placed nothing — reads
  // folders, then branches, then chats. Same convention `chat-rows.ts` sorts
  // the Chats panel by ("folders sort above every chat"), so the two surfaces
  // cannot disagree about a level nobody has dragged.
  for (const chat of chats) {
    nodes.set(chat.id, { kind: 'chat', id: chat.id, chat, children: [] })
    arrival.set(chat.id, seen++)
  }

  for (const folder of folders) parentOf.set(folder.id, folder.parentId || undefined)
  // Resolved BEFORE the workspace loop below, because `folderWorkspaceAnchor`
  // walks this map: a folder nested inside a chat has to step through that chat
  // to find the workspace whose sibling space it really sits in.
  for (const chat of chats) {
    const filed = chat.parentId && nodes.has(chat.parentId) ? chat.parentId : undefined
    const ground = chat.workspaceId && nodes.has(chat.workspaceId) ? chat.workspaceId : undefined
    parentOf.set(chat.id, filed ?? ground)
  }

  // The workspace that owns a folder's visible sibling space. Root-level
  // folders have no anchor. Walking rather than looking only at the immediate
  // parent lets nested folders remain useful without weakening the lineage
  // check.
  //
  // Steps through CHAT rows as well as folders — a folder may live inside a
  // chat (it orders that chat's threads), and stopping there would report no
  // anchor for a folder that plainly sits inside one workspace's space.
  const folderWorkspaceAnchor = (folderId: string): string | undefined => {
    const visited = new Set<string>()
    let cursor: string | undefined = folderId
    while (cursor && !visited.has(cursor)) {
      visited.add(cursor)
      const node = nodes.get(cursor)
      if (!node) return undefined
      if (node.kind === 'workspace') return node.id
      cursor = parentOf.get(cursor)
    }
    return undefined
  }

  for (const ws of workspaces) {
    // A parent absent from this rendered repo (for example the hidden repo-home
    // workspace) is the visible root, matching a root folder's empty anchor.
    const forkParent = ws.parentId && nodes.has(ws.parentId) ? ws.parentId : undefined
    const folderParent = ws.folderId && nodes.has(ws.folderId) ? ws.folderId : undefined
    // A row with no visible fork parent is a fork ROOT, and a fork root has no
    // chain for a folder to split — so its folder is honoured wherever that
    // folder sits. Comparing anchors for these too admitted them into root-level
    // folders ONLY, which silently re-rooted every workspace created on a folder
    // row nested under a workspace: the daemon files such a create as a fork root
    // (folderId set, parentId empty — the pairing is mutually exclusive there),
    // so the edge was dropped and the row appeared outside the folder it was
    // created in. Rows that DO have a fork parent still require the anchors to
    // agree, which is what keeps organisation from splitting a fork chain.
    const compatibleFolder =
      folderParent &&
      (forkParent === undefined || folderWorkspaceAnchor(folderParent) === forkParent)
        ? folderParent
        : undefined
    parentOf.set(ws.id, compatibleFolder ?? forkParent)
  }

  const closesCycle = (id: string): boolean => {
    const visited = new Set<string>([id])
    let cursor = parentOf.get(id)
    while (cursor) {
      if (visited.has(cursor)) return true
      visited.add(cursor)
      cursor = parentOf.get(cursor)
    }
    return false
  }

  const roots: SidebarTreeNode[] = []
  for (const [id, node] of nodes) {
    const parentId = parentOf.get(id)
    const parent = parentId ? nodes.get(parentId) : undefined
    if (!parent || parent === node || closesCycle(id)) roots.push(node)
    else parent.children.push(node)
  }

  // One sibling space, one sort key: a level interleaves folders, branches and
  // chats and they all sort on the SAME dense `order` (spec §3.1; the backend's
  // AgentChatDTO.order says so in as many words).
  const orderOf = (node: SidebarTreeNode) =>
    (node.kind === 'folder'
      ? node.folder.order
      : node.kind === 'chat'
        ? node.chat.order
        : node.workspace.order) ?? NO_ORDER
  const sortSiblings = (list: SidebarTreeNode[]) => {
    list.sort(
      (a, b) => orderOf(a) - orderOf(b) || (arrival.get(a.id) ?? 0) - (arrival.get(b.id) ?? 0),
    )
    for (const child of list) sortSiblings(child.children)
  }
  sortSiblings(roots)

  return roots
}

/**
 * One repo's rows, plus the lookups the keep set needs over them.
 *
 * Built once per repo alongside the tree itself rather than per row: a
 * collapsed parent has to answer "what am I holding?" from its descendants, and
 * asking each row to walk the graph for itself would make a keep-set change cost
 * O(rows × depth).
 *
 * Parents are read back off the BUILT tree, not off the raw workspace records,
 * so the index describes what is actually drawn — cycle re-rooting included.
 */
export interface SidebarRepoTree {
  /** The repo's root rows, in order. */
  roots: SidebarTreeNode[]
  /** Descendant / ancestor questions, for `keep-set.ts`. */
  index: TreeIndex
  /** The node behind an id, for rendering a kept row under a collapsed parent. */
  nodeById: ReadonlyMap<string, SidebarTreeNode>
  /** The row each row hangs off; roots hang off `rootParentId`. */
  parentById: ReadonlyMap<string, string>
}

/**
 * Index a repo's built tree. `rootParentId` is the repo id, so a collapsed repo
 * header can ask the same descendant question a collapsed row does.
 */
export function indexSidebarTree(roots: SidebarTreeNode[], rootParentId: string): SidebarRepoTree {
  const nodeById = new Map<string, SidebarTreeNode>()
  const parentById = new Map<string, string>()

  const walk = (list: SidebarTreeNode[], parentId: string) => {
    for (const node of list) {
      nodeById.set(node.id, node)
      parentById.set(node.id, parentId)
      walk(node.children, node.id)
    }
  }
  walk(roots, rootParentId)

  return { roots, index: indexFromParents(parentById), nodeById, parentById }
}

/** An empty repo tree, stable so an absent repo does not remount the rows. */
export const EMPTY_REPO_TREE: SidebarRepoTree = indexSidebarTree([], '')
