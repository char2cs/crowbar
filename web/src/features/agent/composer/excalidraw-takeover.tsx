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
  /** Resume this scene instead of a blank canvas — the chat's own saved
   *  design (the "+" entry point) or a diagram's Edit button, whichever
   *  opened the takeover. */
  initialScene?: ParsedExcalidrawScene
}

/**
 * Full-pane takeover: replaces the chat view for a quick, focused edit
 * rather than floating a dialog over a transcript the user can still half-see
 * (and can't act on) behind it. Renders `absolute inset-0` over `.agent-chat
 * .chat`'s own containing block — the same one `.dock` already positions
 * against — so it covers exactly this chat's pane, not the whole window (a
 * split editor pane beside it stays untouched) and not just a centered box.
 *
 * Header-only chrome: Close and Attach both live in the top bar, not a
 * bottom toolbar (removed — reported live as space the canvas should have
 * had). Escape does nothing here, deliberately: Excalidraw uses it itself
 * (deselecting a shape, leaving text-edit mode), and a global listener that
 * also closed the takeover fought the tool for the same key.
 *
 * Create/edit UI only — the read-only preview renderer for a settled
 * `excalidraw:{id}` fence in the transcript is `excalidraw-preview.tsx`.
 *
 * ONE nanoid drives both halves of the encoding, per the design spec's
 * resolved open question ("the client mints the id and passes it through"):
 * the fence-tag id for the inline JSON, and the shortid the upload endpoint
 * folds into the PNG's filename. Both `excalidrawMarkdown(id, ...)` and
 * `uploadChatAttachment(..., id)` below receive the SAME `id` — never let
 * the upload mint its own.
 *
 * The scene JSON only gets the inline `excalidraw:{id}` fence treatment
 * under `INLINE_ATTACHMENT_MAX_BYTES` (inline-attachment-cap.ts, shared with
 * the paste plugin's text-attachment threshold) — a scene big enough to trip
 * `MAX_PROMPT_TEXT_BYTES` on its own uploads as a `.excalidraw.json` file
 * link instead. The PNG preview always uploads and inserts regardless: it is
 * the human-visible artifact, and losing it just because the SCENE data was
 * too big to inline would throw away the one thing a reader can actually see.
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

  // Guards the same double-click race `ExcalidrawCanvas`'s own Save button
  // used to guard internally — the button now lives here instead, so the
  // in-flight flag does too.
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
