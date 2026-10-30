import { isEntityPresent } from '@/lib/store/entity-presence'
import type { EntityRef, Operation, OperationItem } from '@/lib/store/operations'
import type { CreateDraft } from '@/lib/store/create-draft'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { Repo } from '@/lib/store/sidebar'
import { resolveHomeOwnerId } from '@/components/sidebar/lib/workspace-owners'

/** The operations whose items are rows the sidebar will draw once they arrive. */
const CREATE_KINDS: Readonly<Record<string, SidebarRow['kind']>> = {
  'branch.import': 'branch',
  'branch.create': 'branch',
  'chat.create': 'chat',
}

/** Drawn after every real sibling: the daemon appends a create at the end of its level. */
const LAST = Number.MAX_SAFE_INTEGER

function failed(op: Operation, item: OperationItem): boolean {
  if (item.state === 'failed' || item.error) return true
  // A refused request, or a restart that interrupted the work, fails every unfinished item.
  return op.state === 'failed' && item.state !== 'succeeded'
}

/** The item that stands for the row: a branch create names its chat and its workspace. */
function drawnItems(op: Operation): OperationItem[] {
  const chats = op.items.filter((item) => item.ref.kind === 'chat')
  return chats.length > 0 ? chats : op.items
}

/** The refs whose arrival retires a create's row. */
export function createItemRefs(ops: readonly Operation[]): EntityRef[] {
  const refs: EntityRef[] = []
  for (const op of ops) {
    if (CREATE_KINDS[op.kind]) for (const item of drawnItems(op)) refs.push(item.ref)
  }
  return refs
}

/**
 * One row per create item whose entity has not arrived: a spinner while it
 * runs (or succeeded and is still in flight to the stores), its reason once it
 * failed. A failed item whose entity exists draws nothing: that row explains itself.
 */
export function rowsFromOperations(
  ops: readonly Operation[],
  scope: { projectId: string; repoId?: string },
  isPresent: (ref: EntityRef) => boolean = isEntityPresent,
): SidebarRow[] {
  const rows: SidebarRow[] = []
  for (const op of ops) {
    const kind = CREATE_KINDS[op.kind]
    if (!kind || op.dismissed || op.projectId !== scope.projectId) continue
    const subject = op.subject
    if (scope.repoId !== undefined && subject.id !== scope.repoId) continue
    for (const item of drawnItems(op)) {
      if (isPresent(item.ref)) continue
      const isFailed = failed(op, item)
      rows.push({
        id: item.ref.id,
        kind,
        parentId: item.parentId === undefined ? subject.id : item.parentId || null,
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

/** The naming input's row, at the slot the create will land in. */
export function rowFromDraft(draft: CreateDraft): SidebarRow {
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

/** An item that names no parent sits under its operation's subject; a repo
 *  subject is drawn as its home row, id'd by the chat that owns it. */
export function placeUnderRepoHomes(rows: SidebarRow[], repos: readonly Repo[]): SidebarRow[] {
  const homes = new Map<string, string>()
  for (const r of repos) {
    if (r.defaultWorkspaceId)
      homes.set(
        r.id,
        resolveHomeOwnerId(r.defaultWorkspaceId, r.defaultOwningChatId, r.chats ?? []),
      )
  }
  return rows.map((row) => {
    const home = row.parentId === null ? undefined : homes.get(row.parentId)
    return home === undefined ? row : { ...row, parentId: home }
  })
}
