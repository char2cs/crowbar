import {
  CaretDown as ChevronDown,
  CaretRight as ChevronRight,
  type Icon as PhosphorIcon,
} from '@phosphor-icons/react'
import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { cn } from '@/utils/cn'

const sectionHeaderClassName =
  'flex min-h-7 shrink-0 items-center justify-between gap-1.5 rounded-none bg-muted px-2.5 py-1'
const sectionGroupClassName = 'flex items-center gap-1'
const sectionTitleClassName = 'text-sm font-medium text-foreground'

interface GitSidebarSectionHeaderProps {
  title: string
  actions?: ReactNode
  collapsible?: boolean
  isCollapsed?: boolean
  onToggle?: () => void
  icon?: PhosphorIcon
  className?: string
}

const GitSidebarSectionHeader = ({
  title,
  actions,
  collapsible = false,
  isCollapsed = false,
  onToggle,
  icon: Icon,
  className,
}: GitSidebarSectionHeaderProps) => {
  const Caret = isCollapsed ? ChevronRight : ChevronDown

  const content = (
    <>
      <div className={cn(sectionGroupClassName, 'min-w-0 flex-1')}>
        {collapsible && <Caret className="size-3.5 shrink-0 text-muted-foreground" />}
        {Icon ? <Icon className="size-3.5 shrink-0 text-muted-foreground" /> : null}
        <span className={sectionTitleClassName}>{title}</span>
      </div>
      {actions ? <div className={cn(sectionGroupClassName, 'shrink-0')}>{actions}</div> : null}
    </>
  )

  if (collapsible) {
    return (
      <Button
        type="button"
        variant="ghost"
        onClick={onToggle}
        className={cn(sectionHeaderClassName, 'w-full hover:bg-muted', className)}
        compact
      >
        {content}
      </Button>
    )
  }

  return <div className={cn(sectionHeaderClassName, className)}>{content}</div>
}

export const gitSidebarSectionActionButtonClassName = (className?: string) =>
  cn('size-6 shrink-0 rounded-lg text-muted-foreground', className)

export default GitSidebarSectionHeader
