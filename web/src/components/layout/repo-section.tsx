import { useCallback, useState } from 'react'
import { DownloadCloud } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  ROW_BASE,
  ROW_ACTIVE,
  ROW_INACTIVE,
  ADD_GLYPH_PATH,
  ROW_SUB_ACTION,
  ROW_SUB_ACTION_GLYPH,
} from './workspace-row-base'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { findWorkspaceForBranch } from '@/lib/workspace/branch-workspace'
import { WorkspaceTreeItem } from './workspace-tree-item'
import { PendingCreateRow } from './pending-create-row'
import { RepoIconPopover } from './repo-icon-popover'
import { RepoImportDialog } from './repo-import-dialog'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import type { WorkspaceTreeNode } from './workspace-tree-utils'
import type { PendingCreate } from './workspace-tree-context'
import { renameRepo } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'

interface RepoSectionProps {
  repo: Repo
  /** Root nodes of this repo's workspace tree (already built by the parent). */
  roots: WorkspaceTreeNode[]
  isCollapsed: boolean
  isRepoDragOver: boolean
  collapsedRepos: Set<string>
  activeWorkspaceId: string
  onWorkspaceClick: (wsId: string, projectId: string, repoId: string) => void
  /**
   * Which repo header row is in inline-rename mode, owned by the parent so that
   * only ONE row across the whole tree can be renaming at a time.
   */
  renamingRepoId: string | null
  setRenamingRepoId: (repoId: string | null) => void
  creatingChildOf: { repoId: string; parentId: string } | null
  startCreating: (repoId: string, parentId: string) => void
  confirmCreate: (branch: string) => void
  cancelCreate: () => void
  pendingCreates: Map<string, PendingCreate>
  clearPendingCreate: (tempId: string) => void
  startImport: (repoId: string, branches: string[]) => void
}

/**
 * One repo in the workspaces sidebar: its header row (which IS the repo-home
 * default workspace's row) plus the subtree beneath it — the inline create
 * input, any optimistic pending-create rows, and the workspace tree itself.
 */
