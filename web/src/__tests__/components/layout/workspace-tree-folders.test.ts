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
  type SidebarChat,
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

const chat = (id: string, over: Partial<SidebarChat> = {}): SidebarChat => ({
  id,
  title: id,
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

  test('a workspace created ON a folder row lands inside it, however deep the folder', () => {
    // The daemon files a create started on a folder row as a fork ROOT: folderId
    // set and parentId empty, the two being mutually exclusive by design (the
    // create endpoint 400s the pairing). Requiring the folder's anchor to equal
    // the fork parent then admits such a row into a ROOT-level folder only, so
    // creating inside any folder that hangs under a workspace dropped the edge
    // and re-rooted the row to the repo root — "created inside, appears outside".
    const tree = buildSidebarTree(
      [ws('main'), ws('created', { folderId: 'backlog' })],
      [folder('backlog', { parentId: 'main' })],
    )
    expect(ids(childOf(tree, 'backlog'))).toEqual(['created'])
    expect(ids(tree)).not.toContain('created')
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

/**
 * Chats are the tree's fourth row kind (design spec §3.1) and are placed by ONE
 * edge (§3.2), not by the workspace rule: no branch means no fork lineage for a
 * folder edge to split, so there are no anchors to check.
 */
describe('buildSidebarTree — chats', () => {
  test('is byte-identical when no chats are passed — the two other callers', () => {
    const workspaces = [ws('root'), ws('kid', { parentId: 'root' })]
    const folders = [folder('f1')]
    expect(buildSidebarTree(workspaces, folders, [])).toEqual(buildSidebarTree(workspaces, folders))
  })

  test('a chat filed nowhere and owning nothing sits at the repo root', () => {
    expect(ids(buildSidebarTree([], [], [chat('c1')]))).toEqual(['c1'])
  })

  test('a chat nests under the workspace it owns when it is filed nowhere', () => {
    const tree = buildSidebarTree([ws('w1')], [], [chat('c1', { workspaceId: 'w1' })])
    expect(ids(childOf(tree, 'w1'))).toEqual(['c1'])
  })

  test('parentId beats workspaceId — a filed chat sits where it is filed', () => {
    const tree = buildSidebarTree(
      [ws('w1')],
      [folder('f1')],
      [chat('c1', { workspaceId: 'w1', parentId: 'f1' })],
    )
    expect(ids(childOf(tree, 'f1'))).toEqual(['c1'])
    expect(childOf(tree, 'w1')).toHaveLength(0)
  })

  test('a chat threads under another chat, arbitrarily deep', () => {
    const tree = buildSidebarTree(
      [],
      [],
      [
        chat('c1'),
        chat('c2', { parentId: 'c1' }),
        chat('c3', { parentId: 'c2' }),
        chat('c4', { parentId: 'c3' }),
      ],
    )
    expect(ids(tree)).toEqual(['c1'])
    expect(ids(childOf(tree, 'c3'))).toEqual(['c4'])
  })

  test('a folder inside a chat still anchors a fork child to the right workspace', () => {
    // The anchor walk has to step THROUGH the chat: stopping there would report
    // no anchor and drop the folder edge, re-rooting the row it holds.
    const tree = buildSidebarTree(
      [ws('w1'), ws('kid', { parentId: 'w1', folderId: 'f1' })],
      [folder('f1', { parentId: 'c1' })],
      [chat('c1', { workspaceId: 'w1' })],
    )
    expect(ids(childOf(tree, 'f1'))).toEqual(['kid'])
  })

  test('chats share the one sibling order with folders and workspaces', () => {
    const tree = buildSidebarTree(
      [ws('w', { order: 1 })],
      [folder('f', { order: 2 })],
      [chat('c', { order: 0 })],
    )
    expect(ids(tree)).toEqual(['c', 'w', 'f'])
  })

  test('on an order TIE a chat sorts below folders and branches', () => {
    // Every row still holding 0 is the whole tree on a daemon that has placed
    // nothing — and it must read the same way the Chats panel does.
    const tree = buildSidebarTree([ws('w')], [folder('f')], [chat('c')])
    expect(ids(tree)).toEqual(['f', 'w', 'c'])
  })

  test('a chat parented to nothing that exists re-roots rather than vanishing', () => {
    expect(ids(buildSidebarTree([], [], [chat('c1', { parentId: 'gone' })]))).toEqual(['c1'])
  })

  test('a chat cycle re-roots rather than looping forever', () => {
    const tree = buildSidebarTree(
      [],
      [],
      [chat('c1', { parentId: 'c2' }), chat('c2', { parentId: 'c1' })],
    )
    expect(tree).toHaveLength(2)
  })

  test('a chat parented to itself re-roots', () => {
    expect(ids(buildSidebarTree([], [], [chat('self', { parentId: 'self' })]))).toEqual(['self'])
  })

  // A cycle that runs THROUGH BOTH structures — chat -> folder -> chat — is the
  // case that justifies one placement authority over a second pass: a merge
  // that placed chats after the fact would hold half this loop in each pass and
  // neither would see it whole. Every row still renders; none is lost to it.
  test('a chat -> folder -> chat cycle re-roots rather than looping forever', () => {
    const tree = buildSidebarTree(
      [],
      [folder('f1', { parentId: 'c2' })],
      [chat('c1', { parentId: 'f1' }), chat('c2', { parentId: 'c1' })],
    )
    const rendered = new Set<string>()
    const collect = (nodes: SidebarTreeNode[]) => {
      for (const node of nodes) {
        rendered.add(node.id)
        collect(node.children)
      }
    }
    collect(tree)
    expect(rendered).toEqual(new Set(['f1', 'c1', 'c2']))
  })

  // The same loop with a WORKSPACE in it, since a workspace resolves its parent
  // by a different rule than a chat does and the guard has to hold across both.
  test('a chat -> folder -> workspace cycle still renders every row', () => {
    const tree = buildSidebarTree(
      [ws('w1', { folderId: 'f1' })],
      [folder('f1', { parentId: 'c1' })],
      [chat('c1', { workspaceId: 'w1' })],
    )
    const rendered = new Set<string>()
    const collect = (nodes: SidebarTreeNode[]) => {
      for (const node of nodes) {
        rendered.add(node.id)
        collect(node.children)
      }
    }
    collect(tree)
    expect(rendered).toEqual(new Set(['w1', 'f1', 'c1']))
  })

  test('passes every chat field through untouched', () => {
    const full = chat('c1', { workspaceId: 'w1', parentId: 'f1', order: 3 })
    const tree = buildSidebarTree([], [folder('f1')], [full])
    const node = findNode(tree, 'c1')
    expect(node?.kind === 'chat' && node.chat).toEqual(full)
  })
})
