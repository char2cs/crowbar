// web/src/__tests__/lib/mock/files.test.ts
import { describe, it, expect } from 'vitest'
import { getMockFileTree } from '@/lib/mock/files'
import type { FileNode } from '@/lib/mock/files'

describe('getMockFileTree', () => {
  it('returns a non-empty array for any rootPath', () => {
    const tree = getMockFileTree('/any/path')
    expect(tree.length).toBeGreaterThan(0)
  })

  it('every node has required fields', () => {
    const tree = getMockFileTree('/workspace')
    function checkNode(node: FileNode) {
      expect(typeof node.name).toBe('string')
      expect(typeof node.path).toBe('string')
      expect(node.isDir !== undefined || node.isDirectory !== undefined).toBe(true)
      if (node.children) {
        node.children.forEach(checkNode)
      }
    }
    tree.forEach(checkNode)
  })

  it('returns at least one directory node', () => {
    const tree = getMockFileTree('/workspace')
    const hasDir = tree.some((n) => n.isDir || n.isDirectory)
    expect(hasDir).toBe(true)
  })

  it('returns at least one file node', () => {
    const tree = getMockFileTree('/workspace')
    function hasFile(nodes: FileNode[]): boolean {
      return nodes.some(
        (n) => !(n.isDir || n.isDirectory) || (n.children ? hasFile(n.children) : false),
      )
    }
    expect(hasFile(tree)).toBe(true)
  })

  it('git status field is optional but at least some nodes have it', () => {
    const tree = getMockFileTree('/workspace')
    function collectAll(nodes: FileNode[]): FileNode[] {
      return nodes.flatMap((n) => [n, ...(n.children ? collectAll(n.children) : [])])
    }
    const allNodes = collectAll(tree)
    const withStatus = allNodes.some((n) => n.gitStatus !== undefined)
    expect(withStatus).toBe(true)
  })
})
