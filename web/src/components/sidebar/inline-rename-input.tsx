import { useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'

interface InlineRenameInputProps {
  defaultValue: string
  /** The face of the label it replaces — a branch row's monospace, a project row's heading —
   *  so the editor doesn't change typeface under the cursor. */
  font?: 'mono' | 'heading'
  /** Shown only while the value is empty, so in effect only for a create: an empty row would
   *  otherwise give no hint of what it wants (see `CREATE_ROW_PLACEHOLDER`). */
  placeholder?: string
  onConfirm: (value: string) => void
  onCancel: () => void
}

/** A row's label replaced in place by a focused, pre-selected input. Enter confirms, Escape
 *  cancels, blur confirms unless either already did (`handledRef` stops the double fire). */
export function InlineRenameInput({
  defaultValue,
  font,
  placeholder,
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
    // The editor may mount away from the gesture (a Recents row renames its tree copy; the
    // context menu can rename a row scrolled away): focus() alone can leave it off-screen.
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
        font === 'mono' && 'font-mono',
        font === 'heading' && 'font-heading text-[18px] font-normal',
      )}
    />
  )
}
