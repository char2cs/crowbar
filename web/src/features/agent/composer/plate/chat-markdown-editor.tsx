import { useCallback, useImperativeHandle, useMemo, useRef } from 'react'
import type { CSSProperties, KeyboardEvent, Ref } from 'react'
import {
  NodeApi,
  PathApi,
  PointApi,
  RangeApi,
  type Path,
  type Point,
  type TRange,
  type Value,
} from 'platejs'
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

/** The path of an EMPTY paragraph ancestor at `at`, or null — used instead of
 *  the point itself so `insertNodes` inserts at that array index (pushing
 *  the still-empty paragraph after the new content) rather than splitting
 *  the paragraph at a text offset, which would leave two empty halves
 *  straddling the new content instead of one trailing one. Real (non-empty)
 *  text at `at` returns null, unchanged from before this existed: attaching
 *  something mid-sentence still inserts inline at that exact point. */
function emptyParagraphPathAt(
  editor: PlateEditor,
  at: Point | Path | TRange | undefined,
): Path | null {
  if (!at) return null
  const entry = editor.api.above({ at, match: { type: 'p' } })
  if (!entry) return null
  const [node, path] = entry
  return NodeApi.string(node) === '' ? path : null
}

/** After inserting an attachment, lands the caret on a fresh, empty line
 *  right after it — reported live: there was nowhere to keep typing except
 *  clicking below the block yourself. A no-op beyond moving the caret if the
 *  document's last node is ALREADY an empty paragraph (either the ordinary
 *  empty draft an attachment was just inserted into, or the same trailing
 *  line a previous attachment already made) — insertAttachmentMarkdownInto's
 *  own `emptyParagraphPathAt` redirect is what keeps that ONE paragraph
 *  trailing rather than stacking a new one behind each attachment. */
function ensureTrailingEditableLine(editor: PlateEditor): void {
  const last = editor.children.at(-1)
  const alreadyEmpty = !!last && last.type === 'p' && NodeApi.string(last) === ''
  if (!alreadyEmpty) {
    editor.tf.insertNodes([{ type: 'p', children: [{ text: '' }] }], {
      at: [editor.children.length],
    })
  }
  const end = editor.api.end([])
  if (end) editor.tf.select(end)
}

/**
 * Where an attachment inserted from OUTSIDE the editable lands, and the
 * insert itself. Split out from focusing the box and reporting the change
 * (the imperative handle below still does both, right after calling this)
 * so the one real branch here — was there already a selection to insert at,
 * or not — can be proven against a bare `createPlateEditor`, with no
 * mounted DOM: `editor.tf.focus()` throws without one.
 *
 * Fenced blocks get their own branch: inserted right after its `code_block`
 * path rather than at the raw point, or it splits the fence's OWN internals
 * and silently drops the new node's content instead of appending it as a
 * sibling (this is what two attachments inserted back-to-back, still inside
 * the first one's own structure, would otherwise hit). Detected specifically
 * via `CodeBlockPlugin` — NOT a blanket `mode: 'highest'`, which "fixes" this
 * by always walking to the top-level block, and in doing so splits a `table`
 * in two (header row separated from body) for the unrelated, previously-fine
 * case of a caret sitting inside a table cell.
 *
 * Lands the caret on a fresh trailing empty line afterward — see
 * `ensureTrailingEditableLine` — so the person can keep typing immediately.
 */
function attachmentInsertionPath(editor: PlateEditor): Path | Point | TRange | undefined {
  const at = editor.selection ?? editor.api.end([])
  const codeBlock = editor.api.above({ at, match: { type: CodeBlockPlugin.key } })
  return codeBlock ? PathApi.next(codeBlock[1]) : (emptyParagraphPathAt(editor, at) ?? at)
}

export function insertAttachmentMarkdownInto(editor: PlateEditor, markdown: string): void {
  const nodes = chatMarkdownToValue(markdown)
  editor.tf.insertNodes(nodes, { at: attachmentInsertionPath(editor) })
  ensureTrailingEditableLine(editor)
}

/**
 * Inserts an image node pointing at a LOCAL, temporary `objectUrl` (from
 * `URL.createObjectURL`) — the optimistic half of an attachment upload: the
 * preview appears immediately, before the network round trip that produces
 * the real, persistable ref even starts. Built as a raw node rather than
 * routed through `insertAttachmentMarkdownInto`'s markdown codec so a
 * filename with `]`/`)` in it can never be misparsed as markdown syntax —
 * this bypasses markdown entirely, exactly once, for exactly this node.
 * `settlePendingImageInto` (below) is what replaces it once the upload
 * settles, one way or the other.
 */
export function insertPendingImageInto(editor: PlateEditor, objectUrl: string, alt: string): void {
  const node = {
    type: 'img',
    url: objectUrl,
    caption: [{ text: alt }],
    children: [{ text: '' }],
  } as unknown as Value[number]
  editor.tf.insertNodes([node], { at: attachmentInsertionPath(editor) })
  ensureTrailingEditableLine(editor)
}

