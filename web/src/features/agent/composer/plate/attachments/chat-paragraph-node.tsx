'use client'

import type { PlateElementProps } from 'platejs/react'
import { PlateElement, useComposedRef } from 'platejs/react'
import { cn } from '@/lib/utils'
import {
  AttachmentDropLine,
  useAttachmentDropTarget,
} from '@/features/agent/composer/plate/attachment-drag-handle'

/**
 * The INTERACTIVE composer's paragraph — same look as the shared
 * `ParagraphElement` (components/ui/paragraph-node.tsx), plus registering
 * as a drop target via `useAttachmentDropTarget`. Without this, an
 * attachment could only ever be reordered against ANOTHER attachment
 * (nothing made a plain paragraph a valid `useDropNode` target at all) —
 * reported live as "I can't move an attachment between paragraphs," even
 * after reordering attachments against each other started working.
 *
 * Registered on `chatComposerPlugins` only (chat-composer-plugins.ts) — the
 * STATIC/settled variant keeps the plain `ParagraphElement`, both because a
 * settled message is read, not edited, and because `useAttachmentDropTarget`
 * calling `@platejs/dnd`'s `useDropNode` needs `editor.plugins.dnd`
 * registered, which the static plugin set deliberately drops.
 */
export function ChatParagraphElement(props: PlateElementProps) {
  const { nodeRef } = useAttachmentDropTarget(props.element)
  return (
    <PlateElement
      {...props}
      ref={useComposedRef(props.ref, nodeRef)}
      className={cn('group/attachment-drop relative m-0 px-0 py-1')}
    >
      <AttachmentDropLine />
      {props.children}
    </PlateElement>
  )
}
