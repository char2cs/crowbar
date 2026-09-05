'use client'

import { useEffect, useState } from 'react'
import type { TLinkElement } from 'platejs'
import { PlateElement, type PlateElementProps } from 'platejs/react'
import { LinkElement } from '@/components/ui/link-node'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import { handleMarkdownAnchorClick } from '@/lib/markdown-link'
import {
  chatAttachmentUrl,
  fetchChatAttachmentMetadata,
  parseChatAttachmentRef,
} from './chat-asset-resolver'

const UNITS = ['KB', 'MB', 'GB', 'TB']

/** Same units scheme as the file explorer's Properties dialog
 *  (use-file-explorer-context-menu.tsx), kept independent since that helper
 *  isn't exported. */
export function formatAttachmentSize(bytes: number | null): string | null {
  if (bytes === null || !Number.isFinite(bytes) || bytes < 0) return null
  if (bytes < 1024) return `${bytes} bytes`
  let value = bytes / 1024
  let unitIndex = 0
  while (value >= 1024 && unitIndex < UNITS.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${value.toFixed(value >= 10 ? 1 : 2)} ${UNITS[unitIndex]}`
}

/**
 * `[filename](ref)` for a real chat attachment renders as a file card
 * instead of a bare link. Falls through to the ordinary `LinkElement` for
 * everything else: a `url` that isn't `chats/{chatId}/attachments/{file}`,
 * or no `MarkdownAssetContext` at all (the un-wired, degraded case).
 *
 * No `props.path`/sibling lookup here (unlike chat-code-block-node.tsx's
 * `findFollowingImageRef`) — everything this component needs comes off the
 * link's own `element.url`, so there's no analogous staleness risk to guard
 * against.
 */
export function ChatLinkElement(props: PlateElementProps<TLinkElement>) {
  const asset = useMarkdownAsset()
  const url = props.element.url
  const parsed = parseChatAttachmentRef(url)

  if (!parsed || !asset) return <LinkElement {...props} />

  return (
    <ChatAttachmentFileCard
      {...props}
      wsId={asset.wsId}
      attachmentRef={url}
      filename={parsed.filename}
    />
  )
}

function ChatAttachmentFileCard({
  wsId,
  attachmentRef,
  filename,
  ...props
}: PlateElementProps<TLinkElement> & {
  wsId: string
  attachmentRef: string
  filename: string
}) {
  const [size, setSize] = useState<number | null>(null)

  useEffect(() => {
    let cancelled = false
    void fetchChatAttachmentMetadata(wsId, attachmentRef).then((meta) => {
      if (!cancelled) setSize(meta?.size ?? null)
    })
    return () => {
      cancelled = true
    }
  }, [wsId, attachmentRef])

  const href = chatAttachmentUrl(wsId, attachmentRef)
  const sizeLabel = formatAttachmentSize(size)

  return (
    <PlateElement
      {...props}
      as="a"
      className="chat-attachment-file-card inline-flex items-center gap-2 rounded-md border border-border bg-muted/40 px-2 py-1 align-middle text-sm no-underline"
      attributes={{
        ...props.attributes,
        href: href ?? undefined,
        onClick: (e) => handleMarkdownAnchorClick(e, href),
        onMouseOver: (e) => {
          e.stopPropagation()
        },
      }}
    >
      <FileExplorerIcon fileName={filename} size={14} className="shrink-0" />
      <span className="truncate">{props.children}</span>
      {sizeLabel && <span className="shrink-0 text-muted-foreground text-xs">{sizeLabel}</span>}
    </PlateElement>
  )
}
