import { Fragment, useCallback, useMemo, useState } from 'react'
import { Plus } from 'lucide-react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useSidebarStore, EMPTY_FOLDERS, type Repo } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore, importProjectAndSync, EMPTY_PROJECTS } from '@/lib/store/projects'
import { dataOf } from '@/lib/loadable'
import { InlineError } from '@/components/ui/inline-error'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { ROW_BASE } from './workspace-row-base'
import { RemovalTray } from './removal-tray'
import { RepoSection } from './repo-section'
import { WorkspaceTreeProvider, useWorkspaceTreeActions } from './workspace-tree-context'
import { ProjectHomeRow } from './project-home-row'
import { applyPendingRemovals } from './removal-plan'
import { RowContextMenu } from './row-context-menu'
import { useTreeKeyboard } from './use-tree-keyboard'
import { toggleRepoKeepingRows, toggleRowKeepingRows } from './row-selection'
import { dragSubjectsFor, readSelectedSubjects } from './drop-target-dom'
import {
  buildSidebarTree,
  indexSidebarTree,
  EMPTY_REPO_TREE,
  type SidebarRepoTree,
} from './workspace-tree-utils'
import type { DropRow } from './drop-target-dom'
import type { Project } from '@/lib/types'

/**
 * Stable empty repo list for a project whose repos have not landed yet. Same
 * rule as EMPTY_PROJECTS (lib/store/projects.ts): a fresh `[]` per call is a new
 * reference every render, which React eventually reports as "Maximum update
 * depth exceeded" somewhere unrelated. Read-only by convention.
 */
const EMPTY_REPOS: Repo[] = []

interface ProjectGroup {
  id: string
  name: string
  repos: Repo[]
}

