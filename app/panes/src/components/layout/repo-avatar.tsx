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

// An icon <img> that renders the caller's fallback instead of a broken-image glyph when the URL
// fails (a reset racing the WS frame, a stale URL); resets whenever the src changes.
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
      // `block`: an inline <img> aligns to the text baseline instead of centring in the flex row.
      className={cn('block', className)}
      onError={() => setErrored(true)}
    />
  )
}

export function RepoAvatarImg(props: RepoAvatarImgProps) {
  // Reset by key: a new src remounts the attempt with `errored` false (an effect would show the
  // fallback for a render). Keyed here so the self-reset is this component's contract.
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
