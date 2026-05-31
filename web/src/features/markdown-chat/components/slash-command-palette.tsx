import { useState, useEffect, useRef } from 'react'

export interface SlashCommand {
  id: string
  label: string
  description: string
  icon?: string
}

// The real list — extend as new skills are added.
export const SLASH_COMMANDS: SlashCommand[] = [
  { id: '/tdd', label: '/tdd', description: 'Test-driven development workflow', icon: '🧪' },
  { id: '/code-review', label: '/code-review', description: 'Review current branch', icon: '🔍' },
  { id: '/plan', label: '/plan', description: 'Write an implementation plan', icon: '📋' },
  { id: '/debug', label: '/debug', description: 'Systematic debugging', icon: '🐛' },
  { id: '/explain', label: '/explain', description: 'Explain selected code', icon: '💬' },
]

interface SlashCommandPaletteProps {
  query: string
  onSelect: (command: SlashCommand) => void
  onClose: () => void
  anchorRect: DOMRect
}

export function SlashCommandPalette({ query, onSelect, onClose, anchorRect }: SlashCommandPaletteProps) {
  const [activeIdx, setActiveIdx] = useState(0)
  const filtered = SLASH_COMMANDS.filter(
    (c) => c.label.toLowerCase().includes(query.toLowerCase()),
  )

  const ref = useRef<HTMLDivElement>(null)

  // Reset active index when filter changes
  useEffect(() => { setActiveIdx(0) }, [query])

  // Keyboard navigation
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'ArrowDown') { e.preventDefault(); setActiveIdx((i) => Math.min(i + 1, filtered.length - 1)) }
      if (e.key === 'ArrowUp') { e.preventDefault(); setActiveIdx((i) => Math.max(i - 1, 0)) }
      if (e.key === 'Enter') { e.preventDefault(); if (filtered[activeIdx]) onSelect(filtered[activeIdx]) }
      if (e.key === 'Escape') { e.preventDefault(); onClose() }
    }
    window.addEventListener('keydown', handler, true)
    return () => window.removeEventListener('keydown', handler, true)
  }, [filtered, activeIdx, onSelect, onClose])

  if (filtered.length === 0) return null

  return (
    <div
      ref={ref}
      className="fixed z-50 w-72 rounded-lg border border-border bg-popover shadow-lg"
      style={{ top: anchorRect.top - 8, left: anchorRect.left, transform: 'translateY(-100%)' }}
    >
      {filtered.map((cmd, i) => (
        <button
          key={cmd.id}
          className={`flex w-full items-start gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-muted ${i === activeIdx ? 'bg-muted' : ''}`}
          onClick={() => onSelect(cmd)}
        >
          {cmd.icon && <span className="mt-0.5 text-base leading-none">{cmd.icon}</span>}
          <div>
            <div className="font-mono font-medium text-foreground">{cmd.label}</div>
            <div className="text-xs text-muted-foreground">{cmd.description}</div>
          </div>
        </button>
      ))}
    </div>
  )
}
