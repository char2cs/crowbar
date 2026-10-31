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

/** Whether the caret has anywhere left to go WITHIN the text — the box's own
 *  arrow-key history recall only fires at these edges, so moving through a
 *  wrapped or multi-paragraph draft is never hijacked. */
export interface CaretEdges {
  atStart: boolean
  atEnd: boolean
}

/** What a caller OUTSIDE the editable can do to it — the same imperative-
 *  handle shape `AgentEmptyDocumentHandle` already uses for `getHandleRect`
 *  (`agent-empty-document.tsx`), for the same reason: the plus button, the
 *  Attach File modal, and the Excalidraw modal all live outside `<Plate>`'s
 *  own tree, so `useEditorRef` is not reachable from any of them. */
export interface ChatMarkdownEditorHandle {
  /**
   * Inserts a block-level node at the current selection (end of document if
   * there is none), built by deserializing `markdown` through this editor's
   * own `chatComposerPlugins`-bound codec — the same path a paste of that
   * text would take, so a fenced `text-attachment`/`excalidraw` block or an
   * `![alt](ref)` image lands as the exact node shape the rendering-side
   * plugins expect.
   */
  insertAttachmentMarkdown(markdown: string): void
  /**
   * Inserts an image pointing at a local `objectUrl` (from
   * `URL.createObjectURL`) — the preview a photo attachment shows
   * immediately, before its upload even starts. Pair with
   * `settlePendingImage` once the upload settles, one way or the other.
   */
  insertPendingImage(objectUrl: string, alt: string): void
  /**
   * Resolves a pending image previously inserted via `insertPendingImage`,
   * found by its own `objectUrl`. `finalMarkdown` is the real `![alt](ref)`
   * the upload resolved to — its `url` replaces the placeholder's in place;
   * `null` means the upload failed, and the placeholder is removed instead.
   */
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
  /** Feeds `createChatPastePlugin`'s `uploadChatAttachment` call for a pasted
   *  image. The production call site (`composer-field.tsx`)
   *  passes them; kept optional here rather than
   *  required so a caller that genuinely can't supply them yet still gets a
   *  working editor — paste interception simply does not register, and a
   *  paste falls through to Slate's own default handling. */
  wsId?: string
  chatId?: string
  placeholder: string
  ariaLabel: string
  /** Fires after every content change; never serializes. `edited` is false for
   *  text loaded through `setMarkdown`/`clear`, true for anything else. */
  onShapeChange: (shape: ComposerShape, edited: boolean) => void
  /**
   * Enter, Tab, arrows, Escape — the chat's own key handling.
   *
   * `readMarkdown` is the box's text AS IT STANDS, not as React last heard it.
   * A key handler installed at render can be called by a keystroke that has not
   * reached state yet, and submitting from state there sends the prompt one
   * character short — or, on the first keystroke, empty.
   */
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

/**
 * The prompt box, as rich markdown.
 *
 * The same editable serves the conversation's pill and the blank chat's
 * document — they differ in type size and in what rides alongside them, not in
 * what you can write. A prompt is markdown either way, and a person who types
 * `- ` should get a list in both.
 *
 * UNCONTROLLED: the text lives in the editor and nowhere else. Callers push text
 * in through the handle and read it out once, on send, so typing serializes
 * nothing and re-renders nothing outside the box.
 */
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
  // THE KEY HANDLER IS A PLUGIN, NOT THE `onKeyDown` DOM PROP.
  //
  // `PlateContent`'s DOM prop runs AFTER Slate's own handling, so preventing the
  // default there is too late for any key Slate already acts on. Enter is
  // exactly that key: the chat sends on it, Slate splits the block on it, and
  // with the prop the split won — three prompts typed in a row stacked up inside
  // the box as three paragraphs and none of them was ever sent. Plugin handlers
  // are piped in ahead of the editor's own behaviour, which is where a handler
  // that says "not this key" has to sit.
  //
  // Read through a ref so the plugin — and therefore the editor — is built once
  // while the handler it calls stays current.
  const keyHandlerRef = useRef(onKeyDown)
  keyHandlerRef.current = onKeyDown
  const keyPlugin = useMemo(
    () =>
      createPlatePlugin({
        key: 'agent-chat-keys',
        handlers: {
          onKeyDown: ({ editor: current, event }) => {
            // Cmd/Ctrl+A selects the WHOLE document ourselves rather than
            // leaving it to WebKit's native `selectAll:` — the same reason
            // Monaco carries its own select-all keybinding (see
            // desktop/src-tauri/src/lib.rs's build_app_menu): a framework-
            // managed contenteditable is not guaranteed to be the element
            // AppKit's editing-command routing actually reaches, and a
            // no-op here costs nothing when the native path would have
            // worked anyway.
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

  // Guards against slate-react's OWN default drop handling, which runs
  // independently of (and BEFORE, in the same synchronous dispatch — see
  // slate-react's `Editable` `onDrop`, which calls `isEventHandled(event,
  // attributes.onDrop)`, i.e. THIS plugin chain, before deciding whether to
  // run its own logic) whatever the app's file-drop handler
  // (`agent-composer.tsx`'s `handlePillDrop`, `agent-empty-document.tsx`'s
  // `handleDrop`) does with the SAME event once it bubbles up to their
  // wrapping `.pill`/`.docwrap` element.
  //
  // Confirmed live (a real `drop` DOM event dispatched at the actual
  // `[data-slate-editor]` node, not a shortcut that calls the insertion
  // function directly): with no guard, slate-react's default `insertData`
  // reads the SAME dropped file's `text/plain` payload and inserts it as a
  // paragraph of raw text — independently of, and in addition to, whatever
  // the app-level handler goes on to upload/insert from the same drop. A
  // dropped CSV that resolves to a table is the visible case (duplicate raw
  // CSV text alongside the real table), but the bug is general to any
  // file-carrying drop that also happens to expose text data.
  //
  // `event.preventDefault()` alone is enough for slate-react to treat the
  // drop as handled (`isEventHandled` checks `event.isDefaultPrevented()`)
  // and skip its own insertion — it does NOT stop propagation, so the app's
  // own `.pill`/`.docwrap` handler still receives and processes the same
  // event exactly as before.
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

  // Undefined without both ids — see the note on `wsId`/`chatId` above. Built
  // once, same as `keyPlugin` above: the editor's plugin list
  // is fixed at mount, not re-derived as props change underneath it.
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
      // NOT `focusAndReport`: this fires whenever the upload settles, which
      // can be long after the user has moved on — typing further, switching
      // chats. Forcing focus back here would yank the caret out from under
      // them the instant a background upload happens to finish.
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
        autoFocus={autoFocus}
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
