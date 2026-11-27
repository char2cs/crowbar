import { nanoid } from 'nanoid'
import { DEFAULT_SPLIT_RATIO, MIN_PANE_SIZE } from '../constants/pane'
import type { LayoutLeaf, LayoutNode, LayoutSplit } from '../types/pane'

export function createLeaf(id: string): LayoutLeaf {
  return { type: 'pane', id }
}

function createSplit(
  direction: 'horizontal' | 'vertical',
  first: LayoutNode,
  second: LayoutNode,
  sizes: [number, number] = DEFAULT_SPLIT_RATIO,
): LayoutSplit {
  return { type: 'split', id: nanoid(), direction, sizes: normalizeSizes(sizes), first, second }
}

function normalizeSizes(sizes: [number, number]): [number, number] {
  const a = Number.isFinite(sizes[0]) ? sizes[0] : DEFAULT_SPLIT_RATIO[0]
  const b = Number.isFinite(sizes[1]) ? sizes[1] : DEFAULT_SPLIT_RATIO[1]
  const total = a + b
  if (total <= 0) return DEFAULT_SPLIT_RATIO
  const norm: [number, number] = [(a / total) * 100, (b / total) * 100]
  const min = Math.min(MIN_PANE_SIZE, 49)
  if (norm[0] < min) return [min, 100 - min]
  if (norm[1] < min) return [100 - min, min]
  return norm
}

export function findLeaf(root: LayoutNode, paneId: string): LayoutLeaf | null {
  if (root.type === 'pane') return root.id === paneId ? root : null
  return findLeaf(root.first, paneId) ?? findLeaf(root.second, paneId)
}

export function findSplit(root: LayoutNode, splitId: string): LayoutSplit | null {
  if (root.type === 'pane') return null
  if (root.id === splitId) return root
  return findSplit(root.first, splitId) ?? findSplit(root.second, splitId)
}

export function getAllLeafIds(root: LayoutNode): string[] {
  if (root.type === 'pane') return [root.id]
  return [...getAllLeafIds(root.first), ...getAllLeafIds(root.second)]
}

export function getFirstLeafId(root: LayoutNode): string {
  if (root.type === 'pane') return root.id
  return getFirstLeafId(root.first)
}

export function splitLayout(
  root: LayoutNode,
  paneId: string,
  direction: 'horizontal' | 'vertical',
  placement: 'before' | 'after' = 'after',
  leafId?: string,
): { layout: LayoutNode; newPaneId: string } | null {
  const newPaneId = leafId ?? nanoid()
  const result = insertLeaf(root, paneId, createLeaf(newPaneId), direction, placement)
  if (!result) return null
  return { layout: result, newPaneId }
}

function insertLeaf(
  node: LayoutNode,
  targetId: string,
  newLeaf: LayoutLeaf,
  direction: 'horizontal' | 'vertical',
  placement: 'before' | 'after',
): LayoutNode | null {
  if (node.type === 'pane') {
    if (node.id !== targetId) return null
    const first = placement === 'before' ? newLeaf : node
    const second = placement === 'before' ? node : newLeaf
    return createSplit(direction, first, second)
  }
  const newFirst = insertLeaf(node.first, targetId, newLeaf, direction, placement)
  if (newFirst) return { ...node, first: newFirst }
  const newSecond = insertLeaf(node.second, targetId, newLeaf, direction, placement)
  if (newSecond) return { ...node, second: newSecond }
  return null
}

export function closeLayout(root: LayoutNode, paneId: string): LayoutNode | null {
  if (root.type === 'pane') return root.id === paneId ? null : root
  const newFirst = closeLayout(root.first, paneId)
  const newSecond = closeLayout(root.second, paneId)
  if (newFirst === null && newSecond === null) return null
  if (newFirst === null) return newSecond
  if (newSecond === null) return newFirst
  return { ...root, first: newFirst, second: newSecond }
}

export function updateSplitSizes(
  root: LayoutNode,
  splitId: string,
  sizes: [number, number],
): LayoutNode {
  if (root.type === 'pane') return root
  if (root.id === splitId) return { ...root, sizes: normalizeSizes(sizes) }
  return {
    ...root,
    first: updateSplitSizes(root.first, splitId, sizes),
    second: updateSplitSizes(root.second, splitId, sizes),
  }
}

export function distributeSplit(root: LayoutNode, splitId: string): LayoutNode {
  return updateSplitSizes(root, splitId, DEFAULT_SPLIT_RATIO)
}

export function normalizeLayout(root: LayoutNode): LayoutNode {
  if (root.type === 'pane') return root
  const first = normalizeLayout(root.first)
  const second = normalizeLayout(root.second)
  return { ...root, sizes: normalizeSizes(root.sizes), first, second }
}

export function getAdjacentLeafId(
  root: LayoutNode,
  paneId: string,
  direction: 'left' | 'right' | 'up' | 'down',
): string | null {
  const splitDir = direction === 'left' || direction === 'right' ? 'horizontal' : 'vertical'
  const forward = direction === 'right' || direction === 'down'
  return findAdjacent(root, paneId, splitDir, forward)
}

function findAdjacent(
  root: LayoutNode,
  paneId: string,
  splitDir: 'horizontal' | 'vertical',
  forward: boolean,
): string | null {
  if (root.type === 'pane') return null
  const firstIds = getAllLeafIds(root.first)
  const secondIds = getAllLeafIds(root.second)
  if (root.direction === splitDir) {
    if (firstIds.includes(paneId) && forward) return getFirstLeafId(root.second)
    if (secondIds.includes(paneId) && !forward) return getFirstLeafId(root.first)
  }
  return (
    findAdjacent(root.first, paneId, splitDir, forward) ??
    findAdjacent(root.second, paneId, splitDir, forward)
  )
}
