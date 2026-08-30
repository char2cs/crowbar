import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  NO_MODES,
  type AllowedModes,
  type DropPolicy,
} from '@/components/tree-dnd/drop-core'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * What a drag may do to a row in the unified sidebar tree, and where the
 * indicator goes — the one matrix that replaces the workspace tree's
 * `SIDEBAR_DROP_POLICY` (`components/layout/drop-rules.ts`) and the chats
 * tree's `CHAT_DROP_POLICY` (`features/agent/tree/lib/chat-drop.ts`) now that
 * both trees are one `SidebarRow` forest (Task 4/8).
 *
 * A `SidebarRow` carries no `repoId`/`projectId` of its own — Task 4 kept it
 * to only what the tree draws. Scope is resolved the same way
 * `sidebar/lib/row-actions.ts` already resolves a row id against the live
 * tree (`performCreateFolder`, `performRenameRow`): a row's id is matched
 * against a repo's three id spaces (its default/home workspace, a tree
 * workspace, or a folder). This runs from pointer-driven code (a drag, never
 * a render), so reading `getState()` here follows the store's own usage rule.
 */

interface RowScope {
  repoId: string
  projectId: string | undefined
}

/**
 * A row's owning repo/project, or null if nothing in the live store claims
 * this id — a race, or a row kind (`chat`, `workflow`) with no producer
 * feeding `repos` yet. Every rule below refuses rather than guesses when this
 * comes back null, same posture the old policies took on an unrecognised
 * selection.
 */
function resolveRowRepo(rowId: string): RowScope | null {
  const repo = useSidebarStore
    .getState()
    .repos.find(
      (r) =>
        r.defaultWorkspaceId === rowId ||
        r.workspaces.some((w) => w.id === rowId) ||
        r.folders?.some((f) => f.id === rowId),
    )
  return repo ? { repoId: repo.id, projectId: repo.projectId } : null
}

/**
 * Which of before/after/into this drag may do to this target.
 *
 * Same-project rule generalizes `drop-rules.ts`'s same-repo rule (its
 * `allowedModes`, ~line 80: `subjects.some((s) => s.repoId !== target.repoId)`):
 * rows now span one project's whole forest rather than one repo's workspaces
 * (`rows-for-project.ts`), so the boundary a drag may not cross moved up one
 * level. Cross-repo, within-project drags stay legal, but only for a row that
 * owns no worktree (design spec §8.3, "cross-repo drag is legal only for a
 * chat with no worktree") — reparenting a fork across repos makes no sense,
 * but a bubble chat pinned to nothing on disk can move freely.
 *
 * Working-row refusal (spec §8.3) reads `SidebarRow.working`, refusing every
 * mode when any dragged row is working — the UI mirror of the backend plan's
 * `guardNotWorking`, duplicated here deliberately since a drag needs to
 * refuse before a network round trip, not after.
 */
export function allowedModes(subjects: readonly SidebarRow[], target: SidebarRow): AllowedModes {
  if (subjects.length === 0) return NO_MODES
  if (subjects.some((s) => s.working)) return NO_MODES
  // Never drop a row onto itself.
  if (subjects.some((s) => s.id === target.id)) return NO_MODES
  // A mixed-kind selection is not a thing the sidebar can express; refuse
  // rather than guess which class wins (carried over from both old policies).
  const kind = subjects[0].kind
  if (subjects.some((s) => s.kind !== kind)) return NO_MODES

  const targetScope = resolveRowRepo(target.id)
  if (!targetScope) return NO_MODES

  for (const subject of subjects) {
    const subjectScope = resolveRowRepo(subject.id)
    if (!subjectScope) return NO_MODES
    if (subjectScope.projectId !== targetScope.projectId) return NO_MODES
    if (subjectScope.repoId !== targetScope.repoId && subject.ownsWorktree) return NO_MODES
  }

  return ALL_MODES
}

/**
 * The outer band of a row reorders; the middle nests. A folder gets the
 * container band — filing into one is cheap and common. Everything else gets
 * the heavy band: nesting under a branch re-parents a fork, nesting under a
 * chat makes the row one of its threads, and nesting into an as-yet-unwired
 * `workflow` row is assumed the same weight until one exists to say
 * otherwise — all heavier moves that deserve a harder-to-hit target. Matches
 * `drop-rules.ts`'s own `edgeBandFor` verbatim, generalized off `SidebarRowKind`.
 */
export function edgeBandFor(kind: string): number {
  return kind === 'folder' ? EDGE_BAND_CONTAINER : EDGE_BAND_HEAVY
}

export const SIDEBAR_DROP_POLICY: DropPolicy<SidebarRow, SidebarRow> = {
  allowedModes,
  edgeBandFor,
}
