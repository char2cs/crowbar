/** One sidebar placement: a node of a project's tree, named by its entity id. */
export interface TreeNode {
  id: string
  kind: 'folder' | 'repo' | 'workspace' | 'chat'
  /** '' is the project root. */
  parentId: string
  order: number
  /** Folders only. */
  name?: string
  /** The `order` tiebreak a level nobody has dragged still sorts by. */
  createdAt?: string
}

/** The questions row derivation asks of a tree, and nothing else. */
export interface TreeIndex {
  /** `parentId`'s children in drawing order. */
  children(parentId: string): readonly TreeNode[]
  node(id: string): TreeNode | undefined
}

const NONE: readonly TreeNode[] = []

/** Indexes `nodes`, keeping their order within each level. */
export function indexNodes(nodes: readonly TreeNode[]): TreeIndex {
  const byId = new Map<string, TreeNode>()
  const byParent = new Map<string, TreeNode[]>()
  for (const node of nodes) {
    byId.set(node.id, node)
    const level = byParent.get(node.parentId)
    if (level) level.push(node)
    else byParent.set(node.parentId, [node])
  }
  return {
    children: (parentId) => byParent.get(parentId) ?? NONE,
    node: (id) => byId.get(id),
  }
}

/** `id`'s descendants at any depth, depth-first in drawing order. */
export function descendantIds(index: TreeIndex, id: string): string[] {
  const out: string[] = []
  const walk = (parentId: string) => {
    for (const child of index.children(parentId)) {
      out.push(child.id)
      walk(child.id)
    }
  }
  walk(id)
  return out
}
