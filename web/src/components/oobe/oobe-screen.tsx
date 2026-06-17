import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import {
  Empty,
  EmptyMedia,
  EmptyHeader,
  EmptyTitle,
  EmptyDescription,
  EmptyContent,
} from '@/components/ui/empty'
import { CrowbarIcon } from '@/components/ui/crowbar-icon'
import { Button } from '@/components/ui/button'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { importProjectAndSync } from '@/lib/store/projects'
import type { Project } from '@/lib/types'

export function OobeScreen() {
  const [importOpen, setImportOpen] = useState(false)
  const navigate = useNavigate()

  function handleImport(project: Project) {
    importProjectAndSync(project)
    setImportOpen(false)
    void navigate({ to: '/' })
  }

  return (
    <div className="flex h-screen flex-col bg-background">
      <Empty>
        <EmptyMedia variant="icon">
          <CrowbarIcon className="text-foreground" />
        </EmptyMedia>
        <EmptyHeader>
          <EmptyTitle>Open a project folder</EmptyTitle>
          <EmptyDescription>Choose a local directory to get started.</EmptyDescription>
        </EmptyHeader>
        <EmptyContent>
          <Button className="w-full rounded-full" onClick={() => setImportOpen(true)}>
            Choose folder
          </Button>
        </EmptyContent>
      </Empty>

      <ImportProjectModal
        open={importOpen}
        onOpenChange={setImportOpen}
        onImport={handleImport}
      />
    </div>
  )
}
