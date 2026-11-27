import { readSelect, selectRepo } from '@/core/entities/select'
import { importBranches } from '@/lib/api'
import { runOperation } from '@/core/entities/run-operation'
import type { OperationItem } from '@/core/entities/operations'

function importLabel(branches: readonly string[]): string {
  return branches.length === 1 ? `Import ${branches[0]}` : `Import ${branches.length} branches`
}

export function performImportBranches(
  repoId: string,
  branches: readonly string[],
  lockedBranches: readonly string[] = [],
): void {
  const projectId = readSelect((s) => selectRepo(s, repoId))?.projectId
  if (!projectId || branches.length === 0) return
  const locked = new Set(lockedBranches)
  const named = branches.map((name) => ({
    name,
    chatId: crypto.randomUUID(),
    lock: locked.has(name),
  }))
  const items: OperationItem[] = named.map((b) => ({
    ref: { kind: 'chat', id: b.chatId },
    label: b.name,
    state: 'accepted',
  }))
  runOperation(
    {
      kind: 'branch.import',
      subject: { kind: 'repo', id: repoId },
      projectId,
      label: importLabel(branches),
      items,
    },
    (opId) => importBranches(projectId, repoId, named, opId),
  )
}
