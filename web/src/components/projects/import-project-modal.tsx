// web/src/components/projects/ImportProjectModal.tsx
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
import { isTauri } from '@/lib/crowbar-bridge'
import { postProject } from '@/lib/api'
import { awaitEntity } from '@/lib/ws/await-entity'
import { projectFromDTO } from '@/lib/store/project-from-dto'
import type { Project, ProjectDTO } from '@/lib/types'

interface ImportProjectModalProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onImport: (project: Project) => void
}

// The backend imports a project from an absolute path on its own filesystem,
// so the dialog takes the path as text. A browser folder picker cannot supply
// one (webkitdirectory yields only the folder *name*), which used to send
// garbage paths like "my-repo" to the import endpoint.
export function ImportProjectModal({ open, onOpenChange, onImport }: ImportProjectModalProps) {
  const [selectedPath, setSelectedPath] = useState('')
  const [projectName, setProjectName] = useState('')
  const [loading, setLoading] = useState(false)

  const trimmedPath = selectedPath.trim()
  const pathLooksAbsolute = trimmedPath.startsWith('/') || /^[A-Za-z]:[\\/]/.test(trimmedPath)

  async function handleBrowse() {
    const selected = await openDialog({ directory: true, multiple: false })
    if (typeof selected === 'string') setSelectedPath(selected)
  }

  const handleImport = async () => {
    if (!pathLooksAbsolute) return
    setLoading(true)
    try {
      // §4/§6 subscribe-before-POST: postProject answers 202 with no body — the
      // daemon assigns the id and only the canonical ProjectDTO carries it, on
      // the `/v0/projects` WS stream. We subscribe FIRST, fire the POST, then
      // resolve the real project by matching the submitted path (the one field
      // we can correlate before the id exists). No client-fabricated uuid.
      const fallbackName =
        trimmedPath
          .replace(/[\\/]+$/, '')
          .split(/[\\/]/)
          .pop() ?? trimmedPath
      const name = projectName.trim() || fallbackName
      const dto = await awaitEntity<ProjectDTO>({
        endpoint: '/v0/projects',
        match: (p) => p.path === trimmedPath && p.status !== 'deleted',
        action: () => postProject(name, trimmedPath),
      })
      onImport(projectFromDTO(dto))
      setSelectedPath('')
      setProjectName('')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to import project')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Import project</DialogTitle>
        </DialogHeader>

        <div className="space-y-5 px-6 py-5">
          <div className="space-y-2">
            <label htmlFor="import-project-path" className="text-sm font-medium text-foreground">
              Project folder
            </label>
            <div className="flex gap-2">
              <Input
                id="import-project-path"
                value={selectedPath}
                onChange={(e) => setSelectedPath(e.target.value)}
                placeholder="/absolute/path/to/project"
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
            <label htmlFor="import-project-name" className="text-sm font-medium text-foreground">
              Project name
            </label>
            <Input
              id="import-project-name"
              value={projectName}
              onChange={(e) => setProjectName(e.target.value)}
              placeholder="My project"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={handleImport} disabled={!pathLooksAbsolute || loading}>
            {loading ? 'Importing…' : 'Import'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
