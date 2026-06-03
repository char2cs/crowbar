import { describe, expect, test } from 'vitest'
import { buildWorkspaceTree } from '@/components/layout/workspace-tree'
import type { Workspace } from '@/lib/store/sidebar'

const ws = (id: string, parentId?: string): Workspace => ({
  id, branch: `branch/${id}`, parentId, age: 'now',
})

describe('buildWorkspaceTree', () => {
  test('flat list with no parentIds → all nodes at root', () => {
    const result = buildWorkspaceTree([ws('a'), ws('b'), ws('c')])
    expect(result).toHaveLength(3)
    expect(result.every(n => n.children.length === 0)).toBe(true)
  })

  test('single level of nesting → correct parent/child attachment', () => {
    const result = buildWorkspaceTree([ws('parent'), ws('child', 'parent')])
    expect(result).toHaveLength(1)
    expect(result[0].workspace.id).toBe('parent')
    expect(result[0].children).toHaveLength(1)
    expect(result[0].children[0].workspace.id).toBe('child')
  })

  test('multi-level nesting', () => {
    const result = buildWorkspaceTree([
      ws('root'), ws('mid', 'root'), ws('leaf', 'mid'),
    ])
    expect(result).toHaveLength(1)
    expect(result[0].children[0].children[0].workspace.id).toBe('leaf')
  })

  test('orphaned parentId → treated as root', () => {
    const result = buildWorkspaceTree([ws('child', 'nonexistent')])
    expect(result).toHaveLength(1)
    expect(result[0].workspace.id).toBe('child')
    expect(result[0].children).toHaveLength(0)
  })

  test('circular reference → both treated as root', () => {
    const a: Workspace = { id: 'a', branch: 'a', parentId: 'b', age: 'now' }
    const b: Workspace = { id: 'b', branch: 'b', parentId: 'a', age: 'now' }
    const result = buildWorkspaceTree([a, b])
    expect(result).toHaveLength(2)
    expect(result[0].children).toHaveLength(0)
    expect(result[1].children).toHaveLength(0)
  })

  test('preserves workspace data on nodes', () => {
    const full: Workspace = { id: 'x', branch: 'feat/x', status: 'pr-open', added: 100, deleted: 5, age: '1h ago' }
    const result = buildWorkspaceTree([full])
    expect(result[0].workspace).toEqual(full)
  })
})
