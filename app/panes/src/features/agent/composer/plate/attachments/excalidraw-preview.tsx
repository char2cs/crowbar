'use client'

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import DOMPurify from 'dompurify'
import { PencilSimpleIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { loadLocalImage, useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import { isDarkMode, useThemeVersion } from '@/features/editor/theme/use-theme-version'
import { requestExcalidrawEdit } from '@/features/agent/composer/lib/excalidraw-edit-requests'
import { ATTACHMENT_BUTTON_OPAQUE_BG } from '@/features/agent/composer/plate/attachment-drag-handle'
import { useChatId } from './chat-id-context'
import { computeSceneAspectRatio, type ParsedExcalidrawScene } from './excalidraw-scene'

const MAX_PREVIEW_HEIGHT_PX = 320

interface ExcalidrawPreviewProps {
  scene: ParsedExcalidrawScene
  pngRef?: string
}

const EXCALIDRAW_DARK_FILTER = 'invert(93%) hue-rotate(180deg)'

function ElementCountPlaceholder({ count }: { count: number }) {
  return (
    <div className="flex items-center gap-2 p-3 text-xs text-muted-foreground">
      Excalidraw diagram ({count} {count === 1 ? 'element' : 'elements'})
    </div>
  )
}

export function ExcalidrawPreview({ scene, pngRef }: ExcalidrawPreviewProps) {
  const asset = useMarkdownAsset()
  const [src, setSrc] = useState<string | null>(null)
  const [svgMarkup, setSvgMarkup] = useState<string | null>(null)
  const loaded = pngRef ? src !== null : svgMarkup !== null
  const aspectRatio = useMemo(() => computeSceneAspectRatio(scene.elements), [scene.elements])
  const containerRef = useRef<HTMLDivElement>(null)
  const [reservedHeight, setReservedHeight] = useState<number | null>(null)
  useLayoutEffect(() => {
    if (aspectRatio === null || loaded) return
    const node = containerRef.current
    if (!node) return
    const measure = () => {
      const width = node.clientWidth
      if (width) setReservedHeight(Math.min(MAX_PREVIEW_HEIGHT_PX, width * aspectRatio))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(node)
    return () => observer.disconnect()
  }, [aspectRatio, loaded])
  useThemeVersion()
  const darkFilter = isDarkMode() ? EXCALIDRAW_DARK_FILTER : undefined

  const chatId = useChatId()
  const handleEdit = useCallback(() => {
    if (chatId) requestExcalidrawEdit(chatId, scene)
  }, [chatId, scene])

  useEffect(() => {
    if (!asset || !pngRef) {
      setSrc(null)
      return
    }
    const controller = new AbortController()
    void loadLocalImage(asset, pngRef, controller.signal).then((data) => {
      if (!controller.signal.aborted) setSrc(data)
    })
    return () => controller.abort()
  }, [asset, pngRef])

  useEffect(() => {
    if (pngRef) return
    let cancelled = false
    setSvgMarkup(null)
    const draw = async () => {
      try {
        const { restore, exportToSvg } = await import('@excalidraw/excalidraw')
        const restored = restore(
          { elements: scene.elements as never, appState: scene.appState, files: undefined },
          null,
          null,
        )
        const svg = await exportToSvg({
          elements: restored.elements,
          appState: restored.appState,
          files: restored.files,
        })
        if (cancelled) return
        setSvgMarkup(
          DOMPurify.sanitize(svg.outerHTML, { USE_PROFILES: { svg: true, svgFilters: true } }),
        )
      } catch {
        if (!cancelled) setSvgMarkup(null)
      }
    }
    void draw()
    return () => {
      cancelled = true
    }
  }, [pngRef, scene])

  const count = scene.elements.length

  return (
    <div
      ref={containerRef}
      className="excalidraw-preview relative"
      style={!loaded && reservedHeight !== null ? { minHeight: reservedHeight } : undefined}
    >
      {chatId && (
        <Button
          variant="outline"
          size="icon-xs"
          aria-label="Edit diagram"
          className={cn(ATTACHMENT_BUTTON_OPAQUE_BG, 'absolute top-2 right-2 z-10')}
          onClick={handleEdit}
        >
          <PencilSimpleIcon className="text-muted-foreground" />
        </Button>
      )}
      {pngRef ? (
        src ? (
          <img
            src={src}
            alt="Excalidraw diagram"
            className="max-h-80 max-w-full rounded object-contain"
            style={{ filter: darkFilter }}
          />
        ) : (
          <ElementCountPlaceholder count={count} />
        )
      ) : svgMarkup ? (
        <div
          className="max-h-80 [&>svg]:h-auto [&>svg]:max-h-80 [&>svg]:w-full"
          style={{ filter: darkFilter }}
          // react-doctor-disable-next-line dangerous-html-sink -- `svgMarkup` is DOMPurify.sanitize() output (USE_PROFILES svg) of exportToSvg's own render, sanitized before setSvgMarkup ever stores it.
          dangerouslySetInnerHTML={{ __html: svgMarkup }}
        />
      ) : (
        <ElementCountPlaceholder count={count} />
      )}
    </div>
  )
}
