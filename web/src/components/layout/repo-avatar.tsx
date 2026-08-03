import { useState } from 'react'
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
interface RepoAvatarImgProps {
  src: string
  alt: string
  className?: string
  fallback: React.ReactNode
}

function RepoAvatarImgAttempt({ src, alt, className, fallback }: RepoAvatarImgProps) {
  const [errored, setErrored] = useState(false)
  if (errored) return <>{fallback}</>
  return (
    <img
      src={src}
      alt={alt}
      draggable={false}
      className={className}
      data-oracle-id="repo-avatar"
      onError={() => setErrored(true)}
    />
  )
}

export function RepoAvatarImg(props: RepoAvatarImgProps) {
  // Reset-by-key, the canonical form: a new src remounts the attempt and its
  // `errored` flag starts false again. This used to be an effect that reset the
  // flag on [src], which showed the fallback for one render after the src
  // changed. Keyed HERE rather than at the call sites so the self-reset stays
  // this component's own contract — both callers just rerender with a new src.
  return <RepoAvatarImgAttempt key={props.src} {...props} />
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
        data-oracle-id="repo-avatar"
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
      data-oracle-id="repo-avatar"
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
