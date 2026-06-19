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
import { toast } from 'sonner'
import { open as openDialog } from '@tauri-apps/plugin-dialog'
import { isTauri } from '@/lib/crowbar-bridge'
import { postRepo, postWorkspace } from '@/lib/api'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useSidebarStore } from '@/lib/store/sidebar'
import { dataOf } from '@/lib/loadable'
import { useNavigate } from '@tanstack/react-router'
import { useProjectStore } from '@/lib/store/projects'

interface AddRepositoryModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function AddRepositoryModal({ open, onOpenChange }: AddRepositoryModalProps) {
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
        setName(selected.replace(/[\\/]+$/, '').split(/[\\/]/).pop() ?? '')
      }
    }
  }

  async function handleAdd() {
    if (!pathLooksAbsolute) return
    setLoading(true)
    try {
      const activeProjectId = useProjectStore.getState().activeProjectId
      const fallbackName =
        trimmedPath
          .replace(/[\\/]+$/, '')
          .split(/[\\/]/)
          .pop() ?? trimmedPath
      const repoName = name.trim() || fallbackName

      // 1. Create the repo record (response includes defaultBranch resolved by daemon)
      const repo = await postRepo(activeProjectId, repoName, trimmedPath)
      const repoId = repo.id
      const branch = repo.defaultBranch || 'main'

      // 2. Create the first workspace on the default branch
      const { id: wsId } = await postWorkspace(repoId, branch)

      // 4. Refresh sidebar
      await useWorkspaceListStore.getState().fetch()
      const fresh = dataOf(useWorkspaceListStore.getState().data)
      if (fresh) useSidebarStore.getState().mergeRepos(fresh)

      onOpenChange(false)
      setPath('')
      setName('')

      void navigate({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: activeProjectId, repoId, wsId },
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
            <label className="text-sm font-medium text-foreground">Repository folder</label>
            <div className="flex gap-2">
              <Input
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
            <label className="text-sm font-medium text-foreground">
              Repository name{' '}
              <span className="font-normal text-muted-foreground">(optional)</span>
            </label>
            <Input
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
