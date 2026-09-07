'use client'

import { useEffect, useState } from 'react'
import type { TLinkElement } from 'platejs'
import { PlateElement, type PlateElementProps, useComposedRef } from 'platejs/react'
import { LinkElement } from '@/components/ui/link-node'
import { FileExplorerIcon } from '@/features/file-explorer/components/file-explorer-icon'
import { useMarkdownAsset } from '@/features/editor/markdown/plate/markdown-asset'
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
 *
 * INTERACTIVE variant — registered on `chatComposerPlugins`. Renders through
 * `ChatAttachmentFileCard`, which is reorderable via `AttachmentDragHandle`.
 * See `ChatLinkElementStatic` below for the settled/read-only counterpart.
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

/** The read-only/settled counterpart of `ChatLinkElement`, registered on
 *  `chatComposerPluginsStatic` (see chat-composer-plugins.ts). Renders
 *  through `ChatAttachmentFileCardStatic` instead, which never touches
 *  `@platejs/dnd`'s `useDraggable` — a settled message is read, not
 *  reordered, so it has no reason to require a `<DndProvider>` ancestor. */
export function ChatLinkElementStatic(props: PlateElementProps<TLinkElement>) {
  const asset = useMarkdownAsset()
  const url = props.element.url
  const parsed = parseChatAttachmentRef(url)

  if (!parsed || !asset) return <LinkElement {...props} />

  return (
    <ChatAttachmentFileCardStatic
      {...props}
      wsId={asset.wsId}
      attachmentRef={url}
      filename={parsed.filename}
    />
  )
}

/** The metadata both the interactive and static cards render — the fetched
 *  size label, and the href a click should follow — kept as one hook so
 *  neither variant can drift from the other's idea of what a card shows. */
function useChatAttachmentCardMeta(
  wsId: string,
  attachmentRef: string,
): { href: string | null; sizeLabel: string | null } {
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

  return { href: chatAttachmentUrl(wsId, attachmentRef), sizeLabel: formatAttachmentSize(size) }
}

type FileCardProps = PlateElementProps<TLinkElement> & {
  wsId: string
  attachmentRef: string
  filename: string
}

/**
 * The drag handle sits OUTSIDE the `<a>` on purpose — nesting a `<button>`
 * (`AttachmentDragHandle`) inside a `<a>` (interactive content inside
 * interactive content) is invalid HTML5 and an accessibility/focus-order
 * smell (a screen reader has to guess which control a click landed on). The
 * code-block variant (`DraggableAttachmentBlock`) never has this problem —
 * its own `PlateElement` root is a plain block, not an anchor — but this one
 * IS an anchor, so the handle and drop line are rendered as SIBLINGS of it,
 * both children of an outer, non-interactive `<span>` that carries the
 * `group/attachment` hover scope and the `position: relative` the handle's
 * own absolute positioning is measured against. `nodeRef` (the draggable
 * node `@platejs/dnd` measures and previews) still composes onto the anchor
 * itself — the actual visible card — not the wrapper, same as before this
 * split; Slate's own `props.attributes`/`ref` stay on the anchor too, so its
 * DOM identity, click/href behaviour and `LinkFloatingToolbar` compatibility
 * are all unchanged from before this task.
 */
function ChatAttachmentFileCard({ wsId, attachmentRef, filename, ...props }: FileCardProps) {
  const { href, sizeLabel } = useChatAttachmentCardMeta(wsId, attachmentRef)
  const { isDragging, nodeRef, handleRef, remove } = useAttachmentDraggable(props.element)

  return (
    <span className="group/attachment relative inline-flex align-middle">
      <AttachmentControls dragRef={handleRef} onDelete={remove} />
      <AttachmentDropLine />
      <PlateElement
        {...props}
        as="a"
        ref={useComposedRef(props.ref, nodeRef)}
        className={cn('chat-attachment-file-card', ATTACHMENT_BOX_CLASS, isDragging && 'opacity-50')}
        attributes={{
          ...props.attributes,
          href: href ?? undefined,
          onClick: (e) => handleMarkdownAnchorClick(e, href),
          onMouseOver: (e) => {
            e.stopPropagation()
          },
        }}
      >
        <FileExplorerIcon fileName={filename} size={28} className="shrink-0" />
        {/* contentEditable={false} — a filename is a reference, not prose a caret should enter. */}
        <span
          contentEditable={false}
          className="line-clamp-2 w-full break-words px-1 font-medium text-foreground text-xs"
        >
          {props.children}
        </span>
        {sizeLabel && <span className="text-[11px] text-muted-foreground">{sizeLabel}</span>}
      </PlateElement>
    </span>
  )
}

function ChatAttachmentFileCardStatic({ wsId, attachmentRef, filename, ...props }: FileCardProps) {
  const { href, sizeLabel } = useChatAttachmentCardMeta(wsId, attachmentRef)

  return (
    <PlateElement
      {...props}
      as="a"
      className={cn('chat-attachment-file-card', ATTACHMENT_BOX_CLASS)}
      attributes={{
        ...props.attributes,
        href: href ?? undefined,
        onClick: (e) => handleMarkdownAnchorClick(e, href),
        onMouseOver: (e) => {
          e.stopPropagation()
        },
      }}
    >
      <FileExplorerIcon fileName={filename} size={28} className="shrink-0" />
      <span className="line-clamp-2 w-full break-words px-1 font-medium text-foreground text-xs">
        {props.children}
      </span>
      {sizeLabel && <span className="text-[11px] text-muted-foreground">{sizeLabel}</span>}
    </PlateElement>
  )
}
