import { useCallback, useImperativeHandle, useMemo, useRef } from 'react'
import { layoutHeight } from '@/lib/layout-size'
import type { CSSProperties, KeyboardEvent, Ref } from 'react'
import { NodeApi, PointApi, RangeApi, type Descendant, type Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import { createPlatePlugin, Plate, PlateContent, usePlateEditor } from 'platejs/react'
import {
  insertAttachmentMarkdownInto,
  insertPendingImageInto,
  settlePendingImageInto,
} from '@/features/agent/composer/plate/chat-attachment-insert'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatComposerStructurePlugin } from '@/features/agent/composer/plate/chat-composer-structure'
import {
  chatMarkdownToValue,
  chatValueToMarkdown,
} from '@/features/agent/composer/plate/chat-composer-serialization'
import { createChatPastePlugin } from '@/features/agent/composer/plate/chat-paste-plugin'
import { cn } from '@/lib/utils'
import { IS_MOBILE } from '@/utils/platform'

/** Whether the caret has anywhere left to go WITHIN the text — the box's own
 *  arrow-key history recall only fires at these edges, so moving through a
 *  wrapped or multi-paragraph draft is never hijacked. */
export interface CaretEdges {
  atStart: boolean
  atEnd: boolean
}

// An imperative handle because the plus button and the attach/Excalidraw modals live outside
// `<Plate>`'s tree, where `useEditorRef` cannot reach.
export interface ChatMarkdownEditorHandle {
  /** Inserts `markdown` at the selection (else the end) through the editor's own codec, so an
   *  attachment block or image lands as the exact node shape a paste of it would. */
  insertAttachmentMarkdown(markdown: string): void
  /** Inserts a local `objectUrl` image as the preview shown before its upload starts; pair
   *  with `settlePendingImage` once the upload settles. */
  insertPendingImage(objectUrl: string, alt: string): void
  /** Finds the pending image by `objectUrl`: swaps in `finalMarkdown`'s url in place, or
   *  removes the placeholder when it is `null` (the upload failed). */
  settlePendingImage(objectUrl: string, finalMarkdown: string | null): void
  /** The box's markdown: serialized here, once, for whoever sends it. */
  getMarkdown(): string
  /** Replaces the text, caret at its end, with a fresh undo history. */
  setMarkdown(markdown: string): void
  /** Adds blocks after the last one, leaving the caret where the person had it. */
  appendMarkdown(markdown: string): void
  focus(): void
  clear(): void
}

/** What the chat reads off the box while it is typed into, without markdown. */
export interface ComposerShape {
  empty: boolean
  /** The text after a leading `/` on the first line, or null when there is none. */
  slashQuery: string | null
}

// A list or quote paragraph is text-bearing markdown even when it has no letters;
// a block id is the editor's bookkeeping, not markdown.
function blankParagraph(node: Descendant): boolean {
  const markup = Object.keys(node).filter((key) => key !== 'id' && key !== 'children')
  if (!('type' in node) || node.type !== 'p' || markup.length > 1) return false
  return NodeApi.string(node).trim() === ''
}

function shapeOf(children: Value): ComposerShape {
  const first = children[0]
  const line = first && 'type' in first && first.type === 'p' ? NodeApi.string(first) : ''
  return {
    empty: children.every(blankParagraph),
    slashQuery: line.startsWith('/') ? line.slice(1) : null,
  }
}

/** Loads markdown as if the box had just opened with it. */
function loadMarkdown(editor: PlateEditor, markdown: string) {
  editor.tf.replaceNodes(chatMarkdownToValue(markdown), { at: [], children: true })
  editor.history.undos = []
  editor.history.redos = []
  const end = editor.api.end([])
  if (end) editor.tf.select(end)
}

export interface ChatMarkdownEditorProps {
  /** Where a pasted image uploads. Without both ids the paste plugin is not registered and a
   *  paste falls through to Slate's default handling. */
  wsId?: string
  chatId?: string
  placeholder: string
  ariaLabel: string
  /** Fires after every content change; never serializes. `edited` is false for
   *  text loaded through `setMarkdown`/`clear`, true for anything else. */
  onShapeChange: (shape: ComposerShape, edited: boolean) => void
  /** The chat's own key handling. `readMarkdown` reads the editor as it stands: React state
   *  can lag the keystroke, and sending from it drops the last character. */
  onKeyDown: (
    event: KeyboardEvent<HTMLDivElement>,
    readMarkdown: () => string,
    caret: CaretEdges,
    shape: ComposerShape,
  ) => void
  /** The editable's measured height, for a control that rides its last line. */
  onHeightChange?: (height: number) => void
  autoFocus?: boolean
  /** Combobox wiring for the chat's skill picker. */
  expanded?: boolean
  controls?: string
  className?: string
  style?: CSSProperties
  ref?: Ref<ChatMarkdownEditorHandle>
}

