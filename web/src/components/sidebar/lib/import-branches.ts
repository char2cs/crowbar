import { useSidebarStore } from '@/lib/store/sidebar'
import { importBranches } from '@/lib/api'
import { runOperation } from '@/lib/store/run-operation'
import type { OperationItem } from '@/lib/store/operations'

function importLabel(branches: readonly string[]): string {
  return branches.length === 1 ? `Import ${branches[0]}` : `Import ${branches.length} branches`
}

/**
 * Imports `branches` into `repoId` as an operation, minting each branch's chat
 * id so the sidebar draws its row from the operation until the chat arrives,
 * and keeps it with its reason if it fails. The daemon creates the workspaces
 * of `lockedBranches` locked; a mixed choice is one batch per lock state.
 */
export function performImportBranches(
  repoId: string,
  branches: readonly string[],
  lockedBranches: readonly string[] = [],
): void {
  const projectId = useSidebarStore.getState().repos.find((r) => r.id === repoId)?.projectId
  if (!projectId || branches.length === 0) return
  const locked = new Set(lockedBranches)
  for (const lock of [true, false]) {
    const batch = branches.filter((b) => locked.has(b) === lock)
    if (batch.length === 0) continue
    const named = batch.map((name) => ({ name, chatId: crypto.randomUUID() }))
    const items: OperationItem[] = named.map((b) => ({
      ref: { kind: 'chat', id: b.chatId },
      label: b.name,
      state: 'accepted',
    }))
    // A refusal fails the local operation: its rows keep the reason, the watcher toasts it.
    runOperation(
      {
        kind: 'branch.import',
        subject: { kind: 'repo', id: repoId },
        projectId,
        label: importLabel(batch),
        items,
      },
      (opId) => importBranches(projectId, repoId, named, lock, opId),
    )
  }
}
