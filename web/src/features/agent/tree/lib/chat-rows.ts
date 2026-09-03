import {
  searchChats,
  type SearchNode,
  type SearchSets,
} from '@/features/agent/tree/lib/chat-search'

/**
 * The Chats panel's render list: one flat array of equal-height rows, each
 * carrying its own depth, built from the chats and folders the daemon sent.
 *
 * FLAT because every consumer wants it flat — the renderer maps it, the
 * virtualizer windows it, the drag indexes into it. A tree that has been
 * flattened into uniform rows is exactly what a virtualizer wants, which is why
 * 1000 chats cost the same as 20: the panel mounts the slice on screen and
 * nothing else. Nesting is expressed by `depth` and `parentId`, as it is in the
 * workspace tree beside it.
 *
 * ONE pass builds everything the panel needs — the rows and the sibling index a
 * drop lands into. Deriving either separately meant a second whole build per
 * keystroke, which is the difference between a filter that lands inside a frame
 * and one that does not.
 *
 * Three defences, none decorative:
 *
 *   - A row whose parent names nothing renders at the ROOT rather than
 *     vanishing. A chat that exists in the store and nowhere on screen is the
 *     worst failure this panel has, and "the parent's delete frame arrived
 *     first" is an ordinary thing to happen.
 *   - A parent CYCLE terminates and still renders every row. The daemon refuses
 *     one at the command, but the store is fed by a wire we do not control and a
 *     renderer that hangs on bad data is a renderer that hangs.
 *   - Every lookup is an id→node map. Nothing here walks the tree per row, so
 *     the build is linear in the number of rows rather than quadratic in it.
 */

export type ChatRowKind = 'chat' | 'chatFolder'

/** A chat, as the tree needs to read it. `AgentChat` satisfies this. */
export interface ChatLike {
  id: string
  title: string
  activeProviderId: string
  createdAt: string
  parentId?: string
  order?: number
}

/** A folder, as the tree needs to read it. `AgentChatFolder` satisfies this. */
export interface FolderLike {
  id: string
  name: string
  parentId?: string
  order?: number
}

export interface ChatRow {
  kind: ChatRowKind
  id: string
  /** The container this row sits in: a chat id, a folder id, or '' at the root. */
  parentId: string
  depth: number
  /** Chat title (never blank — see UNTITLED_CHAT_LABEL) or folder name. */
  title: string
  /** The chat's provider, for its glyph. '' on a folder. */
  providerId: string
  expanded: boolean
  hasChildren: boolean
  /** On screen only as an ancestor of a search hit — drawn dimmed. */
  ctx: boolean
  /**
   * Hoisted out of a folded parent because the chat is on screen. It renders one
   * step under that parent whatever depth it really lives at, and carries no
   * treatment of its own: a kept row is the same row, it is simply still there.
   *
   * Only its DEPTH is the holder's. `parentId` and `path` are the row's own, so
   * a drag reads its real container and a drop plans against it.
   */
  kept: boolean
  /** Folded, and still holding a row it is keeping visible. */
  holding: boolean
  /**
   * This row's REAL ancestor chain, `/a/b/`, ids delimited on both sides — the
   * one it lives in, never the one it happens to be drawn under.
   *
   * Published on the row element and read straight back by the drop policy, so
   * refusing "into my own descendant" is a substring test on an attribute rather
   * than a tree walk per pointermove. Delimited so `/ab/` can never match `/a/`.
   */
  path: string
}

export interface ChatTreeInput {
  chats: readonly ChatLike[]
  folders: readonly FolderLike[]
  /** Folded rows, by id. */
  collapsed: ReadonlySet<string>
  /** Chats ON SCREEN. A folded row holding one keeps it visible. */
  shown: ReadonlySet<string>
  /** Folded rows whose kept rows the user has explicitly let go of. */
  foldedAway: ReadonlySet<string>
  query: string
}

export interface ChatTree {
  rows: ChatRow[]
  /**
   * parentId → its children's ids, in the order they sit in. What a drop indexes
   * into, so a dropped row lands among its REAL siblings rather than among the
   * ones a filter happens to be showing.
   */
  siblings: Map<string, string[]>
}

/** A chat and a folder, reduced to the handful of facts the tree sorts and walks by. */
interface Node {
  id: string
  kind: ChatRowKind
  title: string
  providerId: string
  /**
   * The tie-break when two siblings claim the same `order` — every chat on a
   * daemon that has not placed them yet, which is the whole list on first run.
   * Chats break it by recency (newest first, the arrangement the flat list had);
   * folders sort above them, because a folder you have just made must not appear
   * below a screenful of chats.
   */
  rank: string
  parentId: string
  order: number
}

/** Chats sort newest-first among equals; folders sort above every chat. */
const FOLDER_RANK = '￿'

