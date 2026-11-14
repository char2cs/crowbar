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

// `max-h-80` (320px), so a tall scene never reserves more than the loaded render can use.
const MAX_PREVIEW_HEIGHT_PX = 320

interface ExcalidrawPreviewProps {
  scene: ParsedExcalidrawScene
  /** The PNG uploaded when the composer's editor saved the diagram; an agent-authored fence
   *  has none, since no editor ever exported it. */
  pngRef?: string
}

// Excalidraw's own `--theme-filter`: its dark mode is a CSS filter, and exports are always light.
// The same filter on the PNG and the SVG matches the editor, even for a PNG saved in another theme.
const EXCALIDRAW_DARK_FILTER = 'invert(93%) hue-rotate(180deg)'

/** Placeholder shown before a live render is ready, and if one fails. */
function ElementCountPlaceholder({ count }: { count: number }) {
  return (
    <div className="flex items-center gap-2 p-3 text-xs text-muted-foreground">
      Excalidraw diagram ({count} {count === 1 ? 'element' : 'elements'})
    </div>
  )
}

// Shows the saved PNG when there is one, else renders the scene live with a lazily imported
// `exportToSvg`. `restore()` first fills the bookkeeping fields (seed, versionNonce) that
// agent-written scenes tend to omit, so they render instead of throwing.
export function ExcalidrawPreview({ scene, pngRef }: ExcalidrawPreviewProps) {
  const asset = useMarkdownAsset()
  const [src, setSrc] = useState<string | null>(null)
  const [svgMarkup, setSvgMarkup] = useState<string | null>(null)
  const loaded = pngRef ? src !== null : svgMarkup !== null
  // Reserves the render's footprint before it loads, so nothing changes height when it lands.
  // Only until loaded: past that the content sizes itself and the guess would fight it.
  const aspectRatio = useMemo(() => computeSceneAspectRatio(scene.elements), [scene.elements])
  const containerRef = useRef<HTMLDivElement>(null)
  const [reservedHeight, setReservedHeight] = useState<number | null>(null)
  useLayoutEffect(() => {
    if (aspectRatio === null || loaded) return
    const node = containerRef.current
    if (!node) return
    // Observed, not read once: the width can change before the render resolves, and a surface
    // mounted `display:none` reads 0 until it is shown.
    const measure = () => {
      const width = node.clientWidth
      if (width) setReservedHeight(Math.min(MAX_PREVIEW_HEIGHT_PX, width * aspectRatio))
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(node)
    return () => observer.disconnect()
  }, [aspectRatio, loaded])
  // Reactive: the dark-mode filter below must follow the app's theme toggle,
  // not just whatever was active the first time this mounted.
  useThemeVersion()
  const darkFilter = isDarkMode() ? EXCALIDRAW_DARK_FILTER : undefined

  // Outside a chat (no chat id) there is no composer to edit in: no Edit button.
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
        // Markup through React state, never a direct DOM mutation, which desyncs slate-react's
        // DOM mapping inside a void element. The scene is agent-controlled JSON: sanitize it.
        setSvgMarkup(
          DOMPurify.sanitize(svg.outerHTML, { USE_PROFILES: { svg: true, svgFilters: true } }),
        )
      } catch {
        // A scene restore() cannot repair draws the placeholder: a fallback, not an error.
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
      // Only until the content lands; a loaded render sizes this box itself.
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
