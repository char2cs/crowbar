import { useCallback, useMemo, useSyncExternalStore } from 'react'
import { useShallow } from 'zustand/react/shallow'
import type { Repo } from '@/core/entities/repo-types'
import { readSelect, selectHomeTrees, selectRepos, useSelect } from '@/core/entities/select'
import type { HomeTree } from '@/core/entities/views-from-entities'
import { entityStore, holdsEntity, type EntityState } from '@/core/entities/entity-store'
import { entityKey } from '@/core/stream/host-scope'
import { resolvedHostOf } from '@/lib/host-resolve'
import { treeUiStore, useTreeUi, type CreateDraft, type RemovalEntry } from './tree-ui-store'
import {
  newestProblem,
  operationsState,
  selectFailedOps,
  useOperations,
  type EntityRef,
  type Operation,
} from '@/core/entities/operations'
import {
  getHomeOwningChatId,
  getHomeWorkspaceId,
  useHomeWorkspaceStates,
} from '@/features/workspace/lib/home-workspace-resolver'
import { applyPendingRemovals, renderedHiddenIds } from '@/components/layout/removal-plan'
import { legacyTreeIndex, type LegacyHome } from './legacy-tree-index'
import { viewsFromStores, type ProblemLookup } from './views'
import { createItemRefs, type TreeInput } from './rows'

interface Sources {
  repos: readonly Repo[]
  hidden: ReadonlySet<string>
  removals: readonly RemovalEntry[]
  homeTrees: Readonly<Record<string, HomeTree>>
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
    src.repos.filter((r) => r.projectId === projectId),
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

const refKey = (ref: EntityRef): string => `${ref.kind}:${ref.id}`

function heldOn(s: EntityState, hostId: string | null, ref: EntityRef): boolean {
  return hostId !== null && holdsEntity(s, { ...ref, id: entityKey(hostId, ref.id) })
}

const problemsIn =
  (failed: Readonly<Record<string, Operation>>): ProblemLookup =>
  (...ids) =>
    newestProblem(failed, ids)

export function treeInputFor(
  projectId: string,
  repos: readonly Repo[] = readSelect(selectRepos),
): TreeInput {
  const tray = treeUiStore.getState()
  const ops = operationsState()
  return inputFrom(
    {
      repos,
      hidden: renderedHiddenIds(tray.hiddenIds, tray.removals),
      removals: tray.removals,
      homeTrees: readSelect(selectHomeTrees),
      homeOf: (id) => ({
        workspaceId: getHomeWorkspaceId(id),
        owningChatId: getHomeOwningChatId(id),
      }),
      operations: Object.values(ops.ops),
      draft: treeUiStore.getState().draft,
      problemOf: problemsIn(selectFailedOps(ops)),
      isPresent: (ref) => heldOn(entityStore.getState(), resolvedHostOf({ projectId }), ref),
    },
    projectId,
  )
}

export function useTreeInput(): (projectId: string) => TreeInput {
  const repos = useSelect(selectRepos)
  const removals = useTreeUi((s) => s.removals)
  const trayHidden = useTreeUi((s) => s.hiddenIds)
  const homeTrees = useSelect(selectHomeTrees)
  const homes = useHomeWorkspaceStates()
  const ops = useOperations((s) => s.ops)
  const draft = useTreeUi((s) => s.draft)
  const failed = useOperations(useShallow(selectFailedOps))
  const operations = useMemo(() => Object.values(ops), [ops])
  const refs = useMemo(
    () => operations.map((op) => ({ op, refs: createItemRefs([op]) })),
    [operations],
  )
  const presentKey = useSyncExternalStore(entityStore.subscribe, () => {
    const s = entityStore.getState()
    const { hostOf } = operationsState()
    const held: string[] = []
    for (const { op, refs: items } of refs) {
      const hostId = hostOf[op.id] ?? resolvedHostOf({ projectId: op.projectId })
      for (const ref of items) if (heldOn(s, hostId, ref)) held.push(refKey(ref))
    }
    return held.join('\n')
  })
  const present = useMemo(() => new Set(presentKey.split('\n')), [presentKey])
  const src = useMemo<Sources>(
    () => ({
      repos,
      hidden: renderedHiddenIds(trayHidden, removals),
      removals,
      homeTrees,
      homeOf: (id) => {
        const state = homes.get(id)
        return { workspaceId: state?.wsId ?? null, owningChatId: state?.owningChatId ?? null }
      },
      operations,
      draft,
      problemOf: problemsIn(failed),
      isPresent: (ref) => present.has(refKey(ref)),
    }),
    [repos, trayHidden, removals, homeTrees, homes, operations, draft, failed, present],
  )
  return useCallback((projectId: string) => inputFrom(src, projectId), [src])
}
