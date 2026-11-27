import { useCallback, useMemo } from 'react'
import type { KeyboardEvent } from 'react'
import { LinkPlugin } from '@platejs/link/react'
import type { Value } from 'platejs'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { MARKDOWN_PROSE_CLASS } from '@/features/panes/lib/markdown-prose'
import { cn } from '@/utils/cn'
import { commentEditorKeyAction } from './comment-editor-keys'
import { commentPlugins } from './comment-plugins'
import { commentMarkdownToValue, commentValueToMarkdown } from './comment-serialization'

const COMPOSER_MIN_HEIGHT_PX = 72

interface MarkdownCommentEditorProps {
  initialValue?: string
  placeholder?: string
  autoFocus?: boolean
  onChange: (markdown: string) => void
  onSubmit: (markdown: string) => void
  onCancel: () => void
}

export function MarkdownCommentEditor({
  initialValue = '',
  placeholder,
  autoFocus = true,
  onChange,
  onSubmit,
  onCancel,
}: MarkdownCommentEditorProps) {
  const initial = useMemo(
    () => commentMarkdownToValue(initialValue),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- initial value only, deliberately not re-derived
    [],
  )

  const editor = usePlateEditor({ plugins: commentPlugins, value: initial, autoSelect: 'end' })

  const handleChange = useCallback(() => {
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
      onSubmit(commentValueToMarkdown(editor.children as Value))
    },
    [editor, onCancel, onSubmit],
  )

  return (
    <Plate editor={editor} onChange={handleChange}>
      <div className="relative">
        <PlateContent
          autoFocus={autoFocus}
          placeholder={placeholder}
          onKeyDown={handleKeyDown}
          style={{ minHeight: COMPOSER_MIN_HEIGHT_PX }}
          className={cn(MARKDOWN_PROSE_CLASS, 'outline-none')}
        />
      </div>
    </Plate>
  )
}
