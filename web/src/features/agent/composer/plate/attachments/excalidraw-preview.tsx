'use client'

import { useEffect, useState } from 'react'
import { loadLocalImage, useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import type { ParsedExcalidrawScene } from './excalidraw-scene'

interface ExcalidrawPreviewProps {
  scene: ParsedExcalidrawScene
  /** The persisted PNG's ref — the sibling `![diagram](ref)` node's `url`,
   *  when there is one. Absent falls back to a placeholder. */
  pngRef?: string
}

/** A settled Excalidraw attachment's preview: the persisted PNG, not a
 *  re-render of the scene JSON (no Excalidraw library dependency — the
 *  embedded drawing editor that produces the JSON+PNG is a separate task).
 *  Resolves through the same `MarkdownAssetContext`/`loadLocalImage`
 *  `MarkdownImageElement` uses — same contract, no new fetch path. */
export function ExcalidrawPreview({ scene, pngRef }: ExcalidrawPreviewProps) {
  const asset = useMarkdownAsset()
  const [src, setSrc] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    if (!asset || !pngRef) {
      setSrc(null)
      return
    }
    void loadLocalImage(asset, pngRef).then((data) => {
      if (!cancelled) setSrc(data)
    })
    return () => {
      cancelled = true
    }
  }, [asset, pngRef])

  const count = scene.elements.length

  return (
    <div className="excalidraw-preview rounded-md border border-border bg-background/60 p-2">
      {src ? (
        <img src={src} alt="Excalidraw diagram" className="max-w-full rounded" />
      ) : (
        <div className="flex items-center gap-2 p-3 text-xs text-muted-foreground">
          Excalidraw diagram ({count} {count === 1 ? 'element' : 'elements'})
        </div>
      )}
    </div>
  )
}
