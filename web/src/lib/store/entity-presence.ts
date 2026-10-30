import { useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { dataOf } from '@/lib/loadable'
import { useHomeTreeStore, type HomeTree } from '@/lib/store/home-tree'
import { useProjectDataStore } from '@/lib/store/projects'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'
import type { EntityRef } from '@/lib/store/operations'

export const entityKey = (ref: EntityRef): string => `${ref.kind}:${ref.id}`

function inProjects(projects: readonly Project[], ref: EntityRef): boolean {
  return ref.kind === 'project' && projects.some((p) => p.id === ref.id)
}

function inRepos(repos: readonly Repo[], ref: EntityRef): boolean {
  switch (ref.kind) {
    case 'repo':
      return repos.some((r) => r.id === ref.id)
    case 'workspace':
      return repos.some(
        (r) => r.defaultWorkspaceId === ref.id || r.workspaces.some((w) => w.id === ref.id),
      )
    case 'chat':
      return repos.some((r) => r.chats?.some((c) => c.id === ref.id))
    default:
      return false
  }
}

function atHome(trees: Readonly<Record<string, HomeTree>>, ref: EntityRef): boolean {
  return (
    ref.kind === 'chat' && Object.values(trees).some((t) => t.chats.some((c) => c.id === ref.id))
  )
}

/** Whether the entity `ref` names is in the store the sidebar draws it from. */
export function isEntityPresent(ref: EntityRef): boolean {
  return (
    inProjects(dataOf(useProjectDataStore.getState().data) ?? [], ref) ||
    inRepos(useSidebarStore.getState().repos, ref) ||
    atHome(useHomeTreeStore.getState().trees, ref)
  )
}

function presentKeys(refs: readonly EntityRef[], present: (ref: EntityRef) => boolean): string[] {
  const keys: string[] = []
  for (const ref of refs) if (present(ref)) keys.push(entityKey(ref))
  return keys
}

/** The `entityKey`s of the refs present, re-read when a store holding one changes. */
export function usePresentEntities(refs: readonly EntityRef[]): ReadonlySet<string> {
  const projects = useProjectDataStore(
    useShallow((s) => presentKeys(refs, (r) => inProjects(dataOf(s.data) ?? [], r))),
  )
  const repos = useSidebarStore(useShallow((s) => presentKeys(refs, (r) => inRepos(s.repos, r))))
  const home = useHomeTreeStore(useShallow((s) => presentKeys(refs, (r) => atHome(s.trees, r))))
  return useMemo(() => new Set([...projects, ...repos, ...home]), [projects, repos, home])
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
