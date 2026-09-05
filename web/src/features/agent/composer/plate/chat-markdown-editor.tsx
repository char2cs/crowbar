import { useCallback, useImperativeHandle, useLayoutEffect, useMemo, useRef } from 'react'
import type { CSSProperties, KeyboardEvent, Ref } from 'react'
import { PathApi, PointApi, RangeApi, type Value } from 'platejs'
import type { PlateEditor } from 'platejs/react'
import { createPlatePlugin, Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { CodeBlockPlugin } from '@platejs/code-block/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
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
}

export interface ChatMarkdownEditorProps {
  /** Feeds `createChatPastePlugin`'s `uploadChatAttachment` call for a pasted
   *  image. Both production call sites (`composer-field.tsx`,
   *  `agent-empty-document.tsx`) pass them; kept optional here rather than
   *  required so a caller that genuinely can't supply them yet still gets a
   *  working editor — paste interception simply does not register, and a
   *  paste falls through to Slate's own default handling. */
  wsId?: string
  chatId?: string
  /** Markdown to open with. Read ONCE, at mount — see the note on remounting. */
  initialValue: string
  placeholder: string
  ariaLabel: string
  /** Fires with the document's markdown after any content change. */
  onChange: (markdown: string) => void
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
 * Where an attachment inserted from OUTSIDE the editable lands, and the
 * insert itself. Split out from focusing the box and reporting the change
 * (the imperative handle below still does both, right after calling this)
 * so the one real branch here — was there already a selection to insert at,
 * or not — can be proven against a bare `createPlateEditor`, with no
 * mounted DOM: `editor.tf.focus()` throws without one.
 *
 * Fenced blocks get their own branch: after inserting one with `select:
 * true`, the selection Slate leaves sits inside its `code_line`, not after
 * the `code_block`. A second back-to-back insert landing there needs to
 * target the `code_block` itself, or it splits the fence's OWN internals and
 * silently drops the new node's content instead of appending it as a
 * sibling (this is what the excalidraw modal's fence-then-image save hits).
 * Detected specifically via `CodeBlockPlugin` and inserted right after that
 * block's own path — NOT a blanket `mode: 'highest'`, which "fixes" this by
 * always walking to the top-level block, and in doing so splits a `table`
 * in two (header row separated from body) for the unrelated, previously-fine
 * case of a caret sitting inside a table cell.
 */
export function insertAttachmentMarkdownInto(editor: PlateEditor, markdown: string): void {
  const nodes = chatMarkdownToValue(markdown)
  const at = editor.selection ?? editor.api.end([])
  const codeBlock = editor.api.above({ at, match: { type: CodeBlockPlugin.key } })
  if (codeBlock) {
    editor.tf.insertNodes(nodes, { at: PathApi.next(codeBlock[1]), select: true })
    return
  }
  editor.tf.insertNodes(nodes, { at, select: true })
}

/**
 * The prompt box, as rich markdown.
 *
 * The same editable serves the conversation's pill and the blank chat's
 * document — they differ in type size and in what rides alongside them, not in
 * what you can write. A prompt is markdown either way, and a person who types
 * `- ` should get a list in both.
 *
 * UNCONTROLLED, and the caller drives text in by REMOUNTING (a `key` change)
 * rather than by prop. A controlled contenteditable rebuilds its own children
 * under the selection on every keystroke and puts the caret back at zero; the
 * one thing the chat needs to push in from outside — the text of a queued
 * prompt being edited, or a skill the picker inserted — is exactly the case
 * where losing the caret is fine, because the caret should land at the end of
 * what was just loaded.
 */
export function ChatMarkdownEditor({
  wsId,
  chatId,
  initialValue,
  placeholder,
  ariaLabel,
  onChange,
  onKeyDown,
  onHeightChange,
  autoFocus,
  expanded,
  controls,
  className,
  style,
  ref,
}: ChatMarkdownEditorProps) {
  const hostRef = useRef<HTMLDivElement>(null)

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

  // Deserialize ONCE. Re-parsing per render would rebuild the document under
  // the caret on every keystroke.
  const initial = useMemo(
    () => chatMarkdownToValue(initialValue),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount value only, deliberately not re-derived
    [],
  )

  // Undefined without both ids — see the note on `wsId`/`chatId` above. Built
  // once, same as `keyPlugin` and `initial` below: the editor's plugin list
  // is fixed at mount, not re-derived as props change underneath it.
  const pastePlugin = useMemo(
    () => (wsId && chatId ? createChatPastePlugin({ wsId, chatId }) : undefined),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- mount value only, deliberately not re-derived
    [],
  )

  const editor = usePlateEditor({
    plugins: pastePlugin
      ? [...chatComposerPlugins, keyPlugin, dropGuardPlugin, pastePlugin]
      : [...chatComposerPlugins, keyPlugin, dropGuardPlugin],
    value: initial,
    autoSelect: 'end',
  })

  const handleChange = useCallback(() => {
    // A click or an arrow key cannot change the serialized document, so
    // serializing for them is pure waste. Guarded on a NON-EMPTY operation
    // list: an empty list means we cannot tell what happened, and skipping a
    // real edit would silently drop what was typed.
    const ops = editor.operations
    if (ops.length > 0 && ops.every((op) => op.type === 'set_selection')) return
    onChange(chatValueToMarkdown(editor.children as Value))
  }, [editor, onChange])

  useImperativeHandle(
    ref,
    () => ({
      insertAttachmentMarkdown: (markdown: string) => {
        insertAttachmentMarkdownInto(editor, markdown)
        // Back to the box: the insert was dispatched from outside it (a
        // modal, a drop handler), and the whole point is to keep writing.
        editor.tf.focus()
        // `Plate`'s own `onChange` prop fires off the editor's async change
        // notification, which nothing here waits on — a caller reading
        // `onChange`'s last call right after this returns (Task 29/34 close
        // their modal on it) would see the PREVIOUS markdown. Reported
        // synchronously instead, same as `handleChange` computes it.
        onChange(chatValueToMarkdown(editor.children as Value))
      },
    }),
    [editor, onChange],
  )

  // The editable's height, for whatever rides its last line. Observed rather
  // than derived from the text: a wrapped line and a typed newline are the same
  // thing to a reader, and only the browser knows where the wrap fell.
  //
  // LAYOUT effect, not a plain one: a plain `useEffect` runs AFTER the browser
  // paints, so a box that mounts (or grows) already multi-line — a recalled or
  // recovered draft, a large paste — painted its real, already-tall DOM height
  // for at least one real frame BEFORE this got a chance to report it and flip
  // `.pill` to `.multi` (composer.css). That frame is the pill's fully-round
  // single-line radius stretched over box the height of many lines — reported
  // live as "the input box's corners are wrong on a big message". A layout
  // effect runs synchronously before paint, so the height (and therefore the
  // right radius) is correct in the very first frame the box is visible in.
  useLayoutEffect(() => {
    const host = hostRef.current
    const editable = host?.querySelector<HTMLElement>('[data-slate-editor]')
    if (!editable || !onHeightChange) return
    const report = () => onHeightChange(editable.getBoundingClientRect().height)
    report()
    const observer = new ResizeObserver(report)
    observer.observe(editable)
    return () => observer.disconnect()
  }, [onHeightChange])

  return (
    <Plate editor={editor} onChange={handleChange}>
      <div ref={hostRef} className="contents">
        <PlateContent
          autoFocus={autoFocus}
          placeholder={placeholder}
          aria-label={ariaLabel}
          aria-expanded={expanded}
          aria-controls={controls}
          className={cn('field', className)}
          style={style}
        />
      </div>
    </Plate>
  )
}
