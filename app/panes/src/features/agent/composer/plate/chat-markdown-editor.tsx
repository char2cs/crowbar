import { useCallback, useEffect, useImperativeHandle, useMemo, useRef } from 'react'
import { layoutHeight } from '@/lib/layout-size'
import type { CSSProperties, KeyboardEvent, Ref } from 'react'
import { NodeApi, PointApi, RangeApi, type Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import { createPlatePlugin, Plate, PlateContent, usePlateEditor } from 'platejs/react'
import {
  insertAttachmentMarkdownInto,
  insertPendingImageInto,
  settlePendingImageInto,
} from '@/features/agent/composer/plate/chat-attachment-insert'
import { ChatComposerChipPlugin } from '@/features/agent/composer/plate/chat-composer-chips'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  blankParagraph,
  chatComposerStructurePlugin,
  createDocumentTitlePlugin,
} from '@/features/agent/composer/plate/chat-composer-structure'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import { createChatPastePlugin } from '@/features/agent/composer/plate/chat-paste-plugin'
import { cn } from '@/lib/utils'
import { IS_MOBILE } from '@/utils/platform'
import { forgetDraft, loadDraft, saveDraft } from '@/features/agent/composer/lib/draft-persistence'

export interface CaretEdges {
  atStart: boolean
  atEnd: boolean
}

export interface ChatMarkdownEditorHandle {
  insertAttachmentMarkdown(markdown: string): void
  insertPendingImage(objectUrl: string, alt: string): void
  settlePendingImage(objectUrl: string, finalMarkdown: string | null): void
  getMarkdown(): string
  setMarkdown(markdown: string): void
  appendMarkdown(markdown: string): void
  completeMention(text: string): void
  insertTrigger(trigger: '@'): void
  startCommand(): void
  completeCommand(text: string): void
  focus(): void
  clear(): void
}

export interface ComposerShape {
  empty: boolean
  slashQuery: string | null
  mentionQuery: string | null
}

function textBeforeCaret(editor: PlateEditor): string {
  const { selection } = editor
  if (!selection || !RangeApi.isCollapsed(selection)) return ''
  const leaf = NodeApi.get(editor, selection.anchor.path)
  return leaf && 'text' in leaf ? String(leaf.text).slice(0, selection.anchor.offset) : ''
}

function mentionBefore(editor: PlateEditor): string | null {
  const before = textBeforeCaret(editor)
  if (!editor.selection || !RangeApi.isCollapsed(editor.selection)) return null
  return /(?:^|\s)@([^\s@]*)$/.exec(before)?.[1] ?? null
}

function shapeOf(editor: PlateEditor): ComposerShape {
  const children = editor.children as Value
  const first = children[0]
  const line = first && 'type' in first && first.type === 'p' ? NodeApi.string(first) : ''
  return {
    empty: children.every(blankParagraph),
    slashQuery: line.startsWith('/') ? line.slice(1) : null,
    mentionQuery: mentionBefore(editor),
  }
}

function loadMarkdown(editor: PlateEditor, markdown: string) {
  editor.tf.replaceNodes(chatMarkdownToValue(markdown), { at: [], children: true })
  editor.history.undos = []
  editor.history.redos = []
  const end = editor.api.end([])
  if (end) editor.tf.select(end)
}

export interface ChatMarkdownEditorProps {
  wsId?: string
  chatId?: string
  placeholder: string
  ariaLabel: string
  onShapeChange: (shape: ComposerShape, edited: boolean) => void
  onKeyDown: (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
    shape: ComposerShape,
  ) => void
  onHeightChange?: (height: number) => void
  autoFocus?: boolean
  expanded?: boolean
  controls?: string
  className?: string
  style?: CSSProperties
  ref?: Ref<ChatMarkdownEditorHandle>
  persistAs?: string
  document?: boolean
}

