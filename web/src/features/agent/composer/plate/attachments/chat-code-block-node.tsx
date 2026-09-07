'use client'

import { useCallback, type ReactNode } from 'react'
import { type TCodeBlockElement, NodeApi, PathApi } from 'platejs'
import {
  PlateElement,
  type PlateEditor,
  type PlateElementProps,
  useComposedRef,
  useEditorRef,
} from 'platejs/react'
import { cn } from '@/lib/utils'
import {
  AttachmentControls,
  AttachmentDropLine,
  useAttachmentDraggable,
} from '@/features/agent/composer/plate/attachment-drag-handle'
import { parseAttachmentLang } from './attachment-lang'
import { parseExcalidrawScene, type ParsedExcalidrawScene } from './excalidraw-scene'
import { TextAttachmentPill } from './text-attachment-pill'
import { ExcalidrawPreview } from './excalidraw-preview'

/** A code block's full multi-line source, reconstructed from its `code_line`
 *  children — `NodeApi.string(element)` would concatenate lines with no
 *  separator. Same technique `mermaid-code-block.tsx`'s isMermaid branch
 *  already uses. */
function codeBlockSource(element: TCodeBlockElement): string {
  return element.children.map((line) => NodeApi.string(line)).join('\n')
}

/** The scene a code block resolves to as an excalidraw fence, or `null` if
 *  it isn't one (wrong lang) or its content doesn't parse as a real scene —
 *  the single gate `resolveCodeBlockPreview`'s own excalidraw branch and
 *  `ChatMarkdownImageElement`'s sibling-suppression check both defer to, so
 *  "does this fence show a preview" and "does its PNG sibling hide itself"
 *  can never disagree. */
export function excalidrawSceneFromCodeBlock(
  element: TCodeBlockElement,
): ParsedExcalidrawScene | null {
  const parsed = parseAttachmentLang(element.lang)
  if (parsed?.kind !== 'excalidraw' && element.lang !== 'excalidraw') return null
  return parseExcalidrawScene(codeBlockSource(element))
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

/**
 * Deletes an excalidraw fence AND its own persisted-PNG sibling together —
 * `excalidraw-takeover.tsx` inserts the two as one adjacent pair, one logical
 * attachment. Deleting only the fence (`useAttachmentDraggable`'s generic
 * `remove`, which knows nothing about this pairing) orphaned the image:
 * still `hidden` (`isExcalidrawPngSibling` never re-evaluates once its own
 * preceding fence is gone — slate-react does not re-render a node whose OWN
 * props are unchanged just because a SIBLING was removed), but very much
 * still in the document — and still sent, reappearing in the message the
 * person had just "deleted" it from. Removes the image FIRST, at its own
 * (unshifted) path, before removing the fence — the other order would
 * require re-deriving the image's path after the fence's removal shifts it.
 */
function removeExcalidrawAttachment(editor: PlateEditor, element: TCodeBlockElement) {
  const path = editor.api.findPath(element)
  if (!path) return
  editor.tf.withoutNormalizing(() => {
    if (excalidrawSceneFromCodeBlock(element)) {
      const nextPath = PathApi.next(path)
      const next = editor.api.node(nextPath)?.[0] as { type?: string } | undefined
      if (next?.type === 'img') editor.tf.removeNodes({ at: nextPath })
    }
    editor.tf.removeNodes({ at: path })
  })
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
  } else {
    // A bare `excalidraw` tag (no `:{id}` suffix) never comes from the
    // composer — that path always mints one (excalidrawMarkdown, chat-
    // attachment-markdown.ts) — but it's exactly what an AGENT writes when a
    // reply includes a diagram: it has no way to know the id-suffix
    // convention exists at all. Rather than requiring one, this falls back
    // to content validation alone: parseExcalidrawScene's structural check
    // (a real elements[]/appState shape) is already a strong enough guard
    // against a fence that merely mentions "excalidraw" in prose — the same
    // false-positive the id suffix exists to prevent for the composer's own
    // fences, just proven a different way.
    const scene = excalidrawSceneFromCodeBlock(element)
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
  const editor = useEditorRef()
  const { isDragging, nodeRef, handleRef, remove } = useAttachmentDraggable(props.element)
  const removeAttachment = useCallback(() => {
    if (excalidrawSceneFromCodeBlock(props.element)) {
      removeExcalidrawAttachment(editor, props.element)
    } else {
      remove()
    }
  }, [editor, props.element, remove])

  return (
    <PlateElement
      {...props}
      ref={useComposedRef(props.ref, nodeRef)}
      className={cn('relative my-2', isDragging && 'opacity-50')}
    >
      <AttachmentDropLine />
      <div contentEditable={false} className="group/attachment relative inline-block select-none">
        <AttachmentControls dragRef={handleRef} onDelete={removeAttachment} />
        {preview}
      </div>
      <div className="hidden">{codeBody}</div>
    </PlateElement>
  )
}

/**
 * Chat's fenced-code-block renderer, for the INTERACTIVE editor (the
 * composer, and the transcript's currently-streaming bubble — see
 * `chatComposerPlugins`, not its static derivative). A `text-attachment:{id}`
 * tag with a validly-shaped id, or an `excalidraw:{id}` / bare `excalidraw`
 * tag whose content parses as a real scene, renders a kind-specific preview,
 * reorderable via `AttachmentDragHandle`; anything else — including a bare
 * `text-attachment` tag with no id, or invalid Excalidraw JSON — falls
 * through to a plain code block, identical to before this task and with no
 * drag machinery at all.
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
        // `inline-block`, not a plain block: the chat's very first turn
        // renders full-width (`.frozen` in transcript.css), unlike an
        // ordinary bubble which shrinks to its content — a plain block here
        // would stretch to that full width right along with it, landing
        // ExcalidrawPreview's own absolutely-positioned edit button off in
        // the empty space past the diagram's real (narrower) edge instead of
        // on its corner. Shrinking to content keeps this wrapper's width
        // tied to the preview's own, in every context alike.
        <div className="chat-attachment-block inline-block">
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
