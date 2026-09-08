'use client'

import {
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import DOMPurify from 'dompurify'
import { PencilIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { loadLocalImage, useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import { isDarkMode, useThemeVersion } from '@/features/editor/theme/use-theme-version'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { ATTACHMENT_BUTTON_OPAQUE_BG } from '@/features/agent/composer/plate/attachment-drag-handle'
import { useChatId } from './chat-id-context'
import { computeSceneAspectRatio, type ParsedExcalidrawScene } from './excalidraw-scene'

/** Matches `max-h-80` (Tailwind: 20rem, 320px) on both the placeholder's
 *  reserved footprint and the loaded content below — the same cap either
 *  side of the swap, so a very tall/thin scene never reserves more than the
 *  loaded render could ever actually use. */
const MAX_PREVIEW_HEIGHT_PX = 320

interface ExcalidrawPreviewProps {
  scene: ParsedExcalidrawScene
  /** The persisted PNG's ref — the sibling `![diagram](ref)` node's `url`,
   *  when there is one. Only ever present for a composer-drawn diagram
   *  (excalidraw-modal.tsx uploads it at save time); an agent-authored fence
   *  never has one, since there was no editor instance to export it from. */
  pngRef?: string
}

/** `@excalidraw/excalidraw`'s own `--theme-filter` (index.css): dark mode is
 *  a CSS filter applied to its live `<canvas>`, not a different set of
 *  exported colors — `exportToSvg`/`exportToBlob` always produce the
 *  scene's real (light) colors regardless of `appState.theme`, confirmed
 *  live. Applying the SAME filter here — to both the persisted PNG and a
 *  live SVG render — matches the editor's own look exactly, and unlike
 *  trying to get the export itself to respect a theme, it works for a PNG
 *  that was already saved under a different theme too. */
const EXCALIDRAW_DARK_FILTER = 'invert(93%) hue-rotate(180deg)'

/** Placeholder shown before a live render is ready, and if one fails. */
function ElementCountPlaceholder({ count }: { count: number }) {
  return (
    <div className="flex items-center gap-2 p-3 text-xs text-muted-foreground">
      Excalidraw diagram ({count} {count === 1 ? 'element' : 'elements'})
    </div>
  )
}

/**
 * A settled Excalidraw attachment's preview.
 *
 * Two sources, picked by which is available: a composer-drawn diagram has a
 * persisted PNG sibling and just shows that (cheap, no re-render needed). An
 * agent-authored fence has no such PNG — nothing ever ran the editor to
 * produce one — so its scene JSON is rendered live instead, via the same
 * `@excalidraw/excalidraw` engine the embedded drawing editor already
 * depends on (`exportToSvg`, dynamically imported so a chat with no
 * Excalidraw content never pays for that chunk). `restore()` runs first so a
 * scene missing internal bookkeeping fields (seed, versionNonce, and the
 * like — exactly what an LLM asked to write "valid Excalidraw JSON" tends to
 * omit) still renders instead of throwing.
 */
export function ExcalidrawPreview({ scene, pngRef }: ExcalidrawPreviewProps) {
  const asset = useMarkdownAsset()
  const [src, setSrc] = useState<string | null>(null)
  const [svgMarkup, setSvgMarkup] = useState<string | null>(null)
  // Reserves the loaded render's real footprint from the FIRST paint —
  // before `@excalidraw/excalidraw` has even started loading, let alone
  // exported anything — so nothing here changes height once it does. Read
  // only while nothing has loaded yet (see the `loaded` guard below): once
  // the real content is on screen it drives its own height, and holding
  // this reservation past that point would just be a second, now-wrong
  // guess fighting the real content's natural size.
  const aspectRatio = useMemo(() => computeSceneAspectRatio(scene.elements), [scene.elements])
  const containerRef = useRef<HTMLDivElement>(null)
  const [reservedHeight, setReservedHeight] = useState<number | null>(null)
  useLayoutEffect(() => {
    if (aspectRatio === null) return
    const width = containerRef.current?.clientWidth
    if (!width) return
    setReservedHeight(Math.min(MAX_PREVIEW_HEIGHT_PX, width * aspectRatio))
  }, [aspectRatio])
  // Reactive: the dark-mode filter below must follow the app's theme toggle,
  // not just whatever was active the first time this mounted.
  useThemeVersion()
  const darkFilter = isDarkMode() ? EXCALIDRAW_DARK_FILTER : undefined

  // Nullable, not `useWorkspaceStore()`'s throwing form: a preview can render
  // wherever a message can (a standalone unit-test host included), so absent
  // context just means no Edit button rather than a crash.
  const workspaceStore = useContext(WorkspaceStoreContext)
  const chatId = useChatId()
  const handleEdit = useCallback(() => {
    if (!workspaceStore || !chatId) return
    workspaceStore.getState().requestExcalidrawEdit(chatId, scene)
  }, [workspaceStore, chatId, scene])

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

  useEffect(() => {
    if (pngRef) return
    let cancelled = false
    setSvgMarkup(null)
    void import('@excalidraw/excalidraw')
      .then(({ restore, exportToSvg }) => {
        const restored = restore(
          { elements: scene.elements as never, appState: scene.appState, files: undefined },
          null,
          null,
        )
        return exportToSvg({
          elements: restored.elements,
          appState: restored.appState,
          files: restored.files,
        })
      })
      .then((svg) => {
        if (cancelled) return
        // A string handed to React's own `dangerouslySetInnerHTML`, not
        // `someRef.current.replaceChildren(svg)` (the previous approach):
        // that mutated a DOM node's children OUTSIDE React's rendering
        // entirely, which — for a node inside a Slate VOID element, as this
        // one always is — desynced slate-react's own DOM<->node mapping.
        // Confirmed live: the console logged "Cannot resolve a DOM node
        // from Slate node" right after inserting a diagram, and the
        // composer's own height-tracking (chat-markdown-editor.tsx's
        // ResizeObserver) silently stopped reporting past that point —
        // reported live as the input pill ballooning into a giant oval
        // instead of squaring off for a multi-line box. Setting markup
        // through state keeps this DOM subtree entirely inside React's own
        // reconciliation, the same as everywhere else in this codebase.
        // scene.elements/appState is attachment JSON an agent or another
        // chat participant fully controls, so exportToSvg's output is
        // untrusted — sanitize the same way as every other
        // dangerouslySetInnerHTML sink in this codebase.
        setSvgMarkup(
          DOMPurify.sanitize(svg.outerHTML, { USE_PROFILES: { svg: true, svgFilters: true } }),
        )
      })
      .catch(() => {
        // Malformed-enough scene JSON that even restore() can't repair it —
        // parseExcalidrawScene's own structural check already ruled out
        // "not a scene at all", so this is a genuinely broken one, not a
        // false positive. Falls back to the placeholder rather than an
        // empty box.
      })
    return () => {
      cancelled = true
    }
  }, [pngRef, scene])

  const count = scene.elements.length
  const loaded = pngRef ? src !== null : svgMarkup !== null

  return (
    <div
      ref={containerRef}
      className="excalidraw-preview relative"
      // Only while the real content hasn't landed yet — see the effect
      // above. A loaded render sizes this box itself; holding the
      // reservation past that point would fight it instead of matching it.
      style={!loaded && reservedHeight !== null ? { minHeight: reservedHeight } : undefined}
    >
      {workspaceStore && chatId && (
        <Button
          variant="outline"
          size="icon-xs"
          aria-label="Edit diagram"
          className={cn(ATTACHMENT_BUTTON_OPAQUE_BG, 'absolute top-2 right-2 z-10')}
          onClick={handleEdit}
        >
          <PencilIcon className="text-muted-foreground" />
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
          // react-doctor-disable-next-line dangerous-html-sink -- `svgMarkup` is DOMPurify.sanitize() output (l.126 above, USE_PROFILES svg) of exportToSvg's own render; sanitized before setSvgMarkup ever stores it.
          dangerouslySetInnerHTML={{ __html: svgMarkup }}
        />
      ) : (
        <ElementCountPlaceholder count={count} />
      )}
    </div>
  )
}
