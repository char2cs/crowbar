import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'

export type RepoAvatarData = { url?: string; label: string; color: string }

const sizeClasses = {
  sm: { box: 'h-4 w-4', text: 'text-[10px]', emoji: 'text-xs' },
  lg: { box: 'h-5 w-5', text: 'text-[11px]', emoji: 'text-sm' },
  // Headline size — the New Tab surface, where the repo/branch stack is the
  // largest type on the pane and a 20px mark reads as a stray favicon next to it.
  xl: { box: 'h-6 w-6', text: 'text-[13px]', emoji: 'text-lg' },
}

// Icon <img> with graceful degradation: when the icon URL 404s (icon reset
// racing the WS frame, stale URL after a daemon wipe) render the caller's
// letter/color fallback instead of the browser's broken-image glyph. The
// error state resets whenever the src changes (e.g. a new ?v= version).
export function RepoAvatarImg({
  src,
  alt,
  className,
  fallback,
}: {
  src: string
  alt: string
  className?: string
  fallback: React.ReactNode
}) {
  const [errored, setErrored] = useState(false)
  // Accepted (no-reset-all-state-on-prop-change): "all state" here is one boolean.
  // The self-reset-on-src-change is this component's public contract — both
  // callers (RepoAvatar, workspace-tree) and the covering test rerender with a
  // new src and expect a fresh <img> attempt WITHOUT keying the element; pushing
  // a key= onto every caller would externalize a single-field reset (task-21).
  // react-doctor-disable-next-line no-reset-all-state-on-prop-change
  useEffect(() => {
    setErrored(false)
  }, [src])
  if (errored) return <>{fallback}</>
  return (
    <img
      src={src}
      alt={alt}
      draggable={false}
      className={className}
      onError={() => setErrored(true)}
    />
  )
}

export function RepoAvatar({
  avatar,
  name,
  size = 'sm',
}: {
  avatar: RepoAvatarData
  name: string
  size?: 'sm' | 'lg' | 'xl'
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
  const letterFallback = (
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
  if (avatar.url) {
    return (
      <RepoAvatarImg
        src={avatar.url}
        alt={name}
        className={cn('shrink-0 rounded-sm object-cover', box)}
        fallback={letterFallback}
      />
    )
  }
  return letterFallback
}
