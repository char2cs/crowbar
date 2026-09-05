import { useCallback, useState } from 'react'
import { Excalidraw, exportToBlob } from '@excalidraw/excalidraw'
import '@excalidraw/excalidraw/index.css'
import type { ExcalidrawImperativeAPI } from '@excalidraw/excalidraw/types'
import { Button } from '@/components/ui/button'

export interface ExcalidrawSaveResult {
  sceneJson: string
  pngFile: File
}

interface ExcalidrawCanvasProps {
  onCancel: () => void
  onSave: (result: ExcalidrawSaveResult) => void
}

/** The actual heavy mount — see excalidraw-modal.tsx for why this lives in
 *  its own lazy chunk rather than the composer's own module scope. */
export function ExcalidrawCanvas({ onCancel, onSave }: ExcalidrawCanvasProps) {
  const [api, setApi] = useState<ExcalidrawImperativeAPI | null>(null)
  const [saving, setSaving] = useState(false)

  const handleSave = useCallback(async () => {
    if (!api) return
    setSaving(true)
    try {
      const elements = api.getSceneElements()
      const appState = api.getAppState()
      const files = api.getFiles()
      const sceneJson = JSON.stringify({ type: 'excalidraw', version: 2, elements, files })
      const blob = await exportToBlob({ elements, appState, files, mimeType: 'image/png' })
      const pngFile = new File([blob], 'diagram.png', { type: 'image/png' })
      onSave({ sceneJson, pngFile })
    } finally {
      setSaving(false)
    }
  }, [api, onSave])

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="min-h-0 flex-1">
        <Excalidraw excalidrawAPI={setApi} />
      </div>
      <div className="flex justify-end gap-2 p-3">
        <Button variant="ghost" onClick={onCancel} disabled={saving}>
          Cancel
        </Button>
        <Button onClick={handleSave} disabled={saving || !api}>
          {saving ? 'Saving…' : 'Save'}
        </Button>
      </div>
    </div>
  )
}
