'use client'

import type { ReactNode } from 'react'
import { type TCodeBlockElement, NodeApi, PathApi } from 'platejs'
import { PlateElement, type PlateElementProps } from 'platejs/react'
import { parseAttachmentLang } from './attachment-lang'
import { parseExcalidrawScene } from './excalidraw-scene'
import { TextAttachmentPill } from './text-attachment-pill'
import { ExcalidrawPreview } from './excalidraw-preview'

/** A code block's full multi-line source, reconstructed from its `code_line`
 *  children — `NodeApi.string(element)` would concatenate lines with no
 *  separator. Same technique `mermaid-code-block.tsx`'s isMermaid branch
 *  already uses. */
function codeBlockSource(element: TCodeBlockElement): string {
  return element.children.map((line) => NodeApi.string(line)).join('\n')
}

/** The `url` of the `img` node immediately following this code block, if
 *  any — the Excalidraw kind's persisted-PNG sibling (fenced JSON, then
 *  `![diagram](ref)` right after, per the design spec). `props.path` is
 *  Plate's own up-to-date path for this element (both the interactive and
 *  static renderers compute and pass it), so no redundant `findPath` lookup
 *  is needed. */
function findFollowingImageRef(props: PlateElementProps<TCodeBlockElement>): string | undefined {
  const node = props.editor.api.node(PathApi.next(props.path))?.[0] as
    { type?: string; url?: string } | undefined
  return node?.type === 'img' ? node.url : undefined
}

/**
 * Chat's fenced-code-block renderer. A `text-attachment:{id}`/`excalidraw:{id}`
 * tag with a validly-shaped id (and, for Excalidraw, content that actually
 * parses as a scene) renders a kind-specific preview; anything else —
 * including a bare tag with no id, or invalid JSON — falls through to the
 * same plain rendering `CommentCodeBlockElement` (the component this
 * replaces) already used, verbatim.
 *
 * The raw block stays mounted whenever a preview renders over it — same
 * hidden-but-present technique `mermaid-code-block.tsx` uses — so Slate's
 * node<->DOM mapping is never disturbed.
 */
export function ChatCodeBlockElement(props: PlateElementProps<TCodeBlockElement>) {
  const { element } = props
  const parsed = parseAttachmentLang(element.lang)

  const codeBody = (
    <pre className="overflow-x-auto rounded-md bg-muted/60 p-3 font-mono text-xs leading-relaxed [tab-size:2]">
      <code>{props.children}</code>
    </pre>
  )

  let preview: ReactNode = null
  if (parsed?.kind === 'text-attachment') {
    preview = <TextAttachmentPill text={codeBlockSource(element)} />
  } else if (parsed?.kind === 'excalidraw') {
    const scene = parseExcalidrawScene(codeBlockSource(element))
    if (scene) preview = <ExcalidrawPreview scene={scene} pngRef={findFollowingImageRef(props)} />
  }

  return (
    <PlateElement {...props} className="my-2">
      {preview ? (
        <div className="chat-attachment-block">
          <div contentEditable={false} className="select-none">
            {preview}
          </div>
          <div className="hidden">{codeBody}</div>
        </div>
      ) : (
        codeBody
      )}
    </PlateElement>
  )
}
