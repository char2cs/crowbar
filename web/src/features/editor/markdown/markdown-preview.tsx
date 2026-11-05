import './styles.css'
import { useCallback, useMemo, useRef } from 'react'
import { usePreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import { exists } from '@/features/file-system/controllers/platform'
import { siblingRef } from '@/features/editor/documents/document-ref'
import { openDocument } from '@/features/editor/documents/open-document'
import { usePreviewSource } from '@/features/editor/documents/use-preview-source'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { LinkWorkspaceContext } from '@/lib/markdown-link'
import { openWorkspaceLink } from '@/lib/workspace-link'
import { useSettingsStore } from '@/features/settings/store'
import { logger } from '../utils/logger'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { splitFrontmatter } from './plate/markdown-frontmatter'

export interface MarkdownPreviewProps {
  /** The `markdownPreview` buffer rendered, and the key its scroll offset is kept under (the
   *  pane unmounts the preview behind another tab). Omitted by the legacy CodeEditor host, which
   *  falls back to the active pane's buffer and keeps no scroll. */
  bufferId?: string
}

/** Open a relative link from `source`'s document. An extension-less workspace link also tries
 *  `.md`, checked up front because a failed open reports itself and cannot drive a retry. */
async function openLinkedDocument(source: DocumentRef, href: string): Promise<void> {
  const target = siblingRef(source, href)
  if (target.authority === 'host') {
    await openDocument(target, { preview: true })
    return
  }
  const { workspaceId, path } = target
  if (await exists(workspaceId, path)) {
    await openDocument(target, { preview: true })
  } else if (!path.endsWith('.md') && (await exists(workspaceId, `${path}.md`))) {
    await openDocument(workspaceRef(workspaceId, `${path}.md`), { preview: true })
  } else {
    logger.warn('MarkdownPreview', `File not found: ${path}`)
  }
}

/** GitHub-style heading slug: `## Getting Started` → `getting-started`. */
function slugify(text: string): string {
  return text
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s-]/gu, '')
    .replace(/\s+/g, '-')
}

/** An element with that id, else the heading whose slug is that id. */
function findAnchorTarget(root: HTMLElement | null, id: string): Element | null {
  if (!root) return null
  const byId = root.querySelector(`#${CSS.escape(id)}`)
  if (byId) return byId
  const wanted = decodeURIComponent(id).toLowerCase()
  for (const heading of root.querySelectorAll('h1, h2, h3, h4, h5, h6')) {
    if (slugify(heading.textContent ?? '') === wanted) return heading
  }
  return null
}

export function MarkdownPreview({ bufferId }: MarkdownPreviewProps) {
  const { content: sourceContent, doc: sourceDoc } = usePreviewSource(bufferId)
  // Links in a host document belong to no workspace, so none is refused as another host's.
  const sourceWorkspaceId = sourceDoc?.authority === 'workspace' ? sourceDoc.workspaceId : null
  const fontSize = useSettingsStore((state) => state.settings.fontSize)
  const uiFontFamily = useSettingsStore((state) => state.settings.uiFontFamily)
  const containerRef = useRef<HTMLDivElement>(null)

  // Rendered by the app's one read-only markdown renderer (Plate static, the
  // same one chat messages and review comments use). Frontmatter is metadata,
  // not prose: markdown would read its `---` fences as a rule and a heading.
  const body = useMemo(() => splitFrontmatter(sourceContent).body, [sourceContent])

  // A tab switch unmounts this pane, so the scroll offset is kept outside it. Restored once the
  // rendered body is in the DOM; before that the box is empty and any offset clamps away.
  usePreservedScroll(containerRef, bufferId ?? null, body !== '')

  const handleLinkClick = useCallback(
    async (e: React.MouseEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement
      const link = target.closest('a')

      if (!link) return

      // The link plugin renders a relative url without `href` (it only emits
      // urls it deems safe to navigate); its own url stays on the element.
      const href = link.getAttribute('href') ?? link.getAttribute('data-slate-url')
      if (!href) return

      e.preventDefault()
      e.stopPropagation()

      if (href.startsWith('#')) {
        const targetElement = findAnchorTarget(containerRef.current, href.substring(1))
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
          await openWorkspaceLink(href, sourceWorkspaceId)
        } catch (error) {
          logger.error('MarkdownPreview', 'Failed to open external link:', error)
        }
        return
      }

      if (!sourceDoc) return
      try {
        await openLinkedDocument(sourceDoc, href)
      } catch (error) {
        logger.error('MarkdownPreview', 'Failed to handle link:', error)
      }
    },
    [sourceDoc, sourceWorkspaceId],
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
      // Pure delegation: handleLinkClick acts only on the nearest real <a>, which is natively
      // focusable and fires a bubbling click on Enter, so this wrapper has no semantics of its own.
      role="presentation"
      className="markdown-preview flex h-full justify-center overflow-auto bg-transparent p-6"
      style={{
        fontSize: `${fontSize}px`,
        fontFamily: `${uiFontFamily}, sans-serif`,
      }}
      onClick={handleLinkClick}
      onWheelCapture={handleWheelCapture}
    >
      <div className="markdown-content w-full max-w-3xl">
        <LinkWorkspaceContext value={sourceWorkspaceId}>
          {body && <MarkdownMessageStatic>{body}</MarkdownMessageStatic>}
        </LinkWorkspaceContext>
      </div>
    </div>
  )
}
