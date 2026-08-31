import { dataOf } from '@/lib/loadable'
import { getAllEntities } from '@/lib/persistence/entity-cache'
import { buildRepoTree, toSidebarChat, toSidebarFolder } from '@/lib/store/build-repo-tree'
import { EMPTY_PROJECTS, useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import type { ChatDTO, FolderDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

// ---------------------------------------------------------------------------
// Which projects the sidebar is currently SHOWING THE INSIDE OF.
//
// The sidebar used to be single-active-project from the transport up: one
// project's repo + workspace streams were subscribed, and `buildScopedRepoTree`
// hard-filtered the cross-project entity cache down to that one project. The
// sidebar now holds every project at once, and the naive version of that —
// subscribe everything at boot — is strictly WORSE than what it replaces: N×
// WebSocket subscriptions, N× snapshot replays on every reconnect, N× IndexedDB
// writes, and a full sidebar rebuild triggered by a frame from a project you
// cannot see.
//
// So the axis changed from "which project is active" to "which projects are
// visible": every project the user has NOT folded away, plus the active one.
// Cost is then proportional to what is on screen. A collapsed project costs
// exactly one row (its name, from the always-subscribed /v0/projects stream)
// and nothing else — nothing is withheld, because a collapsed project shows
// only its own row by definition.
//
// Note the polarity: open is the default. Showing every project at once is the
// point of the redesign, so collapse is an explicit act that buys the cost
// back, not the resting state that avoids paying it.
// ---------------------------------------------------------------------------

/**
 * Stable empty tree, returned whenever nothing is visible. Same rule as
 * EMPTY_PROJECTS (lib/store/projects.ts): handing back a fresh `[]` each call
 * makes a Zustand snapshot compare unstable — a new reference every read, which
 * React eventually reports as "Maximum update depth exceeded" somewhere
 * unrelated. Read-only by convention.
 */
const EMPTY_REPOS: Repo[] = []

/**
 * The projects whose repos + workspaces should be subscribed and rendered:
 * every known project the user has not folded away, plus the active one (which
 * stays visible even when its own row is collapsed — the app needs its repo
 * scope either way).
 *
 * "Known" is the always-on `/v0/projects` stream, so a project only starts
 * costing streams once its row actually exists. Before that list lands the set
 * is just the active project, which is exactly what it was pre-Wave-3.
 *
 * Reads store state directly — call it from effects, handlers and async
 * builders only, never from a render path.
 */
export function getVisibleProjectIds(): Set<string> {
  const collapsed = useSidebarStore.getState().collapsedProjects
  const projects = dataOf(useProjectDataStore.getState().data) ?? EMPTY_PROJECTS
  const visible = new Set<string>()
  for (const project of projects) {
    if (!collapsed.has(project.id)) visible.add(project.id)
  }
  const activeProjectId = useProjectStore.getState().activeProjectId
  if (activeProjectId) visible.add(activeProjectId)
  return visible
}

/**
 * Build the sidebar repo tree from the entity cache, scoped to the visible
 * projects. The cache is deliberately cross-project and long-lived (each
 * project's repo stream prunes only its own scope, and rows survive across
 * sessions), which is exactly what makes expanding a project instant and
 * offline-capable — but it also means an unfiltered read would surface repos
 * from projects the user has collapsed away.
 */
export async function readVisibleRepoTree(): Promise<Repo[]> {
  const [repos, workspaces, folders, chats] = await Promise.all([
    getAllEntities<RepoDTO>('crowbar_repos'),
    getAllEntities<WorkspaceDTO>('crowbar_workspaces'),
    getAllEntities<FolderDTO>('crowbar_folders'),
    getAllEntities<ChatDTO>('crowbar_chats'),
  ])
  const visible = getVisibleProjectIds()
  if (visible.size === 0) return EMPTY_REPOS
  // Folders are read here rather than left to default to []: this is the only
  // place the whole tree is assembled from the cache, so a folder that never
  // reaches it is a folder that never reaches the sidebar at all. They need no
  // project filter of their own — toSidebarRepo keeps only the ones whose
  // repoId matches a repo that survived the filter above.
  // Chats are read and filtered exactly as folders are — no project filter of
  // their own, because `toSidebarRepo` keeps only the rows whose repoId matches
  // a repo that survived the filter above. That repoId match is the whole
  // cross-repo guard: the cache holds every repo's rows at once.
  return buildRepoTree(
    repos.filter((repo) => visible.has(repo.projectId)),
    workspaces,
    folders.map(toSidebarFolder),
    chats.map(toSidebarChat),
  )
}