function compareSiblings(a: Node, b: Node): number {
  if (a.order !== b.order) return a.order - b.order
  if (a.rank !== b.rank) return a.rank < b.rank ? 1 : -1
  // Ids last, so a re-render of identical data can never reshuffle the level.
  return a.id < b.id ? -1 : 1
}

/** Everything the tree knows about, chats and folders alike, keyed by id. */
function collectNodes(input: ChatTreeInput): Map<string, Node> {
  const nodes = new Map<string, Node>()
  for (const c of input.chats) {
    nodes.set(c.id, {
      id: c.id,
      kind: 'chat',
      title: c.title,
      providerId: c.activeProviderId,
      rank: c.createdAt,
      parentId: c.parentId ?? '',
      order: c.order ?? 0,
    })
  }
  for (const f of input.folders) {
    nodes.set(f.id, {
      id: f.id,
      kind: 'chatFolder',
      title: f.name,
      providerId: '',
      rank: FOLDER_RANK,
      parentId: f.parentId ?? '',
      order: f.order ?? 0,
    })
  }
  return nodes
}

/** parentId → its children, sorted. A parent that is not a row means ROOT. */
function childrenByParent(nodes: Map<string, Node>): Map<string, Node[]> {
  const kids = new Map<string, Node[]>()
  for (const node of nodes.values()) {
    const parentId = node.parentId && nodes.has(node.parentId) ? node.parentId : ''
    const list = kids.get(parentId)
    if (list) list.push(node)
    else kids.set(parentId, [node])
  }
  for (const list of kids.values()) list.sort(compareSiblings)
  return kids
}

/**
 * The nested shape the filter walks, and the position of every row in it.
 *
 * No visited set, and none is needed. Every row has exactly ONE parent, so
 * `childrenByParent` puts it in exactly one child list and this walk cannot
 * reach it twice — which also means a CYCLE is never reached from the root at
 * all. Its rows are picked up afterwards, by the pass that emits everything
 * `position` never learned about.
 */
function buildForest(
  kids: Map<string, Node[]>,
  parentId: string,
  position: Map<string, number>,
): SearchNode[] {
  const out: SearchNode[] = []
  for (const node of kids.get(parentId) ?? []) {
    position.set(node.id, position.size)
    out.push({
      id: node.id,
      title: node.title,
      children: buildForest(kids, node.id, position),
    })
  }
  return out
}

/**
 * Which folded row is holding each on-screen chat, and in what order.
 *
 * Walked UP from the handful of chats that are on screen rather than down from
 * every folded row: the answer is the same and the cost is bounded by how many
 * panes the user has open instead of by how many chats the workspace holds.
 *
 * The OUTERMOST folded ancestor wins, because everything below it is folded away
 * with it — a kept row hoisted under an ancestor nobody can see is a kept row
 * nobody can see.
 */
function keptByHolder(
  nodes: Map<string, Node>,
  input: ChatTreeInput,
  position: Map<string, number>,
): Map<string, string[]> {
  const held = new Map<string, string[]>()
  for (const chatId of input.shown) {
    if (!nodes.has(chatId)) continue
    let holder = ''
    let cursor = nodes.get(chatId)!.parentId
    const walked = new Set<string>()
    while (cursor && nodes.has(cursor) && !walked.has(cursor)) {
      walked.add(cursor)
      if (input.collapsed.has(cursor)) holder = cursor
      cursor = nodes.get(cursor)!.parentId
    }
    if (!holder || input.foldedAway.has(holder)) continue
    const list = held.get(holder)
    if (list) list.push(chatId)
    else held.set(holder, [chatId])
  }
  // Tree order, not Set-insertion order: two chats kept by one row must read top
  // to bottom the way they would if the row were open.
  for (const list of held.values()) {
    list.sort((a, b) => (position.get(a) ?? 0) - (position.get(b) ?? 0))
  }
  return held
}

/**
 * A row's REAL ancestor chain, `/a/b/`, walked UP from the row itself.
 *
 * The walk down the tree accumulates this for free, and that is where every
 * ordinary row gets it. A KEPT row is the one that cannot: it is drawn under
 * whichever ancestor is holding it, never reached by the walk that would have
 * built its chain, and publishing the HOLDER's chain instead is not a smaller
 * mistake than publishing nothing. The chain is what the drop policy tests for
 * "into my own descendant" — a substring test on exactly this attribute — so a
 * kept row carrying its holder's chain would accept a drop that detaches a
 * subtree from the tree.
 *
 * Cost is bounded by how many chats are on screen times how deep they sit, not
 * by how many the workspace holds: only kept rows ever ask.
 */
function pathOf(nodes: Map<string, Node>, node: Node): string {
  const chain: string[] = []
  let cursor = node.parentId
  // The same cycle guard the rest of this file keeps: the store is fed by a wire
  // we do not control, and a walk that hangs is a sidebar that hangs.
  const walked = new Set<string>([node.id])
  while (cursor && nodes.has(cursor) && !walked.has(cursor)) {
    walked.add(cursor)
    chain.push(cursor)
    cursor = nodes.get(cursor)!.parentId
  }
  // Walked up, so written back down. reduceRight rather than a reverse and a
  // join: it is total — an empty chain is the root's own '/' — where a join
  // would need a length check that no kept row can ever fail (a kept row has a
  // folded ancestor by construction), and an untestable guard is worse than
  // arithmetic that does not need one.
  return chain.reduceRight((out, id) => `${out}${id}/`, '/')
}