function WorkspaceTreeInner() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const allRepos = useSidebarStore((s) => s.repos)
  const hiddenIds = useRemovalTrayStore((s) => s.hiddenIds)
  const collapsedRepos = useSidebarStore((s) => s.collapsedRepos)
  const collapsedProjects = useSidebarStore((s) => s.collapsedProjects)
  const projects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  const {
    creatingChildOf,
    startCreating,
    confirmCreate,
    cancelCreate,
    pendingCreates,
    clearPendingCreate,
    startImport,
    removeRows,
  } = useWorkspaceTreeActions()

  // The rows the tray is holding are taken out here, once, ahead of everything
  // that reads the tree — so a held row is absent from the render, from the
  // drag's hit test and from the keyboard's order alike. Hiding is all it is:
  // the store still has every one of them, which is what makes Cancel a matter
  // of dropping an id rather than rebuilding a subtree.
  const repos = useMemo(() => applyPendingRemovals(allRepos, hiddenIds), [allRepos, hiddenIds])
  // Which repo header row is in inline-rename mode (double-clicked its name),
  // mirroring the branch rows' renamingId — null when none. Owned here, not per
  // RepoSection, so opening one row's rename editor closes any other's.
  const [renamingRepoId, setRenamingRepoId] = useState<string | null>(null)
  const [importProjectOpen, setImportProjectOpen] = useState(false)
  const wsListData = useWorkspaceListStore((s) => s.data)
  const retryWorkspaces = useCallback(() => {
    void useWorkspaceListStore.getState().fetch()
  }, [])
  const activeWorkspaceId = pathname.match(/\/ide\/[^/]+\/[^/]+\/([^/]+)/)?.[1] ?? ''

  // Per-repo tree, memoized so a drag/hover re-render (or any unrelated state
  // change) doesn't rebuild every repo's node graph from scratch.
  //
  // `repos` is the COMPLETE input to buildSidebarTree, and it has to be: it
  // carries all three things the tree is built from — the workspace rows, their
  // `order`, and the repo's `folders`. Every sidebar write (setRepos, mergeRepos,
  // applyWorkspaceDTO) hands out a new repos array AND a new repo object, so a
  // reorder or a folder move changes this reference and repaints. Keying this on
  // `repo.workspaces` alone — which is all the pre-Wave-4 builder consumed —
  // would have swallowed both silently: the sidebar would keep painting the old
  // order until something unrelated happened to re-render it.
  //
  // The keep set's descendant lookups are built in the same pass and cached on
  // the same memo: a folded row has to answer "what am I holding?" on every
  // keep-set change, and walking the graph per row per render would make that
  // cost O(rows × depth) for a question the tree already knows the answer to.
  const treeByRepo = useMemo(() => {
    const map = new Map<string, SidebarRepoTree>()
    for (const repo of repos) {
      const roots = buildSidebarTree(repo.workspaces, repo.folders ?? EMPTY_FOLDERS)
      map.set(repo.id, indexSidebarTree(roots, repo.id))
    }
    return map
  }, [repos])

  // Every project in one list, each holding its own repos. Projects come from
  // the always-on /v0/projects stream; a repo whose ProjectDTO has not arrived
  // yet (a fresh import, or the list still in flight) still gets a section, or
  // its rows would blink out of the sidebar until the project list caught up.
  const groups = useMemo<ProjectGroup[]>(() => {
    const reposByProject = new Map<string, Repo[]>()
    for (const repo of repos) {
      const projectId = repo.projectId ?? ''
      const existing = reposByProject.get(projectId)
      if (existing) existing.push(repo)
      else reposByProject.set(projectId, [repo])
    }

    const out: ProjectGroup[] = []
    const placed = new Set<string>()
    for (const project of projects) {
      placed.add(project.id)
      out.push({
        id: project.id,
        name: project.name,
        repos: reposByProject.get(project.id) ?? EMPTY_REPOS,
      })
    }
    for (const [projectId, list] of reposByProject) {
      if (projectId === '' || placed.has(projectId)) continue
      out.push({ id: projectId, name: 'home', repos: list })
    }
    return out
  }, [projects, repos])

  const handleWorkspaceClick = useCallback(
    (wsId: string, projectId: string, repoId: string) => {
      void navigate({ to: '/ide/$projectId/$repoId/$wsId', params: { projectId, repoId, wsId } })
    },
    [navigate],
  )

  const handleImportProject = useCallback((project: Project) => {
    importProjectAndSync(project)
    setImportProjectOpen(false)
  }, [])

  // Left/Right go through the same collapse the row's own chevron does, keep-set
  // snapshot included — a row folded by the keyboard must hold what a row folded
  // by the mouse holds.
  const handleToggleRow = useCallback(
    (row: DropRow) => {
      if (row.kind === 'project') {
        useSidebarStore.getState().toggleProject(row.id)
        return
      }
      const tree = treeByRepo.get(row.kind === 'repo' ? row.id : (row.repoId ?? ''))
      if (!tree) return
      if (row.kind === 'repo') toggleRepoKeepingRows(row.id, tree.index, activeWorkspaceId)
      else toggleRowKeepingRows(row.id, tree.index, activeWorkspaceId)
    },
    [treeByRepo, activeWorkspaceId],
  )

  // Delete takes the same route a drop on the editor pane does: the rows are
  // held, not deleted, and the tray is the only thing that ever commits one.
  const handleRemove = useCallback(
    (focused: DropRow | null) => {
      const selection = readSelectedSubjects()
      const subjects = focused ? dragSubjectsFor(focused, selection) : selection
      if (subjects.length > 0) removeRows(subjects)
    },
    [removeRows],
  )

  const { treeRef, onKeyDown, onFocus } = useTreeKeyboard({
    onToggle: handleToggleRow,
    onRemove: handleRemove,
  })

  if (wsListData.status === 'error' && repos.length === 0) {
    return (
      <div className="flex flex-1 flex-col overflow-hidden">
        <InlineError error={wsListData.error} onRetry={retryWorkspaces} />
      </div>
    )
  }

  // NOTE: this outer div must remain the SINGLE flex child of the carousel
  // panel — sidebar-carousel.tsx:43-96 computes scroll-snap offsets from the
  // panel's own width, and an extra sibling here breaks that arithmetic.
  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        {/* A real tree, declared as one. The rows were `role="button"`, which
            takes presentational children — so every per-row control nested
            inside them (import, add-child, expand) was stripped of its own
            semantics for assistive tech. `treeitem` is what these rows
            actually are, and unlike `button` it permits interactive
            descendants. Project, repo, folder and workspace rows all sit in
            this one tree; there is no second scroller and no pushed panel.

            The tree — not each row — carries the keyboard. `treeitem` promises
            arrow keys, type-to-jump and Enter, and one handler on the container
            keeps that promise for every row at once; the alternative is the
            same listener bound N times over. Focus and the single tab stop are
            moved through the DOM inside it, so walking the list costs no
            renders at all. */}
        <div
          ref={treeRef}
          role="tree"
          aria-label="Workspaces"
          className="pb-1"
          onKeyDown={onKeyDown}
          onFocus={onFocus}
        >
          {groups.map((group, index) => {
            const isCollapsed = collapsedProjects.has(group.id)
            return (
              <Fragment key={group.id}>
                {/* Decorative only — the rule separates two sections that are
                    already announced by their own rows, so it stays out of the
                    a11y tree rather than injecting a `separator` between
                    treeitems. */}
                {index > 0 && <hr aria-hidden="true" className="mx-3 my-1.5 border-border" />}
                <ProjectHomeRow
                  project={group}
                  isCollapsed={isCollapsed}
                  hasRepos={group.repos.length > 0}
                />
                {!isCollapsed && (
                  // Repos sit at the PROJECT's own indent: the first indent step
                  // belongs to a repo's workspaces, not to the repo row.
                  <div role="group">
                    {group.repos.map((repo) => (
                      <RepoSection
                        key={repo.id}
                        repo={repo}
                        tree={treeByRepo.get(repo.id) ?? EMPTY_REPO_TREE}
                        isCollapsed={collapsedRepos.has(repo.id)}
                        activeWorkspaceId={activeWorkspaceId}
                        onWorkspaceClick={handleWorkspaceClick}
                        renamingRepoId={renamingRepoId}
                        setRenamingRepoId={setRenamingRepoId}
                        creatingChildOf={creatingChildOf}
                        startCreating={startCreating}
                        confirmCreate={confirmCreate}
                        cancelCreate={cancelCreate}
                        pendingCreates={pendingCreates}
                        clearPendingCreate={clearPendingCreate}
                        startImport={startImport}
                      />
                    ))}
                  </div>
                )}
              </Fragment>
            )
          })}
        </div>

        {/* One row closes the list. This is what the pushed "Projects" panel used
            to be reached for; with every project already on screen there is
            nothing left for that screen to show.

            Deliberately OUTSIDE the tree, though inside the same scroller: it is
            an action, not a node. A tree announces its children by position
            ("3 of 4"), and a bare <button> among the treeitems would both break
            that count and claim a place in a hierarchy it has no depth in. Same
            reasoning that made the rows treeitems in the first place, applied
            the other way round. */}
        <button
          type="button"
          className={cn(
            ROW_BASE,
            'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
          )}
          onClick={() => setImportProjectOpen(true)}
        >
          <span className="inline-flex size-5 shrink-0 items-center justify-center">
            <Plus className="size-3.5" />
          </span>
          <span className="min-w-0 flex-1 truncate text-left">Import project</span>
        </button>
      </ScrollArea>
      <RemovalTray />
      <ImportProjectModal
        open={importProjectOpen}
        onOpenChange={setImportProjectOpen}
        onImport={handleImportProject}
      />
      <RowContextMenu treeRef={treeRef} />
    </div>
  )
}

export function WorkspaceTree() {
  return (
    <WorkspaceTreeProvider>
      <WorkspaceTreeInner />
    </WorkspaceTreeProvider>
  )
}