export function RepoSection({
  repo,
  roots,
  isCollapsed,
  isRepoDragOver,
  collapsedRepos,
  activeWorkspaceId,
  onWorkspaceClick,
  renamingRepoId,
  setRenamingRepoId,
  creatingChildOf,
  startCreating,
  confirmCreate,
  cancelCreate,
  pendingCreates,
  clearPendingCreate,
  startImport,
}: RepoSectionProps) {
  const openRepoHome = useCallback(() => {
    if (repo.projectId && repo.defaultWorkspaceId) {
      onWorkspaceClick(repo.defaultWorkspaceId, repo.projectId, repo.id)
    }
  }, [onWorkspaceClick, repo.defaultWorkspaceId, repo.id, repo.projectId])

  const [importOpen, setImportOpen] = useState(false)

  // react-doctor-disable-next-line js-combine-iterations -- pendingCreates is the whole tree's in-flight create operations (bounded by concurrent UI actions, realistically 0-2 at once); a single-pass rewrite here would cost JSX readability for no measurable gain.
  const pendingCreateRows = Array.from(pendingCreates.entries())
    .filter(([, p]) => p.repoId === repo.id && p.parentId === repo.defaultWorkspaceId)
    .map(([tempId, pending]) => (
      <PendingCreateRow
        key={tempId}
        tempId={tempId}
        pending={pending}
        paddingLeft={14}
        onClear={clearPendingCreate}
      />
    ))

  return (
    <div className="mb-1">
      <div
        role="treeitem"
        aria-expanded={!isCollapsed}
        tabIndex={0}
        className={cn(
          ROW_BASE,
          'group',
          activeWorkspaceId !== '' && activeWorkspaceId === repo.defaultWorkspaceId
            ? ROW_ACTIVE
            : ROW_INACTIVE,
          isRepoDragOver && 'ring-1 ring-ring',
        )}
        data-repo-drop={repo.id}
        aria-label={`Open ${repo.name}`}
        data-oracle-id="repo-section"
        onClick={() => {
          // Clicks inside the inline rename editor must not navigate.
          if (renamingRepoId === repo.id) return
          openRepoHome()
        }}
        onKeyDown={(e) => {
          if (
            renamingRepoId !== repo.id &&
            e.target === e.currentTarget &&
            (e.key === 'Enter' || e.key === ' ')
          ) {
            e.preventDefault()
            openRepoHome()
          }
        }}
      >
        {/* The repo avatar is the trigger for the icon-editing popover: clicking
            it edits the icon (the click is stopped from bubbling to the row);
            hovering reveals a pencil. During an agent turn the popover renders
            the spinner in place and is not editable. Repo rename stays on the
            name's double-click below. */}
        <RepoIconPopover repo={repo} />
        {renamingRepoId === repo.id ? (
          // No data-oracle-id: workspace-inline-input.tsx is a separate,
          // not-yet-ported Tier B target and carries none of its own — see
          // workspace-tree-item.tsx's identical rename slot for the full
          // reasoning.
          <WorkspaceInlineInput
            defaultValue={repo.name}
            placeholder="repository-name"
            onConfirm={(name) => {
              setRenamingRepoId(null)
              // The renamed RepoDTO arrives on the repos WS stream and
              // merges into the store, refreshing this row's name — no
              // optimistic write here; only surface a failure.
              if (repo.projectId) {
                void renameRepo(repo.projectId, repo.id, name).catch((err) => {
                  toast.error(err instanceof Error ? err.message : 'Failed to rename repository')
                })
              }
            }}
            onCancel={() => setRenamingRepoId(null)}
          />
        ) : (
          <span className="flex min-w-0 flex-1 items-baseline gap-1.5">
            {/* Clicks bubble straight to the row, so opening the repo home is
                instant — exactly like every other row in this tree. There is no
                double-click window to wait out: `dblclick` arrives only after
                both of its `click` events, so renaming navigates into the repo
                home on the way to the editor. That is the same trade the branch
                rows make (see workspace-tree-item.tsx), and it is benign here —
                the row's only destination is the home of the repo being renamed,
                and the sidebar holding the editor survives the navigation.

                Ellipsize rather than shrink-0: the trailing settings/add/collapse
                buttons are all shrink-0, so a name that refuses to shrink pushes
                them straight off the row and out of reach. */}
            <span
              className="min-w-0 truncate font-mono text-foreground"
              data-oracle-id="repo-section-label"
              data-oracle-line-sized="true"
              onDoubleClick={(e) => {
                e.stopPropagation()
                setRenamingRepoId(repo.id)
              }}
            >
              {repo.name}
            </span>
            {repo.defaultWorkspaceId && (
              <span className="hidden shrink-0 font-mono text-[11px] text-foreground/40 group-hover:inline">
                - default
              </span>
            )}
          </span>
        )}

        <button
          type="button"
          aria-label="Import branches"
          title="Import branches"
          className={ROW_SUB_ACTION}
          onClick={(e) => {
            e.stopPropagation()
            setImportOpen(true)
          }}
          data-oracle-id="repo-section-import"
        >
          <DownloadCloud className="size-3" />
        </button>

        {repo.defaultWorkspaceId && (
          <button
            type="button"
            aria-label="Add child workspace"
            className={ROW_SUB_ACTION}
            onClick={(e) => {
              e.stopPropagation()
              if (collapsedRepos.has(repo.id)) useSidebarStore.getState().toggleRepo(repo.id)
              startCreating(repo.id, repo.defaultWorkspaceId!)
            }}
            data-oracle-id="repo-section-add-child"
          >
            <svg
              aria-hidden="true"
              className="size-3"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
            >
              <path d={ADD_GLYPH_PATH} />
            </svg>
          </button>
        )}

        <button
          type="button"
          aria-label={isCollapsed ? 'Expand repo' : 'Collapse repo'}
          className={ROW_SUB_ACTION}
          onClick={(e) => {
            e.stopPropagation()
            useSidebarStore.getState().toggleRepo(repo.id)
          }}
          data-oracle-id="repo-section-collapse"
        >
          <svg
            aria-hidden="true"
            className={cn('size-3 transition-transform', !isCollapsed && 'rotate-90')}
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
          >
            <path d="M6 3l5 5-5 5" />
          </svg>
        </button>
      </div>
      {!isCollapsed && (
        <div role="group">
          {creatingChildOf?.repoId === repo.id &&
            creatingChildOf?.parentId === repo.defaultWorkspaceId && (
              <div style={{ paddingLeft: 14 }}>
                {/* This row's own chrome is anchored; WorkspaceInlineInput
                    inside it is not — see the rename slot above. */}
                <div
                  className={cn(ROW_BASE, 'border-transparent text-foreground')}
                  data-oracle-id="repo-section-create-input"
                >
                  <svg
                    aria-hidden="true"
                    className={cn('size-4', ROW_SUB_ACTION_GLYPH)}
                    viewBox="0 0 16 16"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="2"
                    strokeLinecap="round"
                  >
                    <path d={ADD_GLYPH_PATH} />
                  </svg>
                  <WorkspaceInlineInput
                    onConfirm={confirmCreate}
                    onCancel={cancelCreate}
                    resolveExisting={(b) => findWorkspaceForBranch(repo, b)}
                    onOpenExisting={(wsId) => {
                      cancelCreate()
                      if (repo.projectId) onWorkspaceClick(wsId, repo.projectId, repo.id)
                    }}
                  />
                </div>
              </div>
            )}
          {pendingCreateRows}
          {roots.map((node) => (
            <WorkspaceTreeItem
              key={node.workspace.id}
              node={node}
              depth={0}
              repoId={repo.id}
              projectId={repo.projectId ?? ''}
              activeWorkspaceId={activeWorkspaceId}
              onWorkspaceClick={onWorkspaceClick}
            />
          ))}
        </div>
      )}
      <RepoImportDialog
        projectId={repo.projectId ?? ''}
        repoId={repo.id}
        defaultBranch={repo.defaultBranch ?? ''}
        open={importOpen}
        onOpenChange={setImportOpen}
        onImport={(branches) => startImport(repo.id, branches)}
      />
    </div>
  )
}
