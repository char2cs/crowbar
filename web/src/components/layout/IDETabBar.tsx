import { X } from 'lucide-react'
import { cn } from '@/utils/cn'

interface IDETabBarProps {
  label: string
  onClose?: () => void
  className?: string
}

/**
 * IDE-style tab bar that renders the active workspace as a document tab,
 * matching the tab paradigm of the broader IDE (Files, Git, etc.).
 */
export function IDETabBar({ label, onClose, className }: IDETabBarProps) {
  return (
    <div
      className={cn(
        'flex h-9 shrink-0 items-end overflow-hidden border-b border-border bg-card',
        className,
      )}
    >
      {/* Active tab — sits flush with the border-b of the bar */}
      <div className="relative flex h-full items-center gap-1.5 border-r border-border bg-background px-3 after:absolute after:bottom-[-1px] after:left-0 after:right-0 after:h-px after:bg-primary">
        <span className="max-w-[220px] truncate text-[12px] text-foreground">{label}</span>
        {onClose && (
          <button
            onClick={onClose}
            aria-label="Close tab"
            className="flex h-4 w-4 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            <X size={11} />
          </button>
        )}
      </div>
      {/* Unfilled tab rail */}
      <div className="flex-1" />
    </div>
  )
}
