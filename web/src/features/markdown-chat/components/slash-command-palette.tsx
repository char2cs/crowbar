import { type CSSProperties, useEffect, useMemo, useState } from 'react'
import type { SlashCommand } from '../types'

// Re-export so existing importers keep working.
export type { SlashCommand }

interface SlashCommandPaletteProps {
  /** Provider-supplied commands. We don't own this list. */
  commands: SlashCommand[]
  /** Text typed after the `/` in the editor — filters the list. */
  query: string
  onSelect: (command: SlashCommand) => void
  onClose: () => void
  anchorRect: DOMRect
}

export function SlashCommandPalette({
  commands,
  query,
  onSelect,
  onClose,
  anchorRect,
}: SlashCommandPaletteProps) {
  const [activeIdx, setActiveIdx] = useState(0)

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return commands
    return commands.filter((c) => c.label.toLowerCase().includes(q))
  }, [commands, query])

  useEffect(() => {
    setActiveIdx(0)
  }, [query])

  // Keyboard nav stays in the editor — the user keeps typing the `/query` there.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setActiveIdx((i) => Math.min(i + 1, filtered.length - 1))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setActiveIdx((i) => Math.max(i - 1, 0))
      } else if (e.key === 'Enter') {
        e.preventDefault()
        if (filtered[activeIdx]) onSelect(filtered[activeIdx])
      } else if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    }
    window.addEventListener('keydown', handler, true)
    return () => window.removeEventListener('keydown', handler, true)
  }, [filtered, activeIdx, onSelect, onClose])

  if (filtered.length === 0) return null

  const style: CSSProperties = {
    position: 'fixed',
    top: anchorRect.top - 8,
    left: anchorRect.left,
    transform: 'translateY(-100%)',
    zIndex: 50,
  }

  return (
    <div
      style={style}
      className="w-56 overflow-hidden rounded-lg border bg-popover p-1 text-popover-foreground shadow-md ring-1 ring-foreground/10"
    >
      {filtered.map((cmd, i) => (
        <button
          key={cmd.id}
          type="button"
          // onMouseDown (not onClick) so the editor doesn't blur before select fires.
          onMouseDown={(e) => {
            e.preventDefault()
            onSelect(cmd)
          }}
          onMouseEnter={() => setActiveIdx(i)}
          className={`flex w-full items-center rounded-md px-2 py-1.5 text-left font-mono text-sm outline-hidden transition-colors ${
            i === activeIdx ? 'bg-accent text-accent-foreground' : 'text-foreground'
          }`}
        >
          {cmd.label}
        </button>
      ))}
    </div>
  )
}
