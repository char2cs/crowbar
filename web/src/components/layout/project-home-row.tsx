import { useState } from 'react'
import { Library, FolderSymlink, LayoutGrid } from 'lucide-react'
import { useNavigate, useMatch } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE, ROW_SUB_ACTION } from './workspace-row-base'
import { useProjectStore, useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { dataOf } from '@/lib/loadable'
import { WorkspaceAgentSpinner } from './workspace-branch-icon'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { ProjectSwitcherPanel } from './project-switcher-panel'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'

function handleOpenSwitcher(e: React.MouseEvent) {
  e.stopPropagation()
  useSidebarNavStore.getState().push({
    id: 'project-switcher',
    title: 'Projects',
    component: <ProjectSwitcherPanel />,
  })
}

export function ProjectHomeRow() {
  const navigate = useNavigate()
  const projectId = useProjectStore((s) => s.activeProjectId)
  const projects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  const activeProject = projects.find((p) => p.id === projectId)
  const isActive = useMatch({ from: '/_shell/ide/$projectId/home', shouldThrow: false })
  // The home workspace's agent-working overlay, kept live at project scope (it
  // rides no repo, so no per-repo workspace stream carries it).
  const working = useHomeWorkspaceStore((s) => s.workspace?.working ?? false)
  const [addRepoOpen, setAddRepoOpen] = useState(false)

  function handleClick() {
    if (!projectId) return
    void navigate({ to: '/ide/$projectId/home', params: { projectId } })
  }

  function handleAddRepo(e: React.MouseEvent) {
    e.stopPropagation()
    setAddRepoOpen(true)
  }

  return (
    <>
      <div
        // react-doctor-disable-next-line prefer-tag-over-role -- can't be a real <button>: it contains two nested <button>s (Import repository, Switch project) below, and HTML forbids interactive content inside a <button>. role="button" + tabIndex + onKeyDown is the correct fallback for a clickable row with nested action buttons.
        role="button"
        tabIndex={0}
        className={cn(ROW_BASE, 'group', isActive ? ROW_ACTIVE : ROW_INACTIVE)}
        onClick={handleClick}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') handleClick()
        }}
      >
        {/* Match the repo-header rows: the mark sits in the same 20px (h-5 w-5)
            box the repo avatar uses, so the label lines up with the repo names
            rather than shifting left off a bare 14px glyph. While an agent works
            in the home workspace the glyph becomes the spinner — the same
            treatment the worktree rows get from WorkspaceBranchIcon.

            Library rather than a house: this row is a project holding many repos,
            and a shelf of spines says "collection" where a roof says "dwelling".
            Outline in both states — the row already signals selection with its
            raised surface (ROW_ACTIVE), so a filled glyph was a second, louder
            signal and the only solid mark in an otherwise all-outline sidebar. */}
        <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center">
          {working ? <WorkspaceAgentSpinner /> : <Library size={16} />}
        </span>
        <span className="min-w-0 flex-1 truncate font-mono text-left">
          {activeProject?.name ?? 'home'}
        </span>
        {/* Trailing actions: the shared UI Button (so they get a real tooltip —
            these are icon-only 24px controls with no visible label, and without
            one the only way to learn what they do is to click), wearing the
            shared ROW_SUB_ACTION chrome so they stay identical to the
            repo-header rows' settings/add/expand buttons.

            Density is the reason for the explicit size-6: the class string alone
            has no box, and Button's own `icon-xs` size variant is size-7 that
            only steps down to size-6 at the sm: breakpoint — which would make
            this row's actions 28px in a narrow window while every other row's
            stayed 24px. size-6 pins the 24px box at every width, matching what
            p-1.5 + a size-3 glyph produces on the raw rows. The glyph keeps its
            own `size-3`: buttonVariants only sizes svgs that carry no size-*
            class of their own.

            No strokeWidth on either glyph — Lucide's default is the sidebar's
            weight (see workspace-row-base.ts); an override here made these the
            boldest marks on screen. */}
        <Button
          variant="ghost"
          size="icon-xs"
          className={cn(ROW_SUB_ACTION, 'size-6')}
          tooltip="Import repository"
          aria-label="Import repository"
          onClick={handleAddRepo}
        >
          {/* Arrow-into-folder, not folder-plus: this brings an existing repo in
              rather than creating an empty one. (Lucide's name says symlink;
              nothing about the mark does.) */}
          <FolderSymlink className="size-3" />
        </Button>
        <Button
          variant="ghost"
          size="icon-xs"
          className={cn(ROW_SUB_ACTION, 'size-6')}
          tooltip="Switch project"
          aria-label="Switch project"
          onClick={handleOpenSwitcher}
        >
          <LayoutGrid className="size-3" />
        </Button>
      </div>
      <AddRepositoryModal open={addRepoOpen} onOpenChange={setAddRepoOpen} />
    </>
  )
}