// The rich-markdown prompt box, shared by the conversation's pill and the blank chat's document.
// Uncontrolled: the text lives only in the editor; callers push through the handle and read once
// on send, so typing serializes and re-renders nothing outside the box.
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
}: ChatMarkdownEditorProps) {
  // A plugin, not the `onKeyDown` DOM prop: the prop runs after Slate's own handling, so Enter
  // would split the block instead of sending. Read through a ref so the editor is built once
  // while the handler stays current.
  const keyHandlerRef = useRef(onKeyDown)
  keyHandlerRef.current = onKeyDown
  const keyPlugin = useMemo(
    () =>
      createPlatePlugin({
        key: 'agent-chat-keys',
        handlers: {
          onKeyDown: ({ editor: current, event }) => {
            // Select-all done here, not by WebKit's native `selectAll:`: AppKit's command
            // routing is not guaranteed to reach a framework-managed contenteditable.
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
              shapeOf(current.children as Value),
            )
          },
        },
      }),
    [],
  )

  // A file drop belongs to the wrapping `.pill`/`.docwrap` handler; slate-react would also insert
  // the file's text/plain payload as raw text. preventDefault marks it handled for slate-react
  // without stopping propagation, so the app's handler still gets the event.
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

  // Undefined without both ids. Built once: the editor's plugin list is fixed at mount.
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
          keyPlugin,
          dropGuardPlugin,
          pastePlugin,
        ]
      : [...chatComposerPlugins, chatComposerStructurePlugin, keyPlugin, dropGuardPlugin],
    autoSelect: 'end',
    // Unused here, and it re-subscribes every edited block to the editor store on
    // each keystroke: one more render queued behind every key typed.
    navigationFeedback: false,
    // Plate drops block ids under NODE_ENV=test; kept so tests see the browser's blocks.
    nodeId: true,
  })

  // Plate copies its `onChange` prop into its own store from an effect, so a new
  // handler per render costs the editor a second commit per keystroke.
  const shapeHandlerRef = useRef(onShapeChange)
  shapeHandlerRef.current = onShapeChange

  // Reported synchronously from the handle too: Plate's own onChange fires later.
  // A load's own later onChange sees the very children it left: not an edit.
  const loadedRef = useRef<Value | null>(null)
  const report = useCallback(
    (edited = true) => shapeHandlerRef.current(shapeOf(editor.children as Value), edited),
    [editor],
  )
  const handleChange = useCallback(() => {
    // A click or an arrow key cannot change the text. An empty operation list
    // cannot be told apart from an edit, so it reports.
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
      // Not `focusAndReport`: the upload can settle long after the person moved on, and
      // refocusing would yank the caret away from wherever they are now.
      settlePendingImage: (objectUrl: string, finalMarkdown: string | null) => {
        settlePendingImageInto(editor, objectUrl, finalMarkdown)
        report()
      },
      getMarkdown: () => chatValueToMarkdown(editor.children as Value),
      setMarkdown: load,
      appendMarkdown: (markdown: string) => {
        if (shapeOf(editor.children as Value).empty) return load(markdown)
        const at = [editor.children.length]
        editor.tf.insertNodes(chatMarkdownToValue(markdown), { at, select: false })
        report()
      },
      focus: () => editor.tf.focus(),
      clear: () => load(''),
    }),
    [editor, focusAndReport, load, report],
  )

  // The editable's measured height, observed because only the browser knows where
  // a line wrapped. A ref callback, not an effect: Slate can replace the editable's
  // node without this component remounting, and the ref sees every attach.
  const heightObserverRef = useRef<ResizeObserver | null>(null)
  const reportHeightFrom = useCallback(
    (node: HTMLDivElement | null) => {
      heightObserverRef.current?.disconnect()
      heightObserverRef.current = null
      if (!node || !onHeightChange) return
      // A zero is the absence of a box (a parked view is display:none), never
      // a height: publishing it re-renders the whole chat view on every switch.
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
        // On the phone focus raises the keyboard over the chat just opened; a tap asks for it.
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
