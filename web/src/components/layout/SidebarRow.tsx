import { cn } from '@/lib/utils'

const ROW = 'flex items-center h-9 px-2 mx-1.5 gap-2 rounded-lg my-0.5 cursor-pointer overflow-hidden select-none'

// ── ChatRow ──────────────────────────────────────────────────────────────────

interface ChatRowProps {
  title: string
  age: string
  active?: boolean
  onClick?: () => void
  onDelete?: () => void
}

export function ChatRow({ title, age, active, onClick, onDelete }: ChatRowProps) {
  return (
    <div className={cn(ROW, 'group', active ? 'bg-accent' : 'hover:bg-accent/50')} onClick={onClick} role="button" tabIndex={0} onKeyDown={onClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick() } } : undefined}>
      <div aria-hidden="true" className="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-md border border-border bg-card text-[11px]">
        💬
      </div>
      <span className={cn('flex-1 truncate text-[13px]', active ? 'text-foreground' : 'text-muted-foreground')}>
        {title}
      </span>
      <span className={cn('flex-shrink-0 text-[11px] text-muted-foreground/50', onDelete && 'group-hover:hidden')}>{age}</span>
      {onDelete && (
        <button
          className="hidden group-hover:flex h-4 w-4 flex-shrink-0 items-center justify-center rounded text-muted-foreground/40 hover:text-muted-foreground"
          onClick={(e) => { e.stopPropagation(); onDelete() }}
          aria-label="Delete chat"
        >×</button>
      )}
    </div>
  )
}

// ── NewRow ────────────────────────────────────────────────────────────────────

interface NewRowProps {
  label: string
  onClick?: () => void
}

export function NewRow({ label, onClick }: NewRowProps) {
  return (
    <div className={cn(ROW, 'hover:bg-accent/50')} onClick={onClick} role="button" tabIndex={0} onKeyDown={onClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick() } } : undefined}>
      <span className="text-[15px] leading-none text-muted-foreground/40">+</span>
      <span className="flex-1 truncate text-[12.5px] text-muted-foreground/40">{label}</span>
    </div>
  )
}
