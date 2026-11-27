'use client'

import type { PlateElementProps } from 'platejs/react'
import { PlateElement, useComposedRef, useReadOnly } from 'platejs/react'
import {
  AttachmentDropLine,
  useAttachmentDropTarget,
} from '@/features/agent/composer/plate/attachment-drag-handle'

const PARAGRAPH_CLASS = 'group/attachment-drop relative m-0 px-0 py-1'

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
