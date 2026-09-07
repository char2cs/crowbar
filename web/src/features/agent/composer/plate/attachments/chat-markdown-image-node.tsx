'use client'

import { type TCodeBlockElement, PathApi } from 'platejs'
import { PlateElement, type PlateElementProps, useComposedRef } from 'platejs/react'
import {
  captionToAlt,
  useResolvedImageSrc,
  type MarkdownImageNode,
} from '@/features/editor/markdown/plate/markdown-image-node'
import { cn } from '@/lib/utils'
import {
  AttachmentControls,
  AttachmentDropLine,
  useAttachmentDraggable,
} from '@/features/agent/composer/plate/attachment-drag-handle'
import { excalidrawSceneFromCodeBlock } from './chat-code-block-node'

/** True when this image is the excalidraw kind's persisted-PNG sibling — the
 *  fenced JSON immediately before it, per the design spec — which
 *  `ExcalidrawPreview` already renders (via `findFollowingImageRef`).
 *  Rendering the plain markdown image TOO would show the same diagram twice,
 *  which is exactly what was reported live. Uses `findPath` fresh, same as
 *  `findFollowingImageRef`, for the same `useNodePath` staleness reason (see
 *  that function's own doc comment). */
function isExcalidrawPngSibling(props: PlateElementProps): boolean {
  const path = props.editor.api.findPath(props.element)
  if (!path) return false
  const prevPath = PathApi.previous(path)
  if (!prevPath) return false
  const prev = props.editor.api.node(prevPath)?.[0] as { type?: string } | undefined
  if (!prev || prev.type !== 'code_block') return false
  return excalidrawSceneFromCodeBlock(prev as TCodeBlockElement) !== null
}

/** Same `max-h-80` cap every other attachment preview uses (the excalidraw
 *  preview, the text-attachment pill) — reported live: nothing capped a
 *  photo's height at all, so a tall one could take over the whole composer. */
const IMAGE_MAX_HEIGHT_CLASS = 'max-h-80'

/**
 * An attachment image's INTERACTIVE half — the composer, and the transcript's
 * currently-streaming bubble (`chatComposerPlugins`, not its static
 * derivative). Reorderable via the SAME `useAttachmentDraggable` every other
 * attachment kind uses (`ATTACHMENT_DND_TYPE` unifies them onto one dnd
 * type) — reported live: every OTHER kind got a drag handle when that
 * feature shipped, images never did.
 */
function ChatAttachmentImageBlock(props: PlateElementProps) {
  const element = props.element as unknown as MarkdownImageNode
  const url = element.url ?? ''
  const alt = captionToAlt(element.caption)
  const resolvedSrc = useResolvedImageSrc(url)
  const { isDragging, nodeRef, handleRef, remove } = useAttachmentDraggable(props.element)

  return (
    <PlateElement
      {...props}
      ref={useComposedRef(props.ref, nodeRef)}
      className={cn('group/attachment relative inline-block', isDragging && 'opacity-50')}
    >
      <AttachmentControls dragRef={handleRef} onDelete={remove} />
      <AttachmentDropLine />
      <span contentEditable={false}>
        <img
          src={resolvedSrc}
          alt={alt}
          className={cn(
            'markdown-image inline-block w-auto max-w-full rounded object-contain',
            IMAGE_MAX_HEIGHT_CLASS,
          )}
        />
      </span>
      {props.children}
    </PlateElement>
  )
}

/** The settled/read-only counterpart of `ChatAttachmentImageBlock` — same
 *  height cap, no drag furniture: a settled message is read, not reordered,
 *  and has no `<DndProvider>` ancestor to call `useAttachmentDraggable`
 *  against (registered on `chatComposerPluginsStatic`, which drops
 *  `DndPlugin` — see chat-composer-plugins.ts). */
function ChatAttachmentImageBlockStatic(props: PlateElementProps) {
  const element = props.element as unknown as MarkdownImageNode
  const url = element.url ?? ''
  const alt = captionToAlt(element.caption)
  const resolvedSrc = useResolvedImageSrc(url)

  return (
    <PlateElement {...props} className="inline-block">
      <span contentEditable={false}>
        <img
          src={resolvedSrc}
          alt={alt}
          className={cn(
            'markdown-image inline-block w-auto max-w-full rounded object-contain',
            IMAGE_MAX_HEIGHT_CLASS,
          )}
        />
      </span>
      {props.children}
    </PlateElement>
  )
}

/**
 * Chat's `img` node renderer, INTERACTIVE variant — a reorderable, height-
 * capped attachment for an ordinary image, but hidden (mounted, not visible —
 * same "raw node stays mounted so Slate's node<->DOM mapping is never
 * disturbed" technique `chat-code-block-node.tsx` uses) for the one case a
 * plain chat editor never wants to show it: the excalidraw PNG sibling,
 * already shown by the fence's own preview.
 */
export function ChatMarkdownImageElement(props: PlateElementProps) {
  if (isExcalidrawPngSibling(props)) {
    return (
      <PlateElement {...props} className="hidden">
        {props.children}
      </PlateElement>
    )
  }
  return <ChatAttachmentImageBlock {...props} />
}

/** The settled/read-only counterpart, registered on `chatComposerPluginsStatic`
 *  (chat-composer-plugins.ts). Same excalidraw-sibling hide, same height cap,
 *  never draggable. */
export function ChatMarkdownImageElementStatic(props: PlateElementProps) {
  if (isExcalidrawPngSibling(props)) {
    return (
      <PlateElement {...props} className="hidden">
        {props.children}
      </PlateElement>
    )
  }
  return <ChatAttachmentImageBlockStatic {...props} />
}
