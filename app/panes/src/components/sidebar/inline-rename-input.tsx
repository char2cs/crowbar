import { useEffect, useRef } from 'react'
import { cn } from '@/lib/utils'

interface InlineRenameInputProps {
  defaultValue: string
  mono?: boolean
  placeholder?: string
  className?: string
  onConfirm: (value: string) => void | Promise<void>
  onCancel: () => void
  cancelOnBlur?: boolean
  errorId?: string
}

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
