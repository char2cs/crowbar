import { useState } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { FolderOpen } from '@phosphor-icons/react'
import { toast } from '@/features/window/stores/toast-store'
import { openNativeDialog as openDialog } from '@/lib/native-dialog'
import { useNavigate } from '@tanstack/react-router'
import { isTauri } from '@/lib/crowbar-bridge'
import { fetchWorkspaces, postRepo, workspaceDTOFromWorktreeFrame } from '@/lib/api'
import { useProjectStore } from '@/lib/store/projects'
import { awaitEntity } from '@/lib/ws/await-entity'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import { syncSidebarFromCache } from '@/lib/store/sidebar-sync'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

interface AddRepositoryModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  /**
   * Which project the repo lands in. Required wherever the caller is not the
   * active project: the sidebar now shows a row per project, and each of those
   * rows offers this modal — reading the active project here would silently
   * import into whichever project happened to be open instead of the one whose
   * row was clicked. Omitted, it keeps the old behaviour for the callers that
   * genuinely mean "the current project".
   */
  projectId?: string
}

export function AddRepositoryModal({ open, onOpenChange, projectId }: AddRepositoryModalProps) {
  const [path, setPath] = useState('')
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  const trimmedPath = path.trim()
  const pathLooksAbsolute = trimmedPath.startsWith('/') || /^[A-Za-z]:[\\/]/.test(trimmedPath)

  async function handleBrowse() {
    const selected = await openDialog({ directory: true, multiple: false })
    if (typeof selected === 'string') {
      setPath(selected)
      if (!name) {
        setName(
          selected
            .replace(/[\\/]+$/, '')
            .split(/[\\/]/)
            .pop() ?? '',
        )
      }
    }
  }

  async function handleAdd() {
    if (!pathLooksAbsolute) return
    setLoading(true)
    try {
      const activeProjectId = projectId || useProjectStore.getState().activeProjectId
      const fallbackName =
        trimmedPath
          .replace(/[\\/]+$/, '')
          .split(/[\\/]/)
          .pop() ?? trimmedPath
      const repoName = name.trim() || fallbackName

      // §3/§4 subscribe-before-POST: postRepo answers 202 with no body. The
      // daemon imports the repo AND auto-imports its default-branch worktree,
      // broadcasting the RepoDTO on the repos stream and the worktree on the
      // repo's CHAT stream (a worktree is held by a chat, so its live updates
      // ride that feed's `worktree_state` frames). We do NOT post the
      // default-branch workspace ourselves (that double-create would collide).
      // Subscribe to the repos stream first, fire the POST, resolve the new repo
      // by matching its path, then resolve its default WorkspaceDTO and navigate
      // into the IDE.
      const repo = await awaitEntity<RepoDTO>({
        endpoint: `/v0/projects/${activeProjectId}/repos`,
        match: (r) => r.path === trimmedPath,
        action: () => postRepo(activeProjectId, repoName, trimmedPath),
      })

      const ws = await awaitEntity<WorkspaceDTO>({
        endpoint: `/v0/projects/${activeProjectId}/repos/${repo.id}/chats/ws`,
        // The frames are chat lifecycle events, not workspace DTOs — pick the
        // owning row's worktree state out and build the DTO from it. Every
        // other kind maps to null and is skipped before `match` ever runs.
        mapFrame: (raw) => workspaceDTOFromWorktreeFrame(raw, activeProjectId, repo.id),
        match: (w) => w.repoId === repo.id && w.status !== 'deleted',
        action: () => Promise.resolve(),
        // The default worktree is created as a side effect of the repo POST in
        // the await ABOVE, so by the time this subscribes it usually already
        // exists — and the chat lifecycle feed, unlike the workspace stream this
        // replaced, replays NOTHING on connect. So the row is read once over
        // REST rather than waited for: without this the common path announces no
        // frame at all and the import hangs to the 30s timeout.
        //
        // The socket stays live alongside it for the other ordering, where the
        // worktree is still being provisioned when we get here and arrives as a
        // `worktree_state` frame. acceptExisting keeps that frame eligible: the
        // row is "pre-existing" from this await's point of view, and banking it
        // would strand the caller.
        seed: () => fetchWorkspaces(activeProjectId, repo.id),
        acceptExisting: true,
      })

      // Persist the resolved entities and rebuild the sidebar BEFORE navigating.
      // awaitEntity only resolves the DTOs; the entity cache and sidebar tree are
      // populated asynchronously by app-sync's streams, which lag this navigate.
      // Without seeding them here the /ide route guard reads a sidebar that does
      // not yet contain the new workspace, treats it as unknown, and bounces back
      // to / (a redirect loop with the landing beforeLoad).
      await upsertEntity('crowbar_repos', repo)
      await upsertEntity('crowbar_workspaces', ws)
      await syncSidebarFromCache()

      onOpenChange(false)
      setPath('')
      setName('')
      void navigate({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: activeProjectId, repoId: repo.id, wsId: ws.id },
      })
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to add repository')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Add repository</DialogTitle>
        </DialogHeader>

        <div className="space-y-5 px-6 py-5">
          <div className="space-y-2">
            <label htmlFor="add-repo-path" className="text-sm font-medium text-foreground">
              Repository folder
            </label>
            <div className="flex gap-2">
              <Input
                id="add-repo-path"
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="/absolute/path/to/repo"
                className="font-mono text-[12px]"
              />
              {isTauri() && (
                <Button variant="outline" size="icon" onClick={handleBrowse} title="Browse…">
                  <FolderOpen className="size-4" />
                </Button>
              )}
            </div>
            {trimmedPath !== '' && !pathLooksAbsolute && (
              <p className="text-[12px] text-destructive">
                Enter an absolute path (e.g. /Users/you/code/my-repo)
              </p>
            )}
          </div>

          <div className="space-y-2">
            <label htmlFor="add-repo-name" className="text-sm font-medium text-foreground">
              Repository name <span className="font-normal text-muted-foreground">(optional)</span>
            </label>
            <Input
              id="add-repo-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Derived from folder name"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleAdd} disabled={!pathLooksAbsolute || loading}>
            {loading ? 'Adding…' : 'Add repository'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
