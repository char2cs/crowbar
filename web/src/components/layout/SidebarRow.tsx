import { cn } from '@/lib/utils'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'

const ROW = 'flex items-center h-9 px-2 mx-1.5 gap-2 rounded-lg my-0.5 cursor-pointer overflow-hidden select-none'

// ── ChatRow ──────────────────────────────────────────────────────────────────

interface ChatRowProps {
  title: string
  age: string
  active?: boolean
  onClick?: () => void
}

export function ChatRow({ title, age, active, onClick }: ChatRowProps) {
  return (
    <div className={cn(ROW, active ? 'bg-accent' : 'hover:bg-accent/50')} onClick={onClick}>
      <div className="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-md border border-border bg-card text-[11px]">
        💬
      </div>
      <span className={cn('flex-1 truncate text-[13px]', active ? 'text-foreground' : 'text-muted-foreground')}>
        {title}
      </span>
      <span className="flex-shrink-0 text-[11px] text-muted-foreground/50">{age}</span>
    </div>
  )
}

// ── RepoRow ───────────────────────────────────────────────────────────────────

interface RepoRowProps {
  name: string
  avatarLabel: string
  avatarColor: string
  collapsed?: boolean
  onClick?: () => void
}

export function RepoRow({ name, avatarLabel, avatarColor, collapsed, onClick }: RepoRowProps) {
  return (
    <div className={cn(ROW, 'hover:bg-accent/50')} onClick={onClick}>
      <Avatar className="h-5 w-5 flex-shrink-0 rounded-md">
        <AvatarFallback className={cn('rounded-md text-[10px] font-bold text-primary-foreground', avatarColor)}>
          {avatarLabel}
        </AvatarFallback>
      </Avatar>
      <span className="text-[10px] flex-shrink-0 text-muted-foreground/50">
        {collapsed ? '›' : '⌄'}
      </span>
      <span className="flex-1 truncate text-[13px] font-medium text-foreground/80">{name}</span>
    </div>
  )
}

// ── WorkspaceRow ──────────────────────────────────────────────────────────────

interface WorkspaceRowProps {
  num?: number
  branch: string
  added?: number
  deleted?: number
  age: string
  active?: boolean
  onClick?: () => void
}

export function WorkspaceRow({ num, branch, added, deleted, age, active, onClick }: WorkspaceRowProps) {
  return (
    <div className={cn(ROW, active ? 'bg-primary/10' : 'hover:bg-accent/50')} onClick={onClick}>
      <span className="w-3 flex-shrink-0 text-right font-mono text-[10px] text-muted-foreground/40">
        {num ?? ''}
      </span>
      <div className="flex min-w-0 flex-1 flex-col justify-center gap-0.5">
        <span className={cn('truncate font-mono text-[12px] leading-tight', active ? 'text-primary' : 'text-muted-foreground')}>
          {branch}
        </span>
        <div className="flex gap-1 text-[10.5px] leading-none text-muted-foreground/40">
          {added !== undefined && <span className="text-green-500">+{added}</span>}
          {deleted !== undefined && <span className="text-red-500">-{deleted}</span>}
          <span>{age}</span>
        </div>
      </div>
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
    <div className={cn(ROW, 'hover:bg-accent/50')} onClick={onClick}>
      <span className="text-[15px] leading-none text-muted-foreground/40">+</span>
      <span className="flex-1 truncate text-[12.5px] text-muted-foreground/40">{label}</span>
    </div>
  )
}
