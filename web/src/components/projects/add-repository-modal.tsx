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
import { openNativeDialog as openDialog } from '@/lib/native-dialog'
import { useNavigate } from '@tanstack/react-router'
import { isTauri } from '@/lib/crowbar-bridge'
import { fetchRepos, fetchWorkspaces, postRepo } from '@/lib/api'
import { useProjectStore } from '@/core/entities/active-project'
import { placeEntities } from '@/core/entities/entity-store'
import { runOperation, whenSettled } from '@/core/entities/run-operation'
import { operationsState, type Operation } from '@/core/entities/operations'
import { report } from '@/lib/errors/report'
import { UserFacingError } from '@/lib/errors/user-facing-error'

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

// A failed branch fails the add as `partial`, but the repo was added: that add
// opens like a whole one, and the failed branch keeps its row and reason.
async function settledOrPartial(opId: string): Promise<Operation> {
  try {
    return await whenSettled(opId)
  } catch (err) {
    const op = operationsState().ops[opId]
    if (op && addedRepoItem(op)) return op
    throw err
  }
}

function addedRepoItem(op: Operation) {
  return op.items.find((item) => item.ref.kind === 'repo' && item.state === 'succeeded')
}

function addedRepoId(op: Operation): string {
  const item = addedRepoItem(op)
  if (!item) throw new UserFacingError(`${op.label}: no repository was added`)
  return item.ref.id
}

async function readRepoHome(projectId: string, repoId: string) {
  const [repos, workspaces] = await Promise.all([
    fetchRepos(projectId),
    fetchWorkspaces(projectId, repoId),
  ])
  const repo = repos.find((r) => r.id === repoId)
  if (!repo) throw new UserFacingError('The added repository is gone')
  const live = workspaces.filter((w) => w.status !== 'deleted')
  return { repo, home: live.find((w) => w.isDefault) ?? live[0] }
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

      const op = runOperation(
        {
          kind: 'repo.create',
          subject: { kind: 'project', id: activeProjectId },
          projectId: activeProjectId,
          label: `Add ${repoName}`,
        },
        (opId) => postRepo(activeProjectId, repoName, trimmedPath, opId),
      )
      const repoId = addedRepoId(await settledOrPartial(op.id))
      const { repo, home } = await readRepoHome(activeProjectId, repoId)

      // The /ide guard reads the entity store; the read's answer lands there before the
      // navigate, so it never bounces back to /.
      placeEntities('repo', { [repo.id]: repo })
      if (home) placeEntities('workspace', { [home.id]: home })

      onOpenChange(false)
      setPath('')
      setName('')
      if (home) {
        void navigate({
          to: '/ide/$projectId/$repoId/$wsId',
          params: { projectId: activeProjectId, repoId: repo.id, wsId: home.id },
        })
      }
    } catch (err) {
      report(err, { action: 'Failed to add repository' })
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
