import { useEffect, useRef, useState } from 'react'

interface WorkspaceInlineInputProps {
  defaultValue?: string
  placeholder?: string
  onConfirm: (value: string) => void
  onCancel: () => void
}

export function WorkspaceInlineInput({
  defaultValue = '',
  placeholder = 'branch-name',
  onConfirm,
  onCancel,
}: WorkspaceInlineInputProps) {
  const [value, setValue] = useState(defaultValue)
  const ref = useRef<HTMLInputElement>(null)
  // Prevents blur from double-firing after Enter/Escape already handled
  const handledRef = useRef(false)

  useEffect(() => {
    ref.current?.focus()
    ref.current?.select()
  }, [])

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      handledRef.current = true
      if (value.trim()) onConfirm(value.trim())
      else onCancel()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      handledRef.current = true
      onCancel()
    }
  }

  function handleBlur() {
    if (handledRef.current) return
    if (value.trim()) onConfirm(value.trim())
    else onCancel()
  }

  return (
    <input
      ref={ref}
      type="text"
      value={value}
      onChange={e => setValue(e.target.value)}
      onKeyDown={handleKeyDown}
      onBlur={handleBlur}
      placeholder={placeholder}
      className="min-w-0 flex-1 bg-transparent font-mono text-[13px] outline-none placeholder:text-muted-foreground/40"
    />
  )
}