export function ChatMarkdownEditor({
  wsId,
  chatId,
  placeholder,
  ariaLabel,
  onShapeChange,
  onKeyDown,
  onHeightChange,
  autoFocus,
  expanded,
  controls,
  className,
  style,
  ref,
  persistAs,
  document,
}: ChatMarkdownEditorProps) {
  const documentRef = useRef(!!document)
  documentRef.current = !!document
  const titlePlugin = useMemo(() => createDocumentTitlePlugin(() => documentRef.current), [])
  const keyHandlerRef = useRef(onKeyDown)
  keyHandlerRef.current = onKeyDown
  const keyPlugin = useMemo(
    () =>
      createPlatePlugin({
        key: 'agent-chat-keys',
        handlers: {
          onKeyDown: ({ editor: current, event }) => {
            if (
              (event.metaKey || event.ctrlKey) &&
              !event.shiftKey &&
              !event.altKey &&
              event.key.toLowerCase() === 'a'
            ) {
              const docStart = current.api.start([])
              const docEnd = current.api.end([])
              if (docStart && docEnd) {
                event.preventDefault()
                current.tf.select({ anchor: docStart, focus: docEnd })
              }
              return
            }
            if (
              documentRef.current &&
              event.key === 'Enter' &&
              !event.metaKey &&
              !event.ctrlKey &&
              shapeOf(current).slashQuery === null &&
              shapeOf(current).mentionQuery === null
            ) {
              return
            }
            const { selection } = current
            let atStart = false
            let atEnd = false
            if (selection && RangeApi.isCollapsed(selection)) {
              const docStart = current.api.start([])
              const docEnd = current.api.end([])
              atStart = !!docStart && PointApi.equals(selection.anchor, docStart)
              atEnd = !!docEnd && PointApi.equals(selection.anchor, docEnd)
            }
            keyHandlerRef.current(
              event as KeyboardEvent<HTMLDivElement>,
              () => chatValueToMarkdown(current.children as Value),
              { atStart, atEnd },
              shapeOf(current),
            )
          },
        },
      }),
    [],
  )

  const dropGuardPlugin = useMemo(
    () =>
      createPlatePlugin({
        key: 'agent-chat-drop-guard',
        handlers: {
          onDrop: ({ event }) => {
            if (!event.dataTransfer?.types.includes('Files')) return
            event.preventDefault()
            return true
          },
        },
      }),
    [],
  )

  const pastePlugin = useMemo(
    () => (wsId && chatId ? createChatPastePlugin({ wsId, chatId }) : undefined),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount value only, deliberately not re-derived
    [],
  )

  const editor = usePlateEditor({
    plugins: pastePlugin
      ? [
          ...chatComposerPlugins,
          chatComposerStructurePlugin,
          titlePlugin,
          ChatComposerChipPlugin,
          keyPlugin,
          dropGuardPlugin,
          pastePlugin,
        ]
      : [
          ...chatComposerPlugins,
          chatComposerStructurePlugin,
          titlePlugin,
          ChatComposerChipPlugin,
          keyPlugin,
          dropGuardPlugin,
        ],
    autoSelect: 'end',
    navigationFeedback: false,
    nodeId: true,
  })

  const shapeHandlerRef = useRef(onShapeChange)
  shapeHandlerRef.current = onShapeChange

  const loadedRef = useRef<Value | null>(null)
  const report = useCallback(
    (edited = true) => shapeHandlerRef.current(shapeOf(editor), edited),
    [editor],
  )
  const handleChange = useCallback(() => {
    const ops = editor.operations
    if (ops.length > 0 && ops.every((op) => op.type === 'set_selection')) return
    report(editor.children !== loadedRef.current)
  }, [editor, report])
  const focusAndReport = useCallback(() => {
    editor.tf.focus()
    report()
  }, [editor, report])
  const load = useCallback(
    (markdown: string) => {
      loadMarkdown(editor, markdown)
      loadedRef.current = editor.children as Value
      editor.tf.focus()
      report(false)
    },
    [editor, report],
  )

  useImperativeHandle(
    ref,
    () => ({
      insertAttachmentMarkdown: (markdown: string) => {
        insertAttachmentMarkdownInto(editor, markdown)
        focusAndReport()
      },
      insertPendingImage: (objectUrl: string, alt: string) => {
        insertPendingImageInto(editor, objectUrl, alt)
        focusAndReport()
      },
      settlePendingImage: (objectUrl: string, finalMarkdown: string | null) => {
        settlePendingImageInto(editor, objectUrl, finalMarkdown)
        report()
      },
      getMarkdown: () => chatValueToMarkdown(editor.children as Value),
      setMarkdown: load,
      appendMarkdown: (markdown: string) => {
        if (shapeOf(editor).empty) return load(markdown)
        const at = [editor.children.length]
        editor.tf.insertNodes(chatMarkdownToValue(markdown), { at, select: false })
        report()
      },
      completeMention: (text: string) => {
        const typed = mentionBefore(editor)
        if (typed === null) return
        editor.tf.delete({ distance: typed.length + 1, unit: 'character', reverse: true })
        editor.tf.insertText(text)
        focusAndReport()
      },
      insertTrigger: (trigger: '@') => {
        editor.tf.focus()
        editor.tf.insertText(/\S$/.test(textBeforeCaret(editor)) ? ` ${trigger}` : trigger)
        report()
      },
      startCommand: () => {
        const first = editor.children[0]
        const commandFirst = first && 'type' in first && NodeApi.string(first).startsWith('/')
        if (!commandFirst) {
          if (shapeOf(editor).empty) load('/')
          else editor.tf.insertNodes({ type: 'p', children: [{ text: '/' }] }, { at: [0] })
        }
        editor.tf.select(editor.api.end([0])!)
        editor.tf.focus()
        report()
      },
      completeCommand: (text: string) => {
        const [start, end] = [editor.api.start([0]), editor.api.end([0])]
        if (!start || !end) return
        editor.tf.select({ anchor: start, focus: end })
        editor.tf.insertText(text)
        focusAndReport()
      },
      focus: () => editor.tf.focus(),
      clear: () => {
        if (persistAs) forgetDraft(persistAs)
        load('')
      },
    }),
    [editor, focusAndReport, load, report, persistAs],
  )

  useEffect(() => {
    if (!persistAs) return
    const saved = loadDraft(persistAs)
    if (saved && shapeOf(editor).empty) {
      loadMarkdown(editor, saved)
      loadedRef.current = editor.children as Value
      report(false)
    }
    const save = () => saveDraft(persistAs, chatValueToMarkdown(editor.children as Value))
    window.addEventListener('pagehide', save)
    return () => {
      window.removeEventListener('pagehide', save)
      save()
    }
  }, [editor, persistAs, report])

  const heightObserverRef = useRef<ResizeObserver | null>(null)
  const reportHeightFrom = useCallback(
    (node: HTMLDivElement | null) => {
      heightObserverRef.current?.disconnect()
      heightObserverRef.current = null
      if (!node || !onHeightChange) return
      const report = () => {
        const height = layoutHeight(node)
        if (height > 0) onHeightChange(height)
      }
      report()
      const observer = new ResizeObserver(report)
      // react-doctor-disable-next-line effect-needs-cleanup -- cleanup exists (disconnect() at the top of this same callback, which also runs on every detach since React calls a ref callback with node=null then); tracer expects a useEffect return, not a ref-callback's own next invocation.
      observer.observe(node)
      heightObserverRef.current = observer
    },
    [onHeightChange],
  )

  return (
    <Plate editor={editor} onChange={handleChange}>
      <PlateContent
        ref={reportHeightFrom}
        autoFocus={autoFocus && !IS_MOBILE}
        placeholder={placeholder}
        aria-label={ariaLabel}
        aria-expanded={expanded}
        aria-controls={controls}
        className={cn('field', className)}
        style={style}
      />
    </Plate>
  )
}
