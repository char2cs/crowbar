// web/src/features/markdown-chat/components/markdown/blocks/excalidraw-block-impl.tsx
import { useCallback } from 'react'
import { Excalidraw } from '@excalidraw/excalidraw'
import type { BlockViewProps } from '@/features/markdown-chat/lib/block-registry'

// Excalidraw types — derived via Parameters<> to avoid importing internal types directly
type ExcalidrawOnChange = NonNullable<Parameters<typeof Excalidraw>[0]['onChange']>
type ExcalidrawElement = Parameters<ExcalidrawOnChange>[0][0]
type AppState = Parameters<ExcalidrawOnChange>[1]

interface ExcalidrawPayload {
  elements: readonly ExcalidrawElement[]
  appState: Partial<AppState>
}

export function ExcalidrawView(props: BlockViewProps) {
  const payload = (props.widget?.payload ?? null) as ExcalidrawPayload | null

  if (!payload) {
    return (
      <div className="relative h-80 w-full overflow-hidden rounded border border-border">
        <div className="flex h-full w-full items-center justify-center text-sm text-muted-foreground">
          Empty drawing
        </div>
      </div>
    )
  }

  return (
    <div className="relative h-80 w-full overflow-hidden rounded border border-border">
      <Excalidraw
        viewModeEnabled
        initialData={{ elements: payload.elements, appState: payload.appState }}
        UIOptions={{ canvasActions: { export: false, loadScene: false } }}
      />
    </div>
  )
}

export function ExcalidrawEditor(props: BlockViewProps) {
  const payload = (props.widget?.payload ?? null) as ExcalidrawPayload | null
  const { onChange } = props

  const handleChange = useCallback(
    (elements: readonly ExcalidrawElement[], appState: AppState) => {
      onChange?.({ elements, appState } as ExcalidrawPayload)
    },
    [onChange],
  )

  return (
    <div className="relative h-80 w-full overflow-hidden rounded border border-border">
      <Excalidraw
        initialData={
          payload ? { elements: payload.elements, appState: payload.appState } : undefined
        }
        onChange={handleChange}
        UIOptions={{ canvasActions: { export: false, loadScene: false } }}
      />
    </div>
  )
}
