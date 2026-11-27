import type { EntityRef, Operation, OperationItem } from '@/core/entities/operations'
import type { CreateDraft, RemovalEntry } from './tree-ui-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { attachRemovalState } from '@/components/layout/removal-plan'
import type { TreeIndex, TreeNode } from './tree-index'
import type { RowViews } from './views'

export interface TreeInput {
  projectId: string
  homeWorkspaceId: string | null
  index: TreeIndex
  views: RowViews
  operations: readonly Operation[]
  isPresent: (ref: EntityRef) => boolean
  ui: { removals: readonly RemovalEntry[]; draft: CreateDraft | null }
}

interface Context {
  inRepo: boolean
  workspaceId: string | null
}

const reported = new Set<string>()

function reportMissing(node: TreeNode): void {
  if (reported.has(node.id)) return
  reported.add(node.id)
  console.warn(`sidebar: no ${node.kind} view for tree node ${node.id}; not drawn`)
}

type Walk = (parentNodeId: string, parentRowId: string | null, ctx: Context) => void

function folderRow(node: TreeNode, parentId: string | null, ctx: Context): SidebarRow {
  return {
    id: node.id,
    kind: 'folder',
    parentId,
    order: node.order,
    createdAt: node.createdAt,
    label: node.name ?? '',
    ownsWorktree: ctx.inRepo,
    workspaceId: ctx.workspaceId,
    working: false,
    hasView: false,
  }
}

function drawNode(
  input: TreeInput,
  rows: SidebarRow[],
  walk: Walk,
  node: TreeNode,
  parentId: string | null,
  ctx: Context,
): void {
  const { views } = input
  if (node.kind === 'folder') {
    rows.push(folderRow(node, parentId, ctx))
    walk(node.id, node.id, ctx)
    return
  }
  if (node.kind === 'repo') {
    const repo = views.repo(node.id)
    if (!repo) return reportMissing(node)
    const inner = { inRepo: true, workspaceId: repo.workspaceId }
    if (repo.rowId === null) return walk(node.id, parentId, inner)
    const { rowId, workspaceId, ...fields } = repo
    rows.push({
      ...fields,
      id: rowId,
      kind: 'branch',
      parentId,
      order: node.order,
      ownsWorktree: true,
      workspaceId,
      hasView: false,
    })
    walk(node.id, rowId, inner)
    return
  }
  if (node.kind === 'workspace') {
    const ws = views.workspace(node.id)
    if (!ws) return reportMissing(node)
    const { rowId, icon, ...fields } = ws
    rows.push({
      ...fields,
      ...icon,
      id: rowId,
      kind: 'branch',
      parentId,
      order: node.order,
      ownsWorktree: true,
      workspaceId: node.id,
      hasView: false,
    })
    walk(node.id, rowId, { ...ctx, workspaceId: node.id })
    return
  }
  const chat = views.chat(node.id)
  if (!chat) return reportMissing(node)
  const base = {
    id: node.id,
    parentId,
    order: node.order,
    createdAt: chat.createdAt,
    label: chat.label,
    labelProvisional: chat.labelProvisional,
    working: false,
    hasView: false,
  }
  if (chat.ownedWorkspaceId !== undefined) {
    rows.push({ ...base, kind: 'branch', ownsWorktree: true, workspaceId: chat.ownedWorkspaceId })
    walk(node.id, node.id, { ...ctx, workspaceId: chat.ownedWorkspaceId })
    return
  }
  rows.push({
    ...base,
    kind: 'chat',
    ownsWorktree: false,
    workspaceId: chat.workspaceId ?? ctx.workspaceId,
    canBranchOut: ctx.inRepo,
  })
  walk(node.id, node.id, ctx)
}

function treeRows(input: TreeInput): SidebarRow[] {
  const rows: SidebarRow[] = []
  const walk: Walk = (parentNodeId, parentRowId, ctx) => {
    for (const node of input.index.children(parentNodeId))
      drawNode(input, rows, walk, node, parentRowId, ctx)
  }
  walk('', null, { inRepo: false, workspaceId: input.homeWorkspaceId })
  return rows
}

const CREATE_KINDS: Readonly<Record<string, SidebarRow['kind']>> = {
  'branch.import': 'branch',
  'branch.create': 'branch',
  'chat.create': 'chat',
  'repo.create': 'branch',
}

const LAST = Number.MAX_SAFE_INTEGER

function failed(op: Operation, item: OperationItem): boolean {
  if (item.state === 'failed' || item.error) return true
  return op.state === 'failed' && item.state !== 'succeeded'
}

function drawnItems(op: Operation): OperationItem[] {
  if (op.kind === 'repo.create') return op.items.filter((item) => item.ref.kind === 'workspace')
  const chats = op.items.filter((item) => item.ref.kind === 'chat')
  return chats.length > 0 ? chats : op.items
}

function drawnUnder(op: Operation): EntityRef {
  if (op.kind !== 'repo.create') return op.subject
  return op.items.find((item) => item.ref.kind === 'repo')?.ref ?? op.subject
}

export function createItemRefs(ops: readonly Operation[]): EntityRef[] {
  const refs: EntityRef[] = []
  for (const op of ops) {
    if (CREATE_KINDS[op.kind]) for (const item of drawnItems(op)) refs.push(item.ref)
  }
  return refs
}

function deletedSince(op: Operation, item: OperationItem): boolean {
  return !op.local && (item.state === 'succeeded' || op.state === 'succeeded')
}

function operationRows(input: TreeInput): SidebarRow[] {
  const rows: SidebarRow[] = []
  for (const op of input.operations) {
    const kind = CREATE_KINDS[op.kind]
    if (!kind || op.dismissed || op.projectId !== input.projectId) continue
    const subject = drawnUnder(op)
    for (const item of drawnItems(op)) {
      if (input.isPresent(item.ref) || deletedSince(op, item)) continue
      const isFailed = failed(op, item)
      const parentId = item.parentId === undefined ? subject.id : item.parentId || null
      rows.push({
        id: item.ref.id,
        kind,
        parentId: (parentId !== null && input.views.repo(parentId)?.rowId) || parentId,
        order: LAST,
        createdAt: op.createdAt,
        label: item.label,
        ownsWorktree: kind === 'branch',
        workspaceId: subject.kind === 'workspace' ? subject.id : null,
        working: !isFailed,
        hasView: false,
        operation: {
          opId: op.id,
          state: isFailed ? 'failed' : item.state,
          problem: isFailed ? (item.error ?? op.error ?? null) : null,
        },
      })
    }
  }
  return rows
}

function draftRow(draft: CreateDraft): SidebarRow {
  return {
    id: 'create-draft',
    kind: draft.kind,
    parentId: draft.parentId || null,
    order: draft.order,
    label: '',
    ownsWorktree: draft.ownsWorktree,
    workspaceId: draft.workspaceId,
    working: false,
    hasView: false,
    draft: true,
  }
}

export function deriveRows(input: TreeInput): SidebarRow[] {
  const { draft, removals } = input.ui
  return [
    ...attachRemovalState(treeRows(input), removals),
    ...operationRows(input),
    ...(draft?.projectId === input.projectId ? [draftRow(draft)] : []),
  ]
}
