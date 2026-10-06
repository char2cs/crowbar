'use client'

import type { PlateElementProps } from 'platejs/react'
import { PlateElement, useComposedRef, useReadOnly } from 'platejs/react'
import {
  AttachmentDropLine,
  useAttachmentDropTarget,
} from '@/features/agent/composer/plate/attachment-drag-handle'

/**
 * The INTERACTIVE composer's paragraph — same look as the shared
 * `ParagraphElement` (components/ui/paragraph-node.tsx), plus registering
 * as a drop target via `useAttachmentDropTarget`. Without this, an
 * attachment could only ever be reordered against ANOTHER attachment
 * (nothing made a plain paragraph a drop target at all) —
 * reported live as "I can't move an attachment between paragraphs," even
 * after reordering attachments against each other started working.
 *
 * Registered on `chatComposerPlugins` only (chat-composer-plugins.ts) — the
 * STATIC/settled variant keeps the plain `ParagraphElement`, because a
 * settled message is read, not edited.
 */
const PARAGRAPH_CLASS = 'group/attachment-drop relative m-0 px-0 py-1'

// The streaming transcript message reuses the composer's plugins but is
// read-only: nothing can land in it, and every droppable it registered made
// dnd-kit re-render its whole scope on each delta.
export function ChatParagraphElement(props: PlateElementProps) {
  const readOnly = useReadOnly()
  if (readOnly) {
    return (
      <PlateElement {...props} className={PARAGRAPH_CLASS}>
        {props.children}
      </PlateElement>
    )
  }
  return <DropTargetParagraph {...props} />
}

function DropTargetParagraph(props: PlateElementProps) {
  const { nodeRef, dropLine } = useAttachmentDropTarget(props.element)
  return (
    <PlateElement {...props} ref={useComposedRef(props.ref, nodeRef)} className={PARAGRAPH_CLASS}>
      <AttachmentDropLine line={dropLine} />
      {props.children}
    </PlateElement>
  )
}
