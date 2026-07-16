import { useEffect, useRef, useState } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { hasTextContent } from '@/features/panes/types/pane-content'
import { buildHtmlPreviewDocument } from './html-preview-document'

export function HtmlPreview() {
  const { hasSourceBuffer, sourceContent, sourcePath } = useWorkspaceStoreContext(
    useShallow((state) => {
      const activeBufferId = state.panes[state.activePaneId]?.activeBufferId ?? null
      const activeBuffer = activeBufferId
        ? state.buffers.find((buffer) => buffer.id === activeBufferId)
        : null
      const sourceBuffer =
        activeBuffer?.type === 'htmlPreview'
          ? (state.buffers.find((buffer) => buffer.path === activeBuffer.sourceFilePath) ??
            activeBuffer)
          : activeBuffer

      return {
        hasSourceBuffer: Boolean(sourceBuffer),
        sourceContent: sourceBuffer && hasTextContent(sourceBuffer) ? sourceBuffer.content : '',
        sourcePath: sourceBuffer?.path,
      }
    }),
  )
  const rootFolderPath = useFileSystemStore.use.rootFolderPath?.()

  const [iframeContent, setIframeContent] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    setIframeContent(
      buildHtmlPreviewDocument(sourceContent, {
        sourcePath,
        rootFolderPath: rootFolderPath ?? undefined,
      }),
    )
  }, [sourceContent, sourcePath, rootFolderPath])

  if (!hasSourceBuffer) {
    return (
      <div className="flex h-full items-center justify-center text-muted-foreground">
        No active buffer
      </div>
    )
  }

  return (
    <div ref={containerRef} className="html-preview h-full w-full bg-white">
      <iframe
        title="HTML Preview"
        srcDoc={iframeContent}
        className="h-full w-full border-none"
        // No `allow-same-origin`: combined with `allow-scripts` it lets the framed
        // document (potentially agent-generated / untrusted HTML) reach the parent
        // Crowbar origin and remove its own sandbox. Dropping it gives the preview an
        // opaque origin — scripts/forms/popups/modals still run, but it can't touch
        // the host app's DOM, cookies, or storage.
        sandbox="allow-scripts allow-forms allow-popups allow-modals"
      />
    </div>
  )
}
