import { useState } from 'react'
import { Button } from '@/components/ui/button'
// react-doctor-disable-next-line prefer-dynamic-import -- only reachable through review-thread-item.tsx / use-review-annotations.tsx / use-review-comment-layer.tsx, all already behind the review surfaces' own React.lazy() boundaries (review-diff-tab.tsx). Verified via `bun vite build`: 0 "platejs" occurrences in the entry chunk.
import { MarkdownCommentEditor } from '@/features/editor/markdown/plate/comment/markdown-comment-editor'

interface CommentComposerProps {
  title?: string
  placeholder?: string
  submitLabel?: string
  autoFocus?: boolean
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
  const [value, setValue] = useState(initialValue)
  const canSubmit = value.trim().length > 0

  function submit(markdown: string) {
    const body = markdown.trim()
    if (body) onSubmit(body)
  }

  return (
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
