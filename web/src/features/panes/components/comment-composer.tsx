import { useState } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import { markdown } from '@codemirror/lang-markdown'
import { keymap } from '@codemirror/view'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'
import { transparentMarkdownTheme, MarkdownPreview } from '@/features/panes/lib/markdown'

interface CommentComposerProps {
  /** Optional header, e.g. "Add a comment on line R7". */
  title?: string
  placeholder?: string
  submitLabel?: string
  autoFocus?: boolean
  onSubmit: (body: string) => void
  onCancel: () => void
}

export function CommentComposer({
  title,
  placeholder = 'Leave a comment',
  submitLabel = 'Comment',
  autoFocus = true,
  onSubmit,
  onCancel,
}: CommentComposerProps) {
  const [value, setValue] = useState('')
  const [mode, setMode] = useState<'write' | 'preview'>('write')
  const canSubmit = value.trim().length > 0

  function submit() {
    if (!canSubmit) return
    onSubmit(value.trim())
    setValue('')
  }

  // Esc cancels, Cmd/Ctrl+Enter submits.
  const submitKeymap = keymap.of([
    {
      key: 'Mod-Enter',
      run: () => {
        submit()
        return true
      },
    },
    {
      key: 'Escape',
      run: () => {
        onCancel()
        return true
      },
    },
  ])

  return (
    <div className="rounded-lg border border-border bg-background">
      {title && (
        <div className="border-b border-border/60 px-3 py-2 text-xs font-semibold text-foreground">
          {title}
        </div>
      )}

      <div className="flex items-center gap-1 border-b border-border/60 px-2 pt-1.5">
        {(['write', 'preview'] as const).map((m) => (
          <button
            key={m}
            type="button"
            onClick={() => setMode(m)}
            className={cn(
              'rounded-t-md px-2.5 py-1 text-xs font-medium capitalize transition-colors',
              mode === m
                ? 'border border-b-0 border-border bg-muted/40 text-foreground'
                : 'text-muted-foreground hover:text-foreground',
            )}
          >
            {m}
          </button>
        ))}
      </div>

      <div className="px-3 py-2">
        {mode === 'write' ? (
          <CodeMirror
            autoFocus={autoFocus}
            value={value}
            placeholder={placeholder}
            extensions={[markdown(), transparentMarkdownTheme, submitKeymap]}
            onChange={setValue}
            basicSetup={{
              lineNumbers: false,
              foldGutter: false,
              dropCursor: false,
              allowMultipleSelections: false,
              indentOnInput: true,
            }}
            className="min-h-[72px] text-sm"
          />
        ) : value.trim() ? (
          <MarkdownPreview className="min-h-[72px]">{value}</MarkdownPreview>
        ) : (
          <p className="min-h-[72px] text-sm text-muted-foreground/40">Nothing to preview</p>
        )}
      </div>

      <div className="flex justify-end gap-2 border-t border-border/60 px-3 py-2">
        <Button type="button" variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          type="button"
          size="sm"
          disabled={!canSubmit}
          onClick={submit}
          className="text-foreground"
        >
          {submitLabel}
        </Button>
      </div>
    </div>
  )
}
