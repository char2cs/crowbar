'use client'

import { useCallback, useContext, type ReactNode } from 'react'
import { type TCodeBlockElement, NodeApi, PathApi } from 'platejs'
import {
  PlateElement,
  type PlateEditor,
  type PlateElementProps,
  useComposedRef,
  useEditorRef,
} from 'platejs/react'
import { cn } from '@/lib/utils'
import { CODE_SYNTAX_THEME } from '@/components/ui/code-syntax-theme'
import { useRedecorateWhenGrammarReady } from '@/components/editor/plugins/use-redecorate-when-grammar-ready'
import {
  AttachmentControls,
  AttachmentDropLine,
  useAttachmentDraggable,
} from '@/features/agent/composer/plate/attachment-drag-handle'
import { parseAttachmentLang } from './attachment-lang'
import { parseExcalidrawScene, type ParsedExcalidrawScene } from './excalidraw-scene'
import { TextAttachmentPill } from './text-attachment-pill'
import { ExcalidrawPreview } from './excalidraw-preview'
import { DiagramBlock, type DiagramTarget } from './diagram-block'
import { StreamingFenceContext } from '@/features/agent/transcript/plate/streaming-fence'
import type { DiagramFence } from '@/features/agent/transcript/plate/streaming-markdown-parse'

function codeBlockSource(element: TCodeBlockElement): string {
  return element.children.map((line) => NodeApi.string(line)).join('\n')
}

export function excalidrawSceneFromCodeBlock(
  element: TCodeBlockElement,
): ParsedExcalidrawScene | null {
  const parsed = parseAttachmentLang(element.lang)
  if (parsed?.kind !== 'excalidraw' && element.lang !== 'excalidraw') return null
  return parseExcalidrawScene(codeBlockSource(element))
}

function findFollowingImageRef(props: PlateElementProps<TCodeBlockElement>): string | undefined {
  const path = props.editor.api.findPath(props.element)
  if (!path) return undefined
  const node = props.editor.api.node(PathApi.next(path))?.[0] as
    { type?: string; url?: string } | undefined
  return node?.type === 'img' ? node.url : undefined
}

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

function codeBodyOf(props: PlateElementProps<TCodeBlockElement>): ReactNode {
  return (
    <pre
      className={cn(
        'overflow-x-auto rounded-md bg-muted/60 p-3 font-mono text-xs leading-relaxed [tab-size:2]',
        CODE_SYNTAX_THEME,
      )}
    >
      <code>{props.children}</code>
    </pre>
  )
}

function resolveCodeBlockPreview(props: PlateElementProps<TCodeBlockElement>): {
  codeBody: ReactNode
  preview: ReactNode
} {
  const { element } = props
  const parsed = parseAttachmentLang(element.lang)
  const codeBody = codeBodyOf(props)

  let preview: ReactNode = null
  if (parsed?.kind === 'text-attachment') {
    preview = <TextAttachmentPill text={codeBlockSource(element)} />
  } else {
    const scene = excalidrawSceneFromCodeBlock(element)
    if (scene) preview = <ExcalidrawPreview scene={scene} pngRef={findFollowingImageRef(props)} />
  }

  return { codeBody, preview }
}

function diagramKind(lang: string): DiagramFence | null {
  if (lang === 'mermaid') return 'mermaid'
  return lang === 'excalidraw' || parseAttachmentLang(lang)?.kind === 'excalidraw'
    ? 'excalidraw'
    : null
}

function isLastTopLevelBlock(props: PlateElementProps<TCodeBlockElement>): boolean {
  const path = props.editor.api.findPath(props.element)
  return path?.length === 1 && path[0] === props.editor.children.length - 1
}

interface DiagramRoute {
  kind: DiagramFence
  target: DiagramTarget
  scene?: ParsedExcalidrawScene
}

function useDiagramTarget(props: PlateElementProps<TCodeBlockElement>): DiagramRoute | null {
  const fence = useContext(StreamingFenceContext)
  if (!fence) return null
  const lang = props.element.lang ?? ''
  const open = fence.openFence
  if (open && lang && open.startsWith(lang.split(':')[0]!) && isLastTopLevelBlock(props)) {
    return { kind: open, target: fence.live ? 'skeleton' : 'code' }
  }
  const kind = diagramKind(lang)
  if (!kind) return null
  if (kind === 'mermaid') return { kind, target: 'diagram' }
  const scene = excalidrawSceneFromCodeBlock(props.element) ?? undefined
  return { kind, target: scene ? 'diagram' : 'code', scene }
}

function DiagramElement({
  diagram,
  ...props
}: PlateElementProps<TCodeBlockElement> & { diagram: DiagramRoute }) {
  return (
    <PlateElement {...props}>
      <DiagramBlock
        kind={diagram.kind}
        target={diagram.target}
        source={codeBlockSource(props.element)}
        preview={
          diagram.scene && (
            <ExcalidrawPreview scene={diagram.scene} pngRef={findFollowingImageRef(props)} />
          )
        }
        code={codeBodyOf(props)}
      />
    </PlateElement>
  )
}

function DraggableAttachmentBlock({
  preview,
  codeBody,
  ...props
}: PlateElementProps<TCodeBlockElement> & { preview: ReactNode; codeBody: ReactNode }) {
  const editor = useEditorRef()
  const { isDragging, nodeRef, handleProps, dropLine, remove } = useAttachmentDraggable(
    props.element,
  )
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
      <AttachmentDropLine line={dropLine} />
      <div contentEditable={false} className="group/attachment relative inline-block select-none">
        <AttachmentControls handleProps={handleProps} onDelete={removeAttachment} />
        {preview}
      </div>
      <div className="hidden">{codeBody}</div>
    </PlateElement>
  )
}

export function ChatCodeBlockElement(props: PlateElementProps<TCodeBlockElement>) {
  useRedecorateWhenGrammarReady(props.editor, props.element)
  const diagram = useDiagramTarget(props)
  if (diagram) return <DiagramElement {...props} diagram={diagram} />
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

export function ChatCodeBlockElementStatic(props: PlateElementProps<TCodeBlockElement>) {
  const diagram = useDiagramTarget(props)
  if (diagram) return <DiagramElement {...props} diagram={diagram} />
  const { codeBody, preview } = resolveCodeBlockPreview(props)

  return (
    <PlateElement {...props} className="my-2">
      {preview ? (
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
