export interface TreeNode {
  id: string
  kind: 'folder' | 'repo' | 'workspace' | 'chat'
  parentId: string
  order: number
  name?: string
  createdAt?: string
}

export interface TreeIndex {
  children(parentId: string): readonly TreeNode[]
  node(id: string): TreeNode | undefined
}

const NONE: readonly TreeNode[] = []

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
