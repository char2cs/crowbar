'use client'

import { type ReactNode, useEffect, useState } from 'react'
import type { TLinkElement } from 'platejs'
import { PlateElement, type PlateElementProps, useComposedRef } from 'platejs/react'
import { LinkElement } from '@/components/ui/link-node'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
import { useHostResolved } from '@/lib/host-resolve'
import { handleMarkdownAnchorClick } from '@/lib/markdown-link'
import { cn } from '@/lib/utils'
import {
  AttachmentControls,
  AttachmentDropLine,
  useAttachmentDraggable,
} from '@/features/agent/composer/plate/attachment-drag-handle'
import {
  chatAttachmentUrl,
  fetchChatAttachmentMetadata,
  parseChatAttachmentRef,
} from './chat-asset-resolver'
import { ATTACHMENT_BOX_CLASS } from './attachment-box'

const UNITS = ['KB', 'MB', 'GB', 'TB']

/** Same units as the file explorer's Properties dialog, whose helper isn't exported. */
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

function useChatAttachmentCardMeta(
  wsId: string,
  attachmentRef: string,
): { href: string | null; sizeLabel: string | null } {
  const [size, setSize] = useState<number | null>(null)
  // The URL names the workspace's host, unknown until its project row loads: no
  // link and no request before then, and the hook re-renders the card when it is.
  const hostResolved = useHostResolved({ wsId })

  useEffect(() => {
    if (!hostResolved) return
    let cancelled = false
    void fetchChatAttachmentMetadata(wsId, attachmentRef).then((meta) => {
      if (!cancelled) setSize(meta?.size ?? null)
    })
    return () => {
      cancelled = true
    }
  }, [wsId, attachmentRef, hostResolved])

  return {
    href: hostResolved ? chatAttachmentUrl(wsId, attachmentRef) : null,
    sizeLabel: formatAttachmentSize(size),
  }
}

type FileCardProps = PlateElementProps<TLinkElement> & {
  wsId: string
  attachmentRef: string
  filename: string
}

/** The anchor a card draws; `editable` keeps the caret out of the filename. */
function FileCardAnchor({
  wsId,
  attachmentRef,
  filename,
  editable = false,
  className,
  ...props
}: FileCardProps & { editable?: boolean; className?: string }) {
  const { href, sizeLabel } = useChatAttachmentCardMeta(wsId, attachmentRef)
  return (
    <PlateElement
      {...props}
      as="a"
      className={cn('chat-attachment-file-card', ATTACHMENT_BOX_CLASS, className)}
      attributes={{
        ...props.attributes,
        href: href ?? undefined,
        onClick: (e) => handleMarkdownAnchorClick(e, href, wsId),
        onMouseOver: (e) => e.stopPropagation(),
      }}
    >
      <FileExplorerIcon fileName={filename} size={28} className="shrink-0" />
      <span
        contentEditable={editable ? false : undefined}
        className="line-clamp-2 w-full break-words px-1 font-medium text-foreground text-xs"
      >
        {props.children}
      </span>
      {sizeLabel && <span className="text-[11px] text-muted-foreground">{sizeLabel}</span>}
    </PlateElement>
  )
}

/** The controls are siblings of the `<a>`, not children: a `<button>` inside an
 *  anchor is invalid HTML. `nodeRef` and Slate's ref still sit on the anchor itself. */
function ChatAttachmentFileCard(props: FileCardProps) {
  const { isDragging, nodeRef, handleProps, dropLine, remove } = useAttachmentDraggable(
    props.element,
  )
  return (
    <span className="group/attachment relative inline-flex align-middle">
      <AttachmentControls handleProps={handleProps} onDelete={remove} />
      <AttachmentDropLine line={dropLine} />
      <FileCardAnchor
        {...props}
        ref={useComposedRef(props.ref, nodeRef)}
        editable
        className={isDragging ? 'opacity-50' : undefined}
      />
    </span>
  )
}

/** Falls through to `LinkElement` for a non-attachment url or with no `MarkdownAssetContext`. */
function chatLink(Card: (props: FileCardProps) => ReactNode) {
  return function ChatLink(props: PlateElementProps<TLinkElement>) {
    const asset = useMarkdownAsset()
    const url = props.element.url
    const parsed = parseChatAttachmentRef(url)
    if (!parsed || !asset || !('wsId' in asset)) return <LinkElement {...props} />
    return <Card {...props} wsId={asset.wsId} attachmentRef={url} filename={parsed.filename} />
  }
}

export const ChatLinkElement = chatLink(ChatAttachmentFileCard)
/** Never touches `useAttachmentDraggable`, so a settled message needs no `DndScope`. */
export const ChatLinkElementStatic = chatLink(FileCardAnchor)
