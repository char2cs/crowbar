import { useEffect, useRef, useState } from 'react'
import { cn } from '@/utils/cn'

interface WorkspaceInlineInputProps {
  defaultValue?: string
  placeholder?: string
  /**
   * What is being typed. `identifier` (the default) is for values git has to
   * accept verbatim — branch names — where a monospace face makes hyphens,
   * slashes and l/1/I legible. `prose` is for free text like a chat title,
   * which should read in the same face as the row it replaces.
   */
  kind?: 'identifier' | 'prose'
  onConfirm: (value: string) => void
  onCancel: () => void
  /** Resolve a branch to the id of the workspace already holding it, or null. */
  resolveExisting?: (branch: string) => string | null
  /** Navigate to the existing workspace when the user clicks the hint. */
  onOpenExisting?: (wsId: string) => void
}

export function WorkspaceInlineInput({
  defaultValue = '',
  placeholder = 'branch-name',
  kind = 'identifier',
  onConfirm,
  onCancel,
  resolveExisting,
  onOpenExisting,
}: WorkspaceInlineInputProps) {
  const [value, setValue] = useState(defaultValue)
  const ref = useRef<HTMLInputElement>(null)
  // Prevents blur from double-firing after Enter/Escape already handled
  const handledRef = useRef(false)

  useEffect(() => {
    ref.current?.focus()
    ref.current?.select()
  }, [])

  const existingWsId = resolveExisting?.(value) ?? null

  function tryConfirm() {
    // A collision suppresses create — the user opens the existing one or renames.
    if (existingWsId) return
    if (value.trim()) onConfirm(value.trim())
    else onCancel()
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      handledRef.current = true
      tryConfirm()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      handledRef.current = true
      onCancel()
    }
  }

  function handleBlur() {
    if (handledRef.current) return
    tryConfirm()
  }

  return (
    <div className="flex min-w-0 flex-1 flex-col">
      <input
        ref={ref}
        type="text"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={handleBlur}
        placeholder={placeholder}
        className={cn(
          'min-w-0 flex-1 bg-transparent text-[13px] outline-none placeholder:text-muted-foreground/40',
          kind === 'identifier' && 'font-mono',
        )}
      />
      {existingWsId && (
        <button
          type="button"
          // Use mousedown so it fires before the input's blur cancels the create.
          onMouseDown={(e) => {
            e.preventDefault()
            handledRef.current = true
            onOpenExisting?.(existingWsId)
          }}
          className="mt-0.5 text-left font-mono text-[11px] text-muted-foreground/70 hover:text-foreground"
        >
          {`'${value.trim()}' already has a workspace — open it`}
        </button>
      )}
    </div>
  )
}
