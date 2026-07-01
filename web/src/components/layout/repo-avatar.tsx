import { cn } from '@/lib/utils'

export type RepoAvatarData = { url?: string; label: string; color: string }

const sizeClasses = {
  sm: { box: 'h-4 w-4', text: 'text-[10px]', emoji: 'text-xs' },
  lg: { box: 'h-5 w-5', text: 'text-[11px]', emoji: 'text-sm' },
}

export function RepoAvatar({
  avatar,
  name,
  size = 'sm',
}: {
  avatar: RepoAvatarData
  name: string
  size?: 'sm' | 'lg'
}) {
  const { box, text, emoji } = sizeClasses[size]
  if (avatar.url?.startsWith('emoji:')) {
    return (
      <span
        className={cn('inline-flex shrink-0 items-center justify-center leading-none', box, emoji)}
      >
        {avatar.url.slice(6)}
      </span>
    )
  }
  if (avatar.url) {
    return (
      <img
        src={avatar.url}
        alt={name}
        draggable={false}
        className={cn('shrink-0 rounded-sm object-cover', box)}
      />
    )
  }
  return (
    <span
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-sm px-0.5 font-bold text-primary-foreground',
        box,
        text,
        avatar.color,
      )}
    >
      {avatar.label}
    </span>
  )
}
