import { Warning } from '@phosphor-icons/react'
import { useCallback, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { AppDialog as Dialog } from '@/components/ui/dialog'
import { findDocumentBuffer } from '@/features/editor/documents/open-document'
import { buildEnvTemplateContent } from '@/features/file-explorer/lib/env-template'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { readDocument, workspaceRef, writeDocument, type DocumentRef } from '@/lib/api/documents'
import { problemOf } from '@/lib/errors/problem'
import { report } from '@/lib/errors/report'
import { getDirName, joinPath } from '@/utils/path-helpers'

interface UseEnvTemplateFileOptions {
  workspaceId: string | null
  onCreateNewFileInDirectory?: (
    directoryPath: string,
    fileName: string,
  ) => void | string | Promise<string | undefined>
  onRefreshDirectory?: (path: string) => void
  onCreated: (path: string) => void
}

interface EnvOverwriteDialogState {
  sourcePath: string
  targetFileName: string
}

async function documentExists(ref: DocumentRef): Promise<boolean> {
  try {
    await readDocument(ref)
    return true
  } catch (err) {
    if (problemOf(err).code === 'not_found') return false
    throw err
  }
}

/** Creates a sibling env template from an env file, asking before it overwrites one. */
export function useEnvTemplateFile({
  workspaceId,
  onCreateNewFileInDirectory,
  onRefreshDirectory,
  onCreated,
}: UseEnvTemplateFileOptions): {
  createEnvTemplateFile: (sourcePath: string, targetFileName: string) => Promise<void>
  envOverwriteDialog: ReactNode
} {
  const [pending, setPending] = useState<EnvOverwriteDialogState | null>(null)

  const create = useCallback(
    async (sourcePath: string, targetFileName: string, overwrite = false) => {
      if (!onCreateNewFileInDirectory || !workspaceId) return
      const directoryPath = getDirName(sourcePath)
      const targetPath = joinPath(directoryPath, targetFileName)
      if (targetPath === sourcePath) return

      try {
        const targetExists = await documentExists(workspaceRef(workspaceId, targetPath))
        if (targetExists && !overwrite) {
          setPending({ sourcePath, targetFileName })
          return
        }
        const { text: sourceContent } = await readDocument(workspaceRef(workspaceId, sourcePath))
        const templateContent = buildEnvTemplateContent(sourceContent)
        const createdPath = targetExists
          ? targetPath
          : (await Promise.resolve(onCreateNewFileInDirectory(directoryPath, targetFileName))) ||
            targetPath
        await writeDocument(workspaceRef(workspaceId, createdPath), templateContent)

        // Buffers are window-level: a sibling worktree's same relative path is another document.
        const createdBuffer = findDocumentBuffer(workspaceRef(workspaceId, createdPath))
        if (createdBuffer) {
          windowPaneStore.setState((state) => ({
            ...state,
            buffers: state.buffers.map((b) =>
              b.id === createdBuffer.id && 'content' in b ? { ...b, content: templateContent } : b,
            ),
          }))
        }
        onRefreshDirectory?.(directoryPath)
        onCreated(createdPath)
      } catch (err) {
        report(err, { action: `Create ${targetFileName}` })
      }
    },
    [onCreateNewFileInDirectory, onRefreshDirectory, onCreated, workspaceId],
  )

  const confirmOverwrite = () => {
    if (!pending) return
    setPending(null)
    void create(pending.sourcePath, pending.targetFileName, true)
  }

  const envOverwriteDialog = pending ? (
    <Dialog
      title="Overwrite Env File"
      icon={Warning}
      onClose={() => setPending(null)}
      size="sm"
      footer={
        <>
          <Button variant="ghost" onClick={() => setPending(null)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={confirmOverwrite} compact>
            Overwrite
          </Button>
        </>
      }
    >
      <p className="ui-font ui-text-sm text-foreground">
        {pending.targetFileName} already exists. Overwrite it?
      </p>
    </Dialog>
  ) : null

  const createEnvTemplateFile = useCallback(
    (sourcePath: string, targetFileName: string) => create(sourcePath, targetFileName),
    [create],
  )
  return { createEnvTemplateFile, envOverwriteDialog }
}
