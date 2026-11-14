import { forwardRef, useCallback, useImperativeHandle, useState } from 'react'
import { Excalidraw, exportToBlob } from '@excalidraw/excalidraw'
import '@excalidraw/excalidraw/index.css'
import type { ExcalidrawImperativeAPI } from '@excalidraw/excalidraw/types'
import { isDarkMode, useThemeVersion } from '@/features/editor/theme/use-theme-version'
import type { ParsedExcalidrawScene } from './plate/attachments/excalidraw-scene'

export interface ExcalidrawSaveResult {
  sceneJson: string
  pngFile: File
}

interface ExcalidrawCanvasProps {
  /** Awaited before Attach re-enables, so a second click can't fire a second save. */
  onSave: (result: ExcalidrawSaveResult) => void | Promise<void>
  initialScene?: ParsedExcalidrawScene
  /** Until this fires `save()` no-ops: Excalidraw's async init has no API yet. */
  onReady?: () => void
}

export interface ExcalidrawCanvasHandle {
  save: () => Promise<void>
}

export const ExcalidrawCanvas = forwardRef<ExcalidrawCanvasHandle, ExcalidrawCanvasProps>(
  function ExcalidrawCanvas({ onSave, initialScene, onReady }, ref) {
    const [api, setApi] = useState<ExcalidrawImperativeAPI | null>(null)
    // Reactive so a theme flip while the takeover is open is picked up.
    useThemeVersion()
    const theme = isDarkMode() ? 'dark' : 'light'

    const save = useCallback(async () => {
      if (!api) return
      const elements = api.getSceneElements()
      const appState = api.getAppState()
      const files = api.getFiles()
      const sceneJson = JSON.stringify({ type: 'excalidraw', version: 2, elements, files })
      const blob = await exportToBlob({ elements, appState, files, mimeType: 'image/png' })
      const pngFile = new File([blob], 'diagram.png', { type: 'image/png' })
      await onSave({ sceneJson, pngFile })
    }, [api, onSave])

    useImperativeHandle(ref, () => ({ save }), [save])

    return (
      <div className="h-full min-h-0">
        <Excalidraw
          excalidrawAPI={(instance) => {
            setApi(instance)
            onReady?.()
          }}
          theme={theme}
          initialData={
            initialScene
              ? { elements: initialScene.elements as never, appState: initialScene.appState }
              : undefined
          }
        />
      </div>
    )
  },
)
