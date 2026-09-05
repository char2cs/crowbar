import { lazy, Suspense, useCallback } from 'react'
import { nanoid } from 'nanoid'
import { Dialog, DialogHeader, DialogPopup, DialogTitle } from '@/components/ui/dialog'
import { uploadChatAttachment } from '@/features/agent/api/upload-chat-attachment'
import type { ExcalidrawSaveResult } from '@/features/agent/composer/excalidraw-canvas'
import { excalidrawMarkdown, imageMarkdown } from '@/features/agent/composer/lib/attachment-markdown'
import { toast } from '@/features/window/stores/toast-store'

const ExcalidrawCanvas = lazy(() =>
  import('@/features/agent/composer/excalidraw-canvas').then((m) => ({ default: m.ExcalidrawCanvas })),
)

interface ExcalidrawModalProps {
  wsId: string
  chatId: string
  open: boolean
  onClose: () => void
  onInsertMarkdown: (markdown: string) => void
}

/**
 * Create/edit UI only — the read-only preview renderer for a settled
 * `excalidraw:{id}` fence in the transcript is Phase 2's job
 * (`excalidraw-preview.tsx`).
 *
 * ONE nanoid drives both halves of the encoding, per the design spec's
 * resolved open question ("the client mints the id and passes it through"):
 * the fence-tag id for the inline JSON, and the shortid the upload endpoint
 * folds into the PNG's filename. Both `excalidrawMarkdown(id, ...)` and
 * `uploadChatAttachment(..., id)` below receive the SAME `id` — never let
 * the upload mint its own.
 */
export function ExcalidrawModal({ wsId, chatId, open, onClose, onInsertMarkdown }: ExcalidrawModalProps) {
  const handleSave = useCallback(
    async ({ sceneJson, pngFile }: ExcalidrawSaveResult) => {
      const id = nanoid()
      try {
        const result = await uploadChatAttachment(wsId, chatId, { file: pngFile }, id)
        onInsertMarkdown(excalidrawMarkdown(id, sceneJson))
        onInsertMarkdown(imageMarkdown('diagram', result.ref))
        onClose()
      } catch (err) {
        toast.error("Couldn't save this drawing", err instanceof Error ? err.message : String(err))
      }
    },
    [wsId, chatId, onInsertMarkdown, onClose],
  )

  return (
    <Dialog open={open} onOpenChange={(next) => !next && onClose()}>
      <DialogPopup className="flex h-[80vh] max-w-4xl flex-col">
        <DialogHeader>
          <DialogTitle>Excalidraw</DialogTitle>
        </DialogHeader>
        <Suspense fallback={null}>
          <ExcalidrawCanvas onCancel={onClose} onSave={handleSave} />
        </Suspense>
      </DialogPopup>
    </Dialog>
  )
}