/**
 * Resolves a pending image previously inserted by `insertPendingImageInto`,
 * found by its own `objectUrl` (unique for the life of that blob, never
 * reused) — NOT by position, since the person may have kept typing or
 * reordered attachments while the upload was in flight.
 *
 * `finalMarkdown` is the real `![alt](ref)` `uploadAttachmentMarkdown`
 * resolved to: its `url` replaces the placeholder's IN PLACE, preserving
 * whatever position the placeholder ended up in. `null` means the upload
 * failed — the placeholder is removed instead, matching what happened
 * before this existed (a failed upload never left anything behind either).
 * Either way the object URL is revoked: once swapped, the browser holds the
 * real image; once removed, there is nothing left to preview.
 */
export function settlePendingImageInto(
  editor: PlateEditor,
  objectUrl: string,
  finalMarkdown: string | null,
): void {
  const [entry] = Array.from(
    editor.api.nodes({
      at: [],
      match: (n) =>
        (n as { type?: string }).type === 'img' && (n as { url?: string }).url === objectUrl,
    }),
  )
  if (entry) {
    const [, path] = entry
    if (finalMarkdown === null) {
      editor.tf.removeNodes({ at: path })
    } else {
      const nodes = chatMarkdownToValue(finalMarkdown)
      const [finalNode] = nodes as unknown as { url?: string }[]
      if (finalNode?.url) {
        editor.tf.setNodes({ url: finalNode.url }, { at: path })
      } else {
        // The upload resolved to something other than an image — the
        // server's own sniffed content type disagreed with the browser's
        // guess that put this on the optimistic image path in the first
        // place (uploadAttachmentMarkdown fell back to fileMarkdown, which
        // deserializes to a link nested in a wrapping paragraph, not a
        // top-level node with its own `.url`). Patching `url` in place
        // would leave a void `img` node pointing at a non-image file —
        // replace the placeholder with whatever it actually resolved to.
        editor.tf.withoutNormalizing(() => {
          editor.tf.removeNodes({ at: path })
          editor.tf.insertNodes(nodes, { at: path })
        })
      }
    }
  }
  URL.revokeObjectURL(objectUrl)
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

  // Back to the box, and reported synchronously: every handle method below
  // mutates the document from OUTSIDE it (a modal, a drop handler, an
  // upload's own async settle), and the whole point is to keep writing right
  // after. `Plate`'s own `onChange` prop fires off the editor's async change
  // notification, which nothing here waits on — a caller reading `onChange`'s
  // last call right after one of these returns (Task 29/34 close their modal
  // on it) would see the PREVIOUS markdown otherwise.
  const report = useCallback(() => {
    onChange(chatValueToMarkdown(editor.children as Value))
  }, [editor, onChange])
  const focusAndReport = useCallback(() => {
    editor.tf.focus()
    report()
  }, [editor, report])

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
    }),
    [editor, focusAndReport, report],
  )

  // The editable's height, for whatever rides its last line. Observed rather
  // than derived from the text: a wrapped line and a typed newline are the same
  // thing to a reader, and only the browser knows where the wrap fell.
  //
  // A REF CALLBACK on the editable itself, not a `useLayoutEffect` querying
  // for `[data-slate-editor]` inside a wrapper ref (the previous approach) —
  // that version bound the observer's lifetime to THIS component's own
  // effect dependencies (`[onHeightChange]`, stable), so it only ever
  // reconnected when this component genuinely unmounted and remounted.
  // Slate/slate-react can tear down and recreate the editable's OWN DOM node
  // — e.g. when `editor` itself changes identity — without this component
  // ever unmounting, and the previous version had no way to notice: the
  // observer kept watching a node that had already been detached, silently,
  // forever (reported live, repeatedly, as the pill's rounding going stale
  // until a full page reload — a dev-only Fast-Refresh module swap is the
  // other way this exact gap shows up). A ref callback has no such blind
  // spot: React calls it on every attach AND detach of THIS SPECIFIC node,
  // whatever caused it, so a fresh node always gets a fresh observer.
  //
  // Still fires synchronously during commit — the same phase a layout effect
  // runs in, before paint — so a box that mounts (or grows) already
  // multi-line still gets the right radius in its very first visible frame,
  // never one frame of the fully-round single-line pill stretched tall.
  const heightObserverRef = useRef<ResizeObserver | null>(null)
  const reportHeightFrom = useCallback(
    (node: HTMLDivElement | null) => {
      heightObserverRef.current?.disconnect()
      heightObserverRef.current = null
      if (!node || !onHeightChange) return
      const report = () => onHeightChange(node.getBoundingClientRect().height)
      report()
      const observer = new ResizeObserver(report)
      // react-doctor-disable-next-line effect-needs-cleanup -- cleanup exists (l.481: disconnect() at the top of this same callback, which also runs on every detach since React calls a ref callback with node=null then); tracer expects a useEffect return, not a ref-callback's own next invocation.
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
