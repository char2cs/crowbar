import './styles.css'
import { useCallback, useMemo, useRef } from 'react'
import { usePreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import { exists } from '@/features/file-system/controllers/platform'
import { siblingRef } from '@/features/editor/documents/document-ref'
import { openPreview } from '@/features/editor/documents/open-preview'
import { usePreviewSource } from '@/features/editor/documents/use-preview-source'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { LinkWorkspaceContext } from '@/lib/markdown-link'
import { openWorkspaceLink } from '@/lib/workspace-link'
import { useSettingsStore } from '@/features/settings/store'
import { logger } from '../utils/logger'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { splitFrontmatter } from './plate/markdown-frontmatter'

export interface MarkdownPreviewProps {
  bufferId?: string
}

async function openLinkedDocument(source: DocumentRef, href: string): Promise<void> {
  const target = siblingRef(source, href)
  if (target.authority === 'host') {
    await openPreview(target)
    return
  }
  const { workspaceId, path } = target
  if (await exists(workspaceId, path)) {
    await openPreview(target)
  } else if (!path.endsWith('.md') && (await exists(workspaceId, `${path}.md`))) {
    await openPreview(workspaceRef(workspaceId, `${path}.md`))
  } else {
    logger.warn('MarkdownPreview', `File not found: ${path}`)
  }
}

function slugify(text: string): string {
  return text
    .trim()
    .toLowerCase()
    .replace(/[^\p{L}\p{N}\s-]/gu, '')
    .replace(/\s+/g, '-')
}

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
  const { content, doc } = usePreviewSource(bufferId)
  return <MarkdownPreviewView doc={doc} content={content} scrollKey={bufferId ?? null} />
}

export interface MarkdownPreviewViewProps {
  doc: DocumentRef | null
  content: string
  scrollKey: string | null
}

export function MarkdownPreviewView({
  doc: sourceDoc,
  content: sourceContent,
  scrollKey,
}: MarkdownPreviewViewProps) {
  const sourceWorkspaceId = sourceDoc?.authority === 'workspace' ? sourceDoc.workspaceId : null
  const fontSize = useSettingsStore((state) => state.settings.fontSize)
  const uiFontFamily = useSettingsStore((state) => state.settings.uiFontFamily)
  const containerRef = useRef<HTMLDivElement>(null)

  const body = useMemo(() => splitFrontmatter(sourceContent).body, [sourceContent])

  usePreservedScroll(containerRef, scrollKey, body !== '')

  const handleLinkClick = useCallback(
    async (e: React.MouseEvent<HTMLDivElement>) => {
      const target = e.target as HTMLElement
      const link = target.closest('a')

      if (!link) return

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
