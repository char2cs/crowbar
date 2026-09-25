import { useEffect, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { useShallow } from 'zustand/react/shallow'
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { hasTextContent } from '@/features/panes/types/pane-content'
import { mimeForPath, resolveAssetPath, toDataUrl } from '@/features/editor/lib/asset-data-url'
import { getDirName } from '@/utils/path-helpers'
import { buildHtmlPreviewDocument } from './html-preview-document'

export function HtmlPreview() {
  const { hasSourceBuffer, sourceContent, sourcePath, workspaceId } = useStore(
    windowPaneStore,
    useShallow((state) => {
      const activeBufferId = state.panes[state.activePaneId]?.activeEditorTabId ?? null
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
        workspaceId: sourceBuffer?.workspaceId,
      }
    }),
  )
  const [iframeContent, setIframeContent] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)

  // Local assets are read through the files API of the file's own workspace
  // and inlined; the latest edit wins over a slower earlier build.
  useEffect(() => {
    let cancelled = false
    const fileDir = sourcePath ? getDirName(sourcePath) : ''
    const loadAsset = async (reference: string) => {
      const path = resolveAssetPath(fileDir, reference)
      const mime = mimeForPath(path)
      if (!workspaceId || !mime) return null
      return toDataUrl(mime, await readWorkspaceFile(workspaceId, path))
    }
    void buildHtmlPreviewDocument(sourceContent, loadAsset).then((doc) => {
      if (!cancelled) setIframeContent(doc)
    })
    return () => {
      cancelled = true
    }
  }, [sourceContent, sourcePath, workspaceId])

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
