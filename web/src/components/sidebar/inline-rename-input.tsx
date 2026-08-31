import { useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'

interface InlineRenameInputProps {
  defaultValue: string
  /** Branch rows draw their label monospace (sidebar-row.tsx); match it so
   *  the editor doesn't change typeface under the cursor. */
  mono?: boolean
  onConfirm: (value: string) => void
  onCancel: () => void
}

/**
 * A row's label, replaced in place by a real, focused, pre-selected
 * `<input>` — this is develop's actual double-click-to-rename behavior
 * (`workspace-inline-input.tsx`, deleted in the tree-retirement commit
 * f119a402, rebuilt fresh here per "no legacy"). Enter confirms, Escape
 * cancels, and blur confirms unless Enter/Escape already handled it —
 * `handledRef` is what stops blur from double-firing after either did.
 */
export function InlineRenameInput({
  defaultValue,
  mono,
  onConfirm,
  onCancel,
}: InlineRenameInputProps) {
  const ref = useRef<HTMLInputElement>(null)
  const handledRef = useRef(false)

  useEffect(() => {
    ref.current?.focus()
    ref.current?.select()
  }, [])

  function tryConfirm(value: string) {
    const trimmed = value.trim()
    if (trimmed) onConfirm(trimmed)
    else onCancel()
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      handledRef.current = true
      tryConfirm(e.currentTarget.value)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      handledRef.current = true
      onCancel()
    }
  }

  function handleBlur(e: React.FocusEvent<HTMLInputElement>) {
    if (handledRef.current) return
    tryConfirm(e.currentTarget.value)
  }

  return (
    <input
      ref={ref}
      type="text"
      defaultValue={defaultValue}
      onKeyDown={handleKeyDown}
      onBlur={handleBlur}
      className={cn(
        'min-w-0 flex-1 truncate bg-transparent text-left text-[13px] font-medium outline-none',
        mono && 'font-mono',
      )}
    />
  )
}
