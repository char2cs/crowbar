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
  onConfirm: (value: string) => void
  onCancel: () => void
}

// The row's label replaced in place by a focused, pre-selected input. Enter confirms, Escape
// cancels, blur confirms — `handledRef` keeps blur from firing again after Enter/Escape.
export function InlineRenameInput({
  defaultValue,
  mono,
  placeholder,
  className,
  onConfirm,
  onCancel,
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
      placeholder={placeholder}
      onKeyDown={handleKeyDown}
      onBlur={handleBlur}
      className={cn(
        'min-w-0 flex-1 truncate bg-transparent text-left text-[13px] font-medium outline-none',
        mono && 'font-mono',
        className,
      )}
    />
  )
}
