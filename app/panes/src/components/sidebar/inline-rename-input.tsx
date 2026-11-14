import { useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'

interface InlineRenameInputProps {
  defaultValue: string
  /** Branch rows draw their label monospace (sidebar-row.tsx); match it so
   *  the editor doesn't change typeface under the cursor. */
  mono?: boolean
  /** Shows only while the value is empty, so it hints a create; a rename seeds a real name. */
  placeholder?: string
  /** Replaces the row-label text style (size, weight, family). */
  className?: string
  /** A promise keeps the input open until it settles; a rejection leaves it open to edit again. */
  onConfirm: (value: string) => void | Promise<void>
  onCancel: () => void
  /** Leaving the input drops the edit instead of confirming it. */
  cancelOnBlur?: boolean
  /** The id of the message saying why the last value was refused. */
  errorId?: string
}

// The row's label replaced in place by a focused, pre-selected input. Enter confirms, Escape
// cancels, blur confirms (or cancels) — `handledRef` keeps blur and a second Enter from firing
// again after Enter/Escape, until a refused confirm hands the input back.
export function InlineRenameInput({
  defaultValue,
  mono,
  placeholder,
  className,
  onConfirm,
  onCancel,
  cancelOnBlur,
  errorId,
}: InlineRenameInputProps) {
  const ref = useRef<HTMLInputElement>(null)
  const handledRef = useRef(false)

  useEffect(() => {
    const input = ref.current
    if (!input) return
    input.focus()
    input.select()
    // The editor may mount away from the gesture (a Recents rename draws on the tree copy, or a
    // row scrolled out of view), and focus() alone leaves the caret off-screen.
    input.scrollIntoView({ block: 'nearest' })
  }, [])

  function tryConfirm(value: string) {
    const trimmed = value.trim()
    if (!trimmed) return onCancel()
    const settled = onConfirm(trimmed)
    if (!settled) return
    void settled.catch(() => {
      handledRef.current = false
      ref.current?.focus()
    })
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      if (handledRef.current) return
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
    if (cancelOnBlur) onCancel()
    else tryConfirm(e.currentTarget.value)
  }

  return (
    <input
      ref={ref}
      type="text"
      defaultValue={defaultValue}
      placeholder={placeholder}
      onKeyDown={handleKeyDown}
      onBlur={handleBlur}
      aria-invalid={errorId ? true : undefined}
      aria-describedby={errorId}
      className={cn(
        'min-w-0 flex-1 truncate bg-transparent text-left text-[13px] font-medium outline-none',
        mono && 'font-mono',
        className,
      )}
    />
  )
}