export function buildChatTree(input: ChatTreeInput): ChatTree {
  const nodes = collectNodes(input)
  const kids = childrenByParent(nodes)
  const position = new Map<string, number>()
  const forest = buildForest(kids, '', position)

  const filtering = input.query.trim().length > 0
  const sets: SearchSets | null = filtering ? searchChats(forest, input.query) : null
  const held = filtering ? new Map<string, string[]>() : keptByHolder(nodes, input, position)

  const rows: ChatRow[] = []

  const emit = (node: Node, depth: number, path: string, kept: boolean): void => {
    const children = kids.get(node.id)
    rows.push({
      kind: node.kind,
      id: node.id,
      // A kept row is drawn under whichever ancestor is holding it, but it still
      // BELONGS where it belongs: publishing the row it is drawn under would let
      // a drop plan a move relative to a container the row is not in.
      parentId: nodes.has(node.parentId) ? node.parentId : '',
      depth,
      title: node.title,
      providerId: node.providerId,
      // A search ignores collapse (a folded parent must never hide the only
      // result), so every row it draws is drawn open.
      expanded: filtering || !input.collapsed.has(node.id),
      // A kept row is hoisted ALONE — its own subtree stays inside the folded
      // ancestor with everything else — so it must not advertise children it is
      // not drawing. This is not a treatment: `hasChildren` is what puts a
      // chevron on the row AND what makes the gap under it mean "before its
      // first child" to a drop. Both would be about rows that are not on screen.
      hasChildren: !kept && children !== undefined && children.length > 0,
      ctx: sets !== null && sets.ctx.has(node.id),
      kept,
      holding: !kept && (held.get(node.id)?.length ?? 0) > 0,
      path,
    })
  }

  const walk = (parentId: string, depth: number, path: string): void => {
    for (const node of kids.get(parentId) ?? []) {
      // `keep` is closed under ancestors-of-matches, so a row that is not in it
      // has no descendant that is: skipping the subtree with it drops nothing.
      if (sets && !sets.keep.has(node.id)) continue
      emit(node, depth, path, false)

      if (!sets && input.collapsed.has(node.id)) {
        // Folded, but still holding what you are reading: those rows stay, one
        // step in. They are reached ONLY here — the walk does not descend into
        // a folded row — so no row can be drawn twice.
        for (const heldId of held.get(node.id) ?? []) {
          // Its OWN chain, not the holder's: a kept row is the same row, and
          // everything a drag reads off it has to say where it really lives.
          const heldNode = nodes.get(heldId)!
          emit(heldNode, depth + 1, pathOf(nodes, heldNode), true)
        }
        continue
      }
      walk(node.id, depth + 1, `${path}${node.id}/`)
    }
  }
  walk('', 0, '/')

  // Anything a CYCLE kept out of the tree is still a real row. Rendering it at
  // the root is wrong about its depth and right about its existence, which is
  // the trade this panel makes every time: a row the user can see and move is
  // recoverable, a row that silently does not exist is not.
  //
  // Unreachable means absent from `position` — the forest walk that built it
  // ignores collapse and filtering, so it holds every row the tree can actually
  // reach. Asking "did we already emit this?" instead is the bug this replaced:
  // a row hidden inside a FOLDED parent has not been emitted either, and every
  // one of them reappeared here as a second copy of itself at the root.
  for (const node of nodes.values()) {
    if (position.has(node.id)) continue
    if (sets && !sets.keep.has(node.id)) continue
    emit(node, 0, '/', false)
  }

  const siblings = new Map<string, string[]>()
  for (const [parentId, list] of kids)
    siblings.set(
      parentId,
      list.map((n) => n.id),
    )

  return { rows, siblings }
}

/**
 * Every id under `nodeId`, folders and chats alike, deepest first, excluding
 * itself.
 *
 * Deepest first because the caller is deleting them: a parent that goes before
 * its children leaves the children pointing at nothing for as long as the
 * requests are in flight, and the panel would flash them at the root on the way
 * past. Takes the sibling index the build already produced, so this is a walk of
 * the subtree and not of the whole tree.
 */
export function chatSubtreeIds(
  siblings: ReadonlyMap<string, readonly string[]>,
  nodeId: string,
): string[] {
  const out: string[] = []
  const seen = new Set<string>([nodeId])
  const walk = (id: string): void => {
    for (const child of siblings.get(id) ?? []) {
      if (seen.has(child)) continue
      seen.add(child)
      walk(child)
      out.push(child)
    }
  }
  walk(nodeId)
  return out
}
