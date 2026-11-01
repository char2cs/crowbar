import { useCallback, useMemo } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useHomeTreeStore, type HomeTree } from '@/lib/store/home-tree'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useRemovalTrayStore, type RemovalEntry } from '@/lib/store/sidebar-removal'
import { useCreateDraftStore, type CreateDraft } from '@/lib/store/create-draft'
import {
  newestProblem,
  operationsState,
  selectFailedOps,
  useOperations,
  type EntityRef,
  type Operation,
} from '@/lib/store/operations'
import { entityKey, isEntityPresent, usePresentEntities } from '@/lib/store/entity-presence'
import {
  getHomeOwningChatId,
  getHomeWorkspaceId,
  useHomeWorkspaceStates,
} from '@/features/workspace/lib/home-workspace-resolver'
import { applyPendingRemovals, renderedHiddenIds } from '@/components/layout/removal-plan'
import { legacyTreeIndex, type LegacyHome } from './legacy-tree-index'
import { viewsFromStores, type ProblemLookup } from './views'
import { createItemRefs, type TreeInput } from './rows'

/** The store reads every project's input is built from. */
interface Sources {
  repos: readonly Repo[]
  hidden: ReadonlySet<string>
  removals: readonly RemovalEntry[]
  /** Whether a repo's rows may be drawn yet. */
  drawn: (repo: Repo) => boolean
  homeTrees: Readonly<Record<string, HomeTree>>
  /** A project's resolved home workspace and the chat that owns it. */
  homeOf: (projectId: string) => { workspaceId: string | null; owningChatId: string | null }
  operations: readonly Operation[]
  draft: CreateDraft | null
  problemOf: ProblemLookup
  isPresent: (ref: EntityRef) => boolean
}

function homeOf(src: Sources, projectId: string): LegacyHome | null {
  const { workspaceId, owningChatId } = src.homeOf(projectId)
  const tree = src.homeTrees[projectId]
  if (!workspaceId || !tree) return null
  return {
    workspaceId,
    owningChatId: owningChatId ?? undefined,
    chats: tree.chats.filter((c) => !src.hidden.has(c.id)),
    folders: tree.folders.filter((f) => !src.hidden.has(f.id)),
  }
}

function inputFrom(src: Sources, projectId: string): TreeInput {
  const repos = applyPendingRemovals(
    src.repos.filter((r) => r.projectId === projectId && src.drawn(r)),
    src.hidden,
  )
  const home = homeOf(src, projectId)
  return {
    projectId,
    homeWorkspaceId: home?.workspaceId ?? null,
    index: legacyTreeIndex(repos, home),
    views: viewsFromStores(repos, home, src.problemOf),
    operations: src.operations,
    isPresent: src.isPresent,
    ui: { removals: src.removals, draft: src.draft },
  }
}

const problemsIn =
  (failed: Readonly<Record<string, Operation>>): ProblemLookup =>
  (...ids) =>
    newestProblem(failed, ids)

/** One project's input as the stores hold it now, for an event handler: every repo it
 *  holds (or the ones the handler was handed), since a handler acts on rows already drawn. */
export function treeInputFor(
  projectId: string,
  repos: readonly Repo[] = useSidebarStore.getState().repos,
): TreeInput {
  const tray = useRemovalTrayStore.getState()
  const ops = operationsState()
  return inputFrom(
    {
      repos,
      hidden: renderedHiddenIds(tray.hiddenIds, tray.entries),
      removals: tray.entries,
      drawn: () => true,
      homeTrees: useHomeTreeStore.getState().trees,
      homeOf: (id) => ({
        workspaceId: getHomeWorkspaceId(id),
        owningChatId: getHomeOwningChatId(id),
      }),
      operations: Object.values(ops.ops),
      draft: useCreateDraftStore.getState().draft,
      problemOf: problemsIn(selectFailedOps(ops)),
      isPresent: isEntityPresent,
    },
    projectId,
  )
}

/** Every project's input for a component, re-derived when a store it reads changes. */
export function useTreeInput(): (projectId: string) => TreeInput {
  const repos = useSidebarStore((s) => s.repos)
  const removals = useRemovalTrayStore((s) => s.entries)
  const trayHidden = useRemovalTrayStore((s) => s.hiddenIds)
  const seeded = useFolderSignalStore((s) => s.seededRepoIds)
  const seededWorkspaces = useFolderSignalStore((s) => s.seededWorkspaceRepoIds)
  const homeTrees = useHomeTreeStore((s) => s.trees)
  const homes = useHomeWorkspaceStates()
  const ops = useOperations((s) => s.ops)
  const draft = useCreateDraftStore((s) => s.draft)
  // Shallow: rows re-derive only when some failure changes, not on every frame.
  const failed = useOperations(useShallow(selectFailedOps))
  const operations = useMemo(() => Object.values(ops), [ops])
  const present = usePresentEntities(useMemo(() => createItemRefs(operations), [operations]))
  const src = useMemo<Sources>(
    () => ({
      repos,
      hidden: renderedHiddenIds(trayHidden, removals),
      removals,
      // A repo draws once its tree or its workspaces have seeded, so a row's id never changes under it.
      drawn: (r) => seeded.has(r.id) || seededWorkspaces.has(r.id),
      homeTrees,
      homeOf: (id) => {
        const state = homes.get(id)
        return { workspaceId: state?.wsId ?? null, owningChatId: state?.owningChatId ?? null }
      },
      operations,
      draft,
      problemOf: problemsIn(failed),
      isPresent: (ref) => present.has(entityKey(ref)),
    }),
    [
      repos,
      trayHidden,
      removals,
      seeded,
      seededWorkspaces,
      homeTrees,
      homes,
      operations,
      draft,
      failed,
      present,
    ],
  )
  return useCallback((projectId: string) => inputFrom(src, projectId), [src])
}
