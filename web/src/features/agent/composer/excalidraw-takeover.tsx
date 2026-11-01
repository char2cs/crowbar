import { lazy, Suspense, useCallback, useRef, useState } from 'react'
import { nanoid } from 'nanoid'
import { Button } from '@/components/ui/button'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import type {
  ExcalidrawCanvasHandle,
  ExcalidrawSaveResult,
} from '@/features/agent/composer/excalidraw-canvas'
import {
  excalidrawMarkdown,
  imageMarkdown,
} from '@/features/agent/composer/lib/attachment-markdown'
import { saveExcalidrawDesign } from '@/features/agent/composer/lib/excalidraw-design-persistence'
import { uploadAttachmentMarkdown } from '@/features/agent/composer/lib/attachment-upload'
import { exceedsInlineSizeCap } from '@/features/agent/composer/lib/inline-attachment-cap'
import type { ParsedExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'
import { CloseIcon } from '@/features/agent/shared/agent-icons'
import { toast } from '@/features/window/stores/toast-store'

const ExcalidrawCanvas = lazy(() =>
  import('@/features/agent/composer/excalidraw-canvas').then((m) => ({
    default: m.ExcalidrawCanvas,
  })),
)

interface ExcalidrawTakeoverProps {
  wsId: string
  chatId: string
  open: boolean
  onClose: () => void
  onInsertMarkdown: (markdown: string) => void
  initialScene?: ParsedExcalidrawScene
}

/**
 * Covers only this chat's pane. Escape is left to Excalidraw, which uses it itself.
 * One id serves both the fence tag and the PNG's upload, so never let the upload
 * mint its own; an oversized scene uploads as a file, but the PNG always inserts.
 */
export function ExcalidrawTakeover({
  wsId,
  chatId,
  open,
  onClose,
  onInsertMarkdown,
  initialScene,
}: ExcalidrawTakeoverProps) {
  const canvasRef = useRef<ExcalidrawCanvasHandle>(null)
  const [attaching, setAttaching] = useState(false)
  const [canvasReady, setCanvasReady] = useState(false)

  const handleSave = useCallback(
    async ({ sceneJson, pngFile }: ExcalidrawSaveResult) => {
      const id = nanoid()
      try {
        const result = await uploadChatAttachment(wsId, chatId, { file: pngFile }, id)
        if (exceedsInlineSizeCap(sceneJson)) {
          const sceneFile = new File([sceneJson], 'diagram.excalidraw.json', {
            type: 'application/json',
          })
          onInsertMarkdown(await uploadAttachmentMarkdown(wsId, chatId, { file: sceneFile }))
        } else {
          onInsertMarkdown(excalidrawMarkdown(id, sceneJson))
        }
        onInsertMarkdown(imageMarkdown('diagram', result.ref))
        saveExcalidrawDesign(wsId, chatId, sceneJson)
        onClose()
      } catch (err) {
        toast.error("Couldn't save this drawing", err instanceof Error ? err.message : String(err))
      }
    },
    [wsId, chatId, onInsertMarkdown, onClose],
  )

  const handleAttach = useCallback(async () => {
    if (attaching || !canvasReady) return
    setAttaching(true)
    try {
      await canvasRef.current?.save()
    } finally {
      setAttaching(false)
    }
  }, [attaching, canvasReady])

  if (!open) return null

  return (
    <div
      data-testid="excalidraw-takeover"
      className="absolute inset-0 z-40 flex flex-col bg-background"
    >
      <div className="flex items-center justify-between border-b border-border px-4 py-2">
        <h2 className="text-sm font-medium">Excalidraw</h2>
        <div className="flex items-center gap-2">
          <Button onClick={handleAttach} disabled={attaching || !canvasReady}>
            {attaching ? 'Attaching…' : 'Attach'}
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label="Close"
            onClick={onClose}
            disabled={attaching}
          >
            <CloseIcon />
          </Button>
        </div>
      </div>
      <div className="min-h-0 flex-1">
        <Suspense fallback={null}>
          <ExcalidrawCanvas
            ref={canvasRef}
            onSave={handleSave}
            initialScene={initialScene}
            onReady={() => setCanvasReady(true)}
          />
        </Suspense>
      </div>
    </div>
  )
}
