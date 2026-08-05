/**
 * Contract pins for the folder-aware, ordered sidebar tree.
 *
 * The rule under test keeps organisation and lineage independent: a folder may
 * sit between a workspace and one of its fork children when both belong to the
 * same visible sibling space. An incompatible folder edge falls back to the
 * real fork parent instead of visually splitting the chain.
 *
 * The other pin is ordering: an un-ordered repo must come out in exactly the
 * order the backend sent it, or the sidebar would shuffle itself the first time
 * anyone opened it.
 */
import { describe, expect, test } from 'vitest'
import {
  buildSidebarTree,
  type PlacedWorkspace,
  type SidebarFolder,
  type SidebarTreeNode,
} from '@/components/layout/workspace-tree-utils'

const ws = (id: string, over: Partial<PlacedWorkspace> = {}): PlacedWorkspace => ({
  id,
  branch: `branch/${id}`,
  age: 'now',
  ...over,
})

const folder = (id: string, over: Partial<SidebarFolder> = {}): SidebarFolder => ({
  id,
  name: id,
  ...over,
})

const ids = (nodes: SidebarTreeNode[]) => nodes.map((n) => n.id)
const findNode = (nodes: SidebarTreeNode[], id: string): SidebarTreeNode | undefined => {
  for (const node of nodes) {
    if (node.id === id) return node
    const nested = findNode(node.children, id)
    if (nested) return nested
  }
  return undefined
}
const childOf = (nodes: SidebarTreeNode[], id: string) => findNode(nodes, id)?.children ?? []

describe('buildSidebarTree — placement', () => {
  test('a workspace with no parent and no folder sits at the repo root', () => {
    expect(ids(buildSidebarTree([ws('a'), ws('b')]))).toEqual(['a', 'b'])
  })

  test('a folder sits at the repo root when it has no parent', () => {
    const tree = buildSidebarTree([], [folder('f1')])
    expect(ids(tree)).toEqual(['f1'])
    expect(tree[0].kind).toBe('folder')
  })

  test('a fork-root with folderId renders inside that folder', () => {
    const tree = buildSidebarTree([ws('root', { folderId: 'f1' })], [folder('f1')])
    expect(ids(tree)).toEqual(['f1'])
    expect(ids(childOf(tree, 'f1'))).toEqual(['root'])
  })

  test('a fork child can be organised inside a folder under its fork parent', () => {
    const tree = buildSidebarTree(
      [ws('parent'), ws('child', { parentId: 'parent', folderId: 'f1' })],
      [folder('f1', { parentId: 'parent' })],
    )
    expect(ids(childOf(tree, 'parent'))).toEqual(['f1'])
    expect(ids(childOf(tree, 'f1'))).toEqual(['child'])
  })

  test('an incompatible folder edge falls back to the real fork parent', () => {
    const tree = buildSidebarTree(
      [ws('parent'), ws('other'), ws('child', { parentId: 'parent', folderId: 'f1' })],
      [folder('f1', { parentId: 'other' })],
    )
    expect(ids(childOf(tree, 'f1'))).toEqual([])
    expect(ids(childOf(tree, 'parent'))).toEqual(['child'])
  })

  test('descendants follow their fork ancestor into a folder', () => {
    const tree = buildSidebarTree(
      [ws('root', { folderId: 'f1' }), ws('kid', { parentId: 'root' })],
      [folder('f1')],
    )
    const inFolder = childOf(tree, 'f1')
    expect(ids(inFolder)).toEqual(['root'])
    expect(ids(inFolder[0].children)).toEqual(['kid'])
  })

  test('folders nest inside folders', () => {
    const tree = buildSidebarTree([], [folder('outer'), folder('inner', { parentId: 'outer' })])
    expect(ids(tree)).toEqual(['outer'])
    expect(ids(childOf(tree, 'outer'))).toEqual(['inner'])
  })

  test('a folder can hang off a protected branch', () => {
    const tree = buildSidebarTree(
      [ws('develop', { status: 'locked' }), ws('feat', { parentId: 'develop', folderId: 'f1' })],
      [folder('f1', { parentId: 'develop' })],
    )
    expect(ids(tree)).toEqual(['develop'])
    expect(ids(childOf(tree, 'develop'))).toEqual(['f1'])
    expect(ids(childOf(tree, 'f1'))).toEqual(['feat'])
  })

  test('an unresolvable folderId falls back to the repo root, not a dropped row', () => {
    const tree = buildSidebarTree([ws('orphan', { folderId: 'gone' })])
    expect(ids(tree)).toEqual(['orphan'])
  })
})

describe('buildSidebarTree — ordering', () => {
  test('un-ordered rows keep the order they arrived in', () => {
    expect(ids(buildSidebarTree([ws('c'), ws('a'), ws('b')]))).toEqual(['c', 'a', 'b'])
  })

  test('order sorts siblings', () => {
    const tree = buildSidebarTree([
      ws('third', { order: 2 }),
      ws('first', { order: 0 }),
      ws('second', { order: 1 }),
    ])
    expect(ids(tree)).toEqual(['first', 'second', 'third'])
  })

  test('folders and workspaces share one sibling order', () => {
    const tree = buildSidebarTree([ws('w', { order: 1 })], [folder('f', { order: 0 })])
    expect(ids(tree)).toEqual(['f', 'w'])
  })

  test('rows without an order sort after rows with one', () => {
    const tree = buildSidebarTree([ws('none'), ws('ordered', { order: 0 })])
    expect(ids(tree)).toEqual(['ordered', 'none'])
  })

  test('ordering applies at every depth', () => {
    const tree = buildSidebarTree([
      ws('root'),
      ws('b', { parentId: 'root', order: 1 }),
      ws('a', { parentId: 'root', order: 0 }),
    ])
    expect(ids(childOf(tree, 'root'))).toEqual(['a', 'b'])
  })
})

describe('buildSidebarTree — corrupt input degrades, never hangs', () => {
  test('a two-node cycle re-roots both', () => {
    const tree = buildSidebarTree([ws('a', { parentId: 'b' }), ws('b', { parentId: 'a' })])
    expect(tree).toHaveLength(2)
    expect(tree.every((n) => n.children.length === 0)).toBe(true)
  })

  test('a folder cycle re-roots rather than looping forever', () => {
    const tree = buildSidebarTree(
      [],
      [folder('f1', { parentId: 'f2' }), folder('f2', { parentId: 'f1' })],
    )
    expect(tree).toHaveLength(2)
  })

  test('a workspace parented to itself re-roots', () => {
    expect(ids(buildSidebarTree([ws('self', { parentId: 'self' })]))).toEqual(['self'])
  })

  test('passes every workspace field through untouched', () => {
    const full = ws('x', { status: 'pr-open', added: 100, deleted: 5, order: 3 })
    const tree = buildSidebarTree([full])
    expect(tree[0].kind === 'workspace' && tree[0].workspace).toEqual(full)
  })
})
