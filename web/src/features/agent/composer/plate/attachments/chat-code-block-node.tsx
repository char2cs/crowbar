'use client'

import type { ReactNode } from 'react'
import { type TCodeBlockElement, NodeApi, PathApi } from 'platejs'
import { PlateElement, type PlateElementProps, useComposedRef } from 'platejs/react'
import { cn } from '@/lib/utils'
import {
  AttachmentDragHandle,
  AttachmentDropLine,
  useAttachmentDraggable,
} from '@/features/agent/composer/plate/attachment-drag-handle'
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
 *  `![diagram](ref)` right after, per the design spec).
 *
 * Uses `editor.api.findPath` rather than `props.path`: in the interactive
 * renderer `props.path` is sourced from Plate's `useNodePath`, which is
 * memoized on `[editor.api, node]` (`@platejs/core`'s `PluginElementWithPath`)
 * — per its own JSDoc, "if another node is updated in a way that affects
 * this node's path, this hook will not return the new path." `findPath` is
 * an unmemoized tree-walk, correct regardless of what else moved; the static
 * renderer needs no such care since it recomputes `path` fresh every render
 * regardless. (Empirically, slate-react's own `MemoizedElement` wrapper
 * bails out of re-rendering this node at all unless its OWN `element`
 * reference changes — which would invalidate `useNodePath`'s memo too — so
 * this staleness could not be reproduced through the normal rendering
 * pipeline in this library version; that is an internal implementation
 * detail of a dependency, not a documented contract, so it is not relied on
 * here. `findPath` matches Plate's own stated `useNodePath` caveat and costs
 * one extra tree-walk on an already-rare path.) The `!path` guard exists for
 * `findPath`'s own `Path | undefined` contract when the element isn't in the
 * tree at all — see `findFollowingImageRef`'s own unit test for that case,
 * exercised directly since a mounted document can't trigger it. */
export function findFollowingImageRef(
  props: PlateElementProps<TCodeBlockElement>,
): string | undefined {
  const path = props.editor.api.findPath(props.element)
  if (!path) return undefined
  const node = props.editor.api.node(PathApi.next(path))?.[0] as
    { type?: string; url?: string } | undefined
  return node?.type === 'img' ? node.url : undefined
}

/** The raw code body, plus whichever kind-specific preview a fence resolves
 *  to (or `null` for a plain/unrecognized block) — the one piece of logic
 *  the draggable (interactive) and plain (static) renderers below must never
 *  be allowed to disagree on. */
function resolveCodeBlockPreview(props: PlateElementProps<TCodeBlockElement>): {
  codeBody: ReactNode
  preview: ReactNode
} {
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

  return { codeBody, preview }
}

/**
 * The draggable shell for an attachment preview — split into its own
 * component, rather than calling `useAttachmentDraggable` unconditionally
 * from `ChatCodeBlockElement` itself, so an ORDINARY, non-attachment code
 * block never mounts `@platejs/dnd`'s `useDraggable` at all and therefore
 * never needs a `<DndProvider>` ancestor either — only a fence that actually
 * resolved to a preview does. (Conditionally choosing WHICH component to
 * render is fine under the rules of hooks; conditionally calling a hook
 * from inside one component is not — this is why the branch lives one level
 * up, in `ChatCodeBlockElement`, rather than as an early return in here.)
 */
function DraggableAttachmentBlock({
  preview,
  codeBody,
  ...props
}: PlateElementProps<TCodeBlockElement> & { preview: ReactNode; codeBody: ReactNode }) {
  const { isDragging, nodeRef, handleRef } = useAttachmentDraggable(props.element)

  return (
    <PlateElement
      {...props}
      ref={useComposedRef(props.ref, nodeRef)}
      className={cn('group/attachment relative my-2', isDragging && 'opacity-50')}
    >
      <AttachmentDragHandle dragRef={handleRef} />
      <AttachmentDropLine />
      <div contentEditable={false} className="select-none">
        {preview}
      </div>
      <div className="hidden">{codeBody}</div>
    </PlateElement>
  )
}

/**
 * Chat's fenced-code-block renderer, for the INTERACTIVE editor (the
 * composer, and the transcript's currently-streaming bubble — see
 * `chatComposerPlugins`, not its static derivative). A `text-attachment:{id}`/
 * `excalidraw:{id}` tag with a validly-shaped id (and, for Excalidraw,
 * content that actually parses as a scene) renders a kind-specific preview,
 * reorderable via `AttachmentDragHandle`; anything else — including a bare
 * tag with no id, or invalid JSON — falls through to a plain code block,
 * identical to before this task and with no drag machinery at all.
 *
 * The raw block stays mounted whenever a preview renders over it — same
 * hidden-but-present technique `mermaid-code-block.tsx` uses — so Slate's
 * node<->DOM mapping is never disturbed.
 */
export function ChatCodeBlockElement(props: PlateElementProps<TCodeBlockElement>) {
  const { codeBody, preview } = resolveCodeBlockPreview(props)

  if (!preview) {
    return (
      <PlateElement {...props} className="my-2">
        {codeBody}
      </PlateElement>
    )
  }

  return <DraggableAttachmentBlock {...props} preview={preview} codeBody={codeBody} />
}

/**
 * The read-only/settled counterpart, registered on `chatComposerPluginsStatic`
 * (see chat-composer-plugins.ts). Same preview resolution as the interactive
 * renderer above, but never touches `@platejs/dnd`'s `useDraggable` — a
 * settled message is read, not reordered, so it has no reason to require a
 * `<DndProvider>` ancestor either.
 */
export function ChatCodeBlockElementStatic(props: PlateElementProps<TCodeBlockElement>) {
  const { codeBody, preview } = resolveCodeBlockPreview(props)

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
