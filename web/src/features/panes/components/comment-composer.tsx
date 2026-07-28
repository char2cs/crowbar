import { useState } from 'react'
import { Button } from '@/components/ui/button'
// react-doctor-disable-next-line prefer-dynamic-import -- only reachable through review-thread-item.tsx / use-review-annotations.tsx / use-review-comment-layer.tsx, all already behind the review surfaces' own React.lazy() boundaries (review-diff-tab.tsx). Verified via `bun vite build`: 0 "platejs" occurrences in the entry chunk.
import { MarkdownCommentEditor } from '@/features/editor/markdown/plate/comment/markdown-comment-editor'

interface CommentComposerProps {
  /** Optional header, e.g. "Add a comment on line R7". */
  title?: string
  placeholder?: string
  submitLabel?: string
  autoFocus?: boolean
  /** Seed the editor with existing text — used when editing a comment. */
  initialValue?: string
  onSubmit: (body: string) => void
  onCancel: () => void
}

export function CommentComposer({
  title,
  placeholder = 'Leave a comment',
  submitLabel = 'Comment',
  autoFocus = true,
  initialValue = '',
  onSubmit,
  onCancel,
}: CommentComposerProps) {
  // Mirrors the editor's document as markdown. Its only job is to answer "is
  // there anything to post" — the text that actually gets posted is serialized
  // from the editor at submit time, so this can never be the stale copy.
  const [value, setValue] = useState(initialValue)
  const canSubmit = value.trim().length > 0

  function submit(markdown: string) {
    const body = markdown.trim()
    // Deliberately no clear-on-submit: every call site unmounts this component
    // when the submit succeeds, and on the paths where one doesn't, wiping the
    // box would destroy a comment whose POST had just failed.
    if (body) onSubmit(body)
  }

  return (
    // `ui-font whitespace-normal` here rather than at the call sites: the
    // annotation variant is slotted into the diff's <pre>, so everything below
    // inherits both the CODE font and `white-space: pre` unless it opts out.
    // Prose in a review is prose, whatever it is anchored to — and it must not
    // depend on each caller remembering that. (The editable overrides
    // white-space itself, as an editor should; this is for the chrome.)
    <div className="ui-font whitespace-normal rounded-lg border border-border bg-background">
      {title && (
        <div className="border-b border-border/60 px-3 py-2 text-xs font-semibold text-foreground">
          {title}
        </div>
      )}

      <div className="px-3 py-2">
        <MarkdownCommentEditor
          initialValue={initialValue}
          placeholder={placeholder}
          autoFocus={autoFocus}
          onChange={setValue}
          onSubmit={submit}
          onCancel={onCancel}
        />
      </div>

      <div className="flex justify-end gap-2 border-t border-border/60 px-3 py-2">
        <Button type="button" variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button type="button" size="sm" disabled={!canSubmit} onClick={() => submit(value)}>
          {submitLabel}
        </Button>
      </div>
    </div>
  )
}
