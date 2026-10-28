import { dataOf } from '@/lib/loadable'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { EntityRef } from '@/lib/store/operations'

/** Whether the entity `ref` names is in the store the sidebar draws it from. */
export function isEntityPresent(ref: EntityRef): boolean {
  const repos = useSidebarStore.getState().repos
  switch (ref.kind) {
    case 'project':
      return (dataOf(useProjectDataStore.getState().data) ?? []).some((p) => p.id === ref.id)
    case 'repo':
      return repos.some((r) => r.id === ref.id)
    case 'workspace':
      return repos.some(
        (r) => r.defaultWorkspaceId === ref.id || r.workspaces.some((w) => w.id === ref.id),
      )
    case 'chat':
      return (
        repos.some((r) => r.chats?.some((c) => c.id === ref.id)) ||
        Object.values(useHomeTreeStore.getState().trees).some((t) =>
          t.chats.some((c) => c.id === ref.id),
        )
      )
  }
}

/** Calls `listener` whenever a store `isEntityPresent` reads changes. */
export function subscribeEntityPresence(listener: () => void): () => void {
  const stops = [
    useSidebarStore.subscribe(listener),
    useProjectDataStore.subscribe(listener),
    useHomeTreeStore.subscribe(listener),
  ]
  return () => stops.forEach((stop) => stop())
}
