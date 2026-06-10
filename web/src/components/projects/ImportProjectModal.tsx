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
import { toast } from 'sonner'
import { postProject, fetchProject } from '@/lib/api'
import type { Project } from '@/lib/types'

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

  const handleImport = async () => {
    if (!pathLooksAbsolute) return
    setLoading(true)
    try {
      // The mutation returns only { id }; re-fetch the full project so the
      // sidebar gets a complete entity (name/path) rather than undefined fields.
      const fallbackName =
        trimmedPath
          .replace(/[\\/]+$/, '')
          .split(/[\\/]/)
          .pop() ?? trimmedPath
      const { id } = await postProject(projectName.trim() || fallbackName, trimmedPath)
      const project = await fetchProject(id)
      onImport(project)
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

        <div className="space-y-4 py-2">
          <div className="space-y-1.5">
            <label className="text-sm font-medium text-foreground">Project folder</label>
            <Input
              value={selectedPath}
              onChange={(e) => setSelectedPath(e.target.value)}
              placeholder="/absolute/path/to/project"
              className="font-mono text-[12px]"
            />
            {trimmedPath !== '' && !pathLooksAbsolute && (
              <p className="text-[12px] text-destructive">
                Enter an absolute path (e.g. /Users/you/code/my-repo)
              </p>
            )}
          </div>

          <div className="space-y-1.5">
            <label className="text-sm font-medium text-foreground">Project name</label>
            <Input
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
