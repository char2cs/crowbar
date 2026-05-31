// web/src/features/markdown-chat/components/excalidraw-widget.tsx
import { useCallback } from 'react'
import { Excalidraw } from '@excalidraw/excalidraw'
import { registerWidget } from '../extensions/widget-registry'
import type { WidgetComponentProps } from '../extensions/widget-registry'

// Excalidraw types — derived via Parameters<> to avoid importing internal types directly
type ExcalidrawOnChange = NonNullable<Parameters<typeof Excalidraw>[0]['onChange']>
type ExcalidrawElement = Parameters<ExcalidrawOnChange>[0][0]
type AppState = Parameters<ExcalidrawOnChange>[1]

interface ExcalidrawPayload {
  elements: readonly ExcalidrawElement[]
  appState: Partial<AppState>
}

export function ExcalidrawWidget({ data, onChange }: WidgetComponentProps) {
  const payload = data.payload as ExcalidrawPayload | null

  const handleChange = useCallback(
    (elements: readonly ExcalidrawElement[], appState: AppState) => {
      onChange({ elements, appState } as ExcalidrawPayload)
    },
    [onChange],
  )

  return (
    <div className="relative h-80 w-full overflow-hidden rounded border border-border">
      <Excalidraw
        initialData={
          payload
            ? { elements: payload.elements, appState: payload.appState }
            : undefined
        }
        onChange={handleChange}
        UIOptions={{ canvasActions: { export: false, loadScene: false } }}
      />
    </div>
  )
}

// Self-register into the widget registry on import
registerWidget('excalidraw', ExcalidrawWidget)
