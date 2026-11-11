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
import { ChatImageLightbox, useChatImageLightbox } from './chat-image-lightbox'

/** The PNG after an excalidraw fence is already drawn by `ExcalidrawPreview`.
 *  `findPath` fresh rather than `useNodePath`, which can be stale. */
function isExcalidrawPngSibling(props: PlateElementProps): boolean {
  const path = props.editor.api.findPath(props.element)
  if (!path) return false
  const prevPath = PathApi.previous(path)
  if (!prevPath) return false
  const prev = props.editor.api.node(prevPath)?.[0] as { type?: string } | undefined
  if (!prev || prev.type !== 'code_block') return false
  return excalidrawSceneFromCodeBlock(prev as TCodeBlockElement) !== null
}

/** The cap every attachment preview shares, so a tall photo can't take over the composer. */
const IMAGE_MAX_HEIGHT_CLASS = 'max-h-80'

/** A `<button>` rather than an onClick on the `<img>`, so it is keyboard-reachable. */
function ZoomableChatImage({ src, alt }: { src: string; alt: string }) {
  const lightbox = useChatImageLightbox()
  return (
    <>
      <button
        type="button"
        onClick={lightbox.show}
        title="Click to expand"
        className="group/zoom relative inline-block cursor-zoom-in rounded"
      >
        <img
          src={src}
          alt={alt}
          className={cn(
            'markdown-image inline-block w-auto max-w-full rounded object-contain',
            IMAGE_MAX_HEIGHT_CLASS,
          )}
        />
        <span className="absolute inset-0 rounded bg-black/0 transition-colors group-hover/zoom:bg-black/10" />
      </button>
      <ChatImageLightbox
        src={src}
        alt={alt}
        open={lightbox.open}
        onOpenChange={lightbox.onOpenChange}
      />
    </>
  )
}

function ChatAttachmentImageBlock(props: PlateElementProps) {
  const element = props.element as unknown as MarkdownImageNode
  const url = element.url ?? ''
  const alt = captionToAlt(element.caption)
  const resolvedSrc = useResolvedImageSrc(url)
  const { isDragging, nodeRef, handleProps, dropLine, remove } = useAttachmentDraggable(
    props.element,
  )

  return (
    <PlateElement
      {...props}
      ref={useComposedRef(props.ref, nodeRef)}
      className={cn('group/attachment relative inline-block', isDragging && 'opacity-50')}
    >
      <AttachmentControls handleProps={handleProps} onDelete={remove} />
      <AttachmentDropLine line={dropLine} />
      <span contentEditable={false}>
        <ZoomableChatImage src={resolvedSrc} alt={alt} />
      </span>
      {props.children}
    </PlateElement>
  )
}

function ChatAttachmentImageBlockStatic(props: PlateElementProps) {
  const element = props.element as unknown as MarkdownImageNode
  const url = element.url ?? ''
  const alt = captionToAlt(element.caption)
  const resolvedSrc = useResolvedImageSrc(url)

  return (
    <PlateElement {...props} className="inline-block">
      <span contentEditable={false}>
        <ZoomableChatImage src={resolvedSrc} alt={alt} />
      </span>
      {props.children}
    </PlateElement>
  )
}

/** The excalidraw PNG sibling stays mounted but hidden, so Slate's node<->DOM
 *  mapping is never disturbed. */
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
