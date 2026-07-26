import './styles.css'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useShallow } from 'zustand/react/shallow'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { usePreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import { useEditorSettingsStore } from '@/features/editor/stores/settings-store'
import { exists } from '@/features/file-system/controllers/platform'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { hasTextContent } from '@/features/panes/types/pane-content'
import { openExternalUrl } from '@/lib/external-open'
import { useSettingsStore } from '@/features/settings/store'
import { logger } from '../utils/logger'
import { parseMarkdown } from './parser'
import { resolvePreviewLinkPath } from './resolve-preview-link'

export interface MarkdownPreviewProps {
  /**
   * The `markdownPreview` buffer this instance renders. Also the key its scroll
   * offset is retained under, since PaneContainer unmounts the preview whenever
   * another tab is active in the pane. Omitted by the legacy CodeEditor host,
   * which falls back to the active pane's buffer and keeps no scroll.
   */
  bufferId?: string
}

export function MarkdownPreview({ bufferId }: MarkdownPreviewProps) {
  const { sourceBufferPath, sourceContent } = useWorkspaceStoreContext(
    useShallow((state) => {
      // Prefer the buffer this instance was handed: with a split, the pane
      // rendering the preview is not necessarily the ACTIVE pane, and reading
      // the active pane's buffer would show another pane's file.
      const ownBuffer = bufferId ? state.buffers.find((buffer) => buffer.id === bufferId) : null
      const activeBufferId = state.panes[state.activePaneId]?.activeBufferId ?? null
      const activeBuffer =
        ownBuffer ??
        (activeBufferId ? state.buffers.find((buffer) => buffer.id === activeBufferId) : null)
      const sourceBuffer =
        activeBuffer?.type === 'markdownPreview'
          ? (state.buffers.find((buffer) => buffer.path === activeBuffer.sourceFilePath) ??
            activeBuffer)
          : activeBuffer

      return {
        sourceBufferPath: sourceBuffer?.path,
        sourceContent: sourceBuffer && hasTextContent(sourceBuffer) ? sourceBuffer.content : '',
      }
    }),
  )
  const fontSize = useEditorSettingsStore.use.fontSize()
  const uiFontFamily = useSettingsStore((state) => state.settings.uiFontFamily)
  const handleFileSelect = useFileSystemStore((s) => s.handleFileSelect)
  const [html, setHtml] = useState('')
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!sourceContent) {
      setHtml('')
      return
    }

    const parsedHtml = parseMarkdown(sourceContent)
    setHtml(parsedHtml)
  }, [sourceContent])

  // A tab switch unmounts this pane entirely (PaneContainer renders only the
  // active buffer), so the scroll offset has to be retained outside the
  // component. Restored once the parsed HTML is in the DOM — before that the
  // scroll box is empty and any offset would be clamped away.
  usePreservedScroll(containerRef, bufferId ?? null, html !== '')

  const handleLinkClick = useCallback(
    async (e: React.MouseEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement
      const link = target.closest('a')

      if (!link) return

      const href = link.getAttribute('href')
      if (!href) return

      e.preventDefault()
      e.stopPropagation()

      if (href.startsWith('#')) {
        const elementId = href.substring(1)
        const targetElement = containerRef.current?.querySelector(`#${CSS.escape(elementId)}`)
        if (targetElement) {
          targetElement.scrollIntoView({ behavior: 'smooth' })
        }
        return
      }

      const isExternalLink =
        href.startsWith('http://') ||
        href.startsWith('https://') ||
        href.startsWith('mailto:') ||
        href.startsWith('tel:') ||
        href.startsWith('//')

      if (isExternalLink) {
        try {
          await openExternalUrl(href)
        } catch (error) {
          logger.error('MarkdownPreview', 'Failed to open external link:', error)
        }
        return
      }

      if (!sourceBufferPath) return

      const targetPath = resolvePreviewLinkPath(href, sourceBufferPath)

      try {
        // The `.md`-suffix retry is why this keeps a pre-check instead of just
        // opening optimistically: bare markdown links ([spec](./spec)) are
        // extension-less by convention, and the open path swallows its own
        // failure into a toast, so a failed open cannot drive the second
        // attempt. `exists` costs one lazy directory listing and rejects on a
        // real daemon error rather than reporting "not there".
        if (await exists(targetPath)) {
          await handleFileSelect?.(targetPath, false)
          return
        }

        const withMd = targetPath.endsWith('.md') ? targetPath : `${targetPath}.md`
        if (await exists(withMd)) {
          await handleFileSelect?.(withMd, false)
          return
        }

        logger.warn('MarkdownPreview', `File not found: ${targetPath}`)
      } catch (error) {
        logger.error('MarkdownPreview', 'Failed to handle link:', error)
      }
    },
    [sourceBufferPath, handleFileSelect],
  )

  const handleWheelCapture = useCallback((event: React.WheelEvent<HTMLDivElement>) => {
    const container = containerRef.current
    if (!container) return

    const canScroll = container.scrollHeight > container.clientHeight
    if (!canScroll || event.deltaY === 0) return

    container.scrollTop += event.deltaY
    event.preventDefault()
  }, [])

  return (
    <div
      ref={containerRef}
      // handleLinkClick is pure event delegation: it walks up to the nearest
      // real <a> (target.closest('a')) rendered inside the sanitized markdown
      // below and only acts on that. The actual interactive elements are
      // those anchors — already natively focusable and keyboard-activatable
      // (Enter on a focused link fires a bubbling click, which lands right
      // here) — so this wrapper carries no semantics of its own.
      role="presentation"
      className="markdown-preview flex h-full justify-center overflow-auto bg-transparent p-6"
      style={{
        fontSize: `${fontSize}px`,
        fontFamily: `${uiFontFamily}, sans-serif`,
      }}
      onClick={handleLinkClick}
      onWheelCapture={handleWheelCapture}
    >
      <div
        className="markdown-content w-full max-w-3xl"
        // react-doctor-disable-next-line dangerous-html-sink -- `html` is the output of parseMarkdown(), which returns DOMPurify.sanitize(rawHtml) (parser.ts:217). Already flows through the existing DOMPurify usage; the rule can't trace the cross-function async setState.
        dangerouslySetInnerHTML={{ __html: html }}
      />
    </div>
  )
}
