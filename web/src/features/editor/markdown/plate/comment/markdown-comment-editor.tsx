import { useCallback, useMemo } from 'react'
import type { KeyboardEvent } from 'react'
import { LinkPlugin } from '@platejs/link/react'
// `Value` resolves from `platejs` (re-exported from `@platejs/slate`), not
// from `platejs/react` — the react entrypoint's barrel does not re-export it.
import type { Value } from 'platejs'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { MARKDOWN_PROSE_CLASS } from '@/features/panes/lib/markdown-prose'
import { cn } from '@/utils/cn'
import { commentEditorKeyAction } from './comment-editor-keys'
import { commentPlugins } from './comment-plugins'
import { commentMarkdownToValue, commentValueToMarkdown } from './comment-serialization'

/** Roughly three lines — the room the CodeMirror box this replaces reserved. */
const COMPOSER_MIN_HEIGHT_PX = 72

interface MarkdownCommentEditorProps {
  /** Existing markdown to edit. Read ONCE, at mount. */
  initialValue?: string
  placeholder?: string
  autoFocus?: boolean
  /** Fires with the document's markdown after any content change. */
  onChange: (markdown: string) => void
  /** Cmd/Ctrl+Enter. Receives the CURRENT markdown, not a captured value. */
  onSubmit: (markdown: string) => void
  /** Escape. */
  onCancel: () => void
}

/**
 * The rich markdown editable behind the review comment composer.
 *
 * It replaces a CodeMirror textarea sitting behind a Write/Preview tab pair.
 * That toggle was never a feature — it existed because the input could not
 * render what you typed, so you had to leave the input to find out. Editing
 * the formatted text directly removes the mode rather than improving it, which
 * is why nothing here has a preview: it IS the preview. `MARKDOWN_PROSE_CLASS`
 * is the same class `MarkdownPreview` styles a posted comment with, so the two
 * agree by construction.
 */
export function MarkdownCommentEditor({
  initialValue = '',
  placeholder,
  autoFocus = true,
  onChange,
  onSubmit,
  onCancel,
}: MarkdownCommentEditorProps) {
  // Deserialize ONCE. Re-parsing on every render would rebuild the document
  // under the caret on each keystroke; a comment's text only enters from
  // outside at mount (a fresh draft, or the body of the comment being edited).
  const initial = useMemo(
    () => commentMarkdownToValue(initialValue),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- initial value only, deliberately not re-derived
    [],
  )

  // `autoSelect: 'end'` so editing an existing comment puts the caret after
  // the last word rather than before the first one.
  const editor = usePlateEditor({ plugins: commentPlugins, value: initial, autoSelect: 'end' })

  const handleChange = useCallback(() => {
    // Selection-only events (a click, an arrow key) cannot change the
    // serialized document, so serializing for them is pure waste.
    // Guarded on a NON-EMPTY operation list: an empty list means we cannot
    // tell what happened, and skipping a real edit would drop the user's text.
    const ops = editor.operations
    if (ops.length > 0 && ops.every((op) => op.type === 'set_selection')) return
    onChange(commentValueToMarkdown(editor.children as Value))
  }, [editor, onChange])

  const handleKeyDown = useCallback(
    (event: KeyboardEvent<HTMLDivElement>) => {
      const action = commentEditorKeyAction(event, {
        isLinkEditorOpen: Boolean(editor.getOption(LinkPlugin, 'mode')),
      })
      if (action === null) return
      event.preventDefault()
      if (action === 'cancel') {
        onCancel()
        return
      }
      // Serialized HERE rather than read from a value captured at render: this
      // handler can outlive the render that installed it, and posting a
      // keystroke-old comment would silently drop the last thing typed.
      onSubmit(commentValueToMarkdown(editor.children as Value))
    },
    [editor, onCancel, onSubmit],
  )

  return (
    <Plate editor={editor} onChange={handleChange}>
      {/* `relative` is load-bearing, not layout garnish. The selection toolbar
          renders as a SIBLING of the editable (Plate's `afterEditable`) and
          positions itself `absolute`, so without a positioned ancestor here its
          containing block is whatever distant element happens to be positioned
          — measured live, that put the toolbar ~670px right and ~230px below
          the words it belongs to. The file editor gets this for free from the
          `relative h-full` wrapper it uses for its view toggle. */}
      <div className="relative">
        <PlateContent
          autoFocus={autoFocus}
          placeholder={placeholder}
          onKeyDown={handleKeyDown}
          // A style prop, not a `min-h-*` class: Slate's Editable writes
          // `min-height: 20px` INLINE, which beats any class and collapsed the
          // box to a single line. Slate spreads the caller's style after its
          // own defaults, so this is the one place the value can be set.
          style={{ minHeight: COMPOSER_MIN_HEIGHT_PX }}
          className={cn(MARKDOWN_PROSE_CLASS, 'outline-none')}
        />
      </div>
    </Plate>
  )
}
