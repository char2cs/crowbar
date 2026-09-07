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
  /** Awaited by the caller (the takeover's header Attach button) before it
   *  re-enables — an async upload still in flight must not let a fast
   *  second click fire a second save. */
  onSave: (result: ExcalidrawSaveResult) => void | Promise<void>
  /** Resume an existing design instead of a blank canvas — the takeover's
   *  own "+" entry point (the user's saved design) and its Edit button on a
   *  rendered diagram (that diagram's scene) both preload through this. */
  initialScene?: ParsedExcalidrawScene
  /** Fires once the live Excalidraw API is available. Before this, `save()`
   *  silently no-ops — there is nothing to export yet, since the library's
   *  own async init hasn't handed back its imperative API. The takeover's
   *  Attach button stays disabled until this fires, so a click during that
   *  window can no longer look like a hang with nothing actually saved. */
  onReady?: () => void
}

export interface ExcalidrawCanvasHandle {
  /** Exports the live scene and hands it to `onSave`. There is no button
   *  inside this component — the takeover's header Attach button is the
   *  only caller (see excalidraw-takeover.tsx: the bottom toolbar this used
   *  to own was removed, reported live as chrome that didn't belong). */
  save: () => Promise<void>
}

/** The actual heavy mount — see excalidraw-takeover.tsx for why this lives
 *  in its own lazy chunk rather than the composer's own module scope. */
export const ExcalidrawCanvas = forwardRef<ExcalidrawCanvasHandle, ExcalidrawCanvasProps>(
  function ExcalidrawCanvas({ onSave, initialScene, onReady }, ref) {
    const [api, setApi] = useState<ExcalidrawImperativeAPI | null>(null)
    // Reactive, not just read-once-at-mount: opening the takeover in dark
    // mode and then flipping the app's theme while it's still open must not
    // leave it stuck on whichever theme was active at open time.
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
