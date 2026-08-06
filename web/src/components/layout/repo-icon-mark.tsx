import { RepoAvatarImg } from './repo-avatar'
import { cn } from '@/lib/utils'

/** What a repo carries for its mark. `avatarURL` holds BOTH custom shapes: an
 *  `emoji:<char>` marker or a real proxy URL. The letter tile is the default. */
export interface RepoIconSource {
  name: string
  avatarURL?: string
  avatarLabel: string
  avatarColor: string
}

/** The `avatarURL` prefix that means "this is an emoji, not a URL". */
const EMOJI_PREFIX = 'emoji:'

// The REPO HOME ROW is the reference: it is where a repo's icon is set, so it is
// the rendering every other surface has to match. Its values — rounded-md, px-1,
// and an emoji a full step larger than the letter tile's text — are what the old
// RepoAvatar disagreed with (rounded-sm, px-0.5, text-sm), which is how the same
// repo came out a different size and shape in the pill than on its own row.
const SIZES = {
  sm: { box: 'size-4', text: 'text-[10px]', emoji: 'text-base' },
  lg: { box: 'size-5', text: 'text-[11px]', emoji: 'text-lg' },
  // Headline size — the New Tab surface, where the repo/branch stack is the
  // largest type in the pane and a 20px mark reads as a stray favicon.
  xl: { box: 'size-6', text: 'text-[13px]', emoji: 'text-xl' },
}

interface RepoIconMarkProps {
  repo: RepoIconSource
  size: keyof typeof SIZES
  /** Cache-buster for the surface that can also CHANGE the icon (the popover
   *  trigger); see the same prop on ProjectIconMark. */
  version?: number
}

/**
 * A repo's mark, read-only: emoji, then uploaded image, then the generated
 * letter tile — its initial over the colour derived from its name.
 *
 * One component for every surface that shows a repo, for the same reason
 * ProjectIconMark exists. The row drew this through IconPopover, which takes
 * `emoji` and `iconUrl` as separate props, while the context pill and the New
 * Tab heading drew it through RepoAvatar, which encodes the emoji INSIDE the url
 * as an `emoji:` sentinel. Two representations of one three-state mark meant a
 * repo with an emoji icon showed the emoji on its own row and a letter tile in
 * the pill directly above it.
 */
export function RepoIconMark({ repo, size, version }: RepoIconMarkProps) {
  const { box, text, emoji } = SIZES[size]

  if (repo.avatarURL?.startsWith(EMOJI_PREFIX)) {
    return (
      <span
        className={cn('inline-flex shrink-0 items-center justify-center leading-none', box, emoji)}
      >
        {repo.avatarURL.slice(EMOJI_PREFIX.length)}
      </span>
    )
  }

  const letterTile = (
    <span
      className={cn(
        'inline-flex shrink-0 items-center justify-center rounded-md px-1 font-bold text-primary-foreground',
        box,
        text,
        repo.avatarColor,
      )}
    >
      {repo.avatarLabel}
    </span>
  )

  if (!repo.avatarURL) return letterTile

  // The proxy URL is stable, so re-uploading leaves `src` untouched and a
  // mounted <img> never refetches. Only the surface that can change the icon
  // passes a version; read-only ones ride the daemon's own `?v=`.
  const src =
    version === undefined
      ? repo.avatarURL
      : `${repo.avatarURL}${repo.avatarURL.includes('?') ? '&' : '?'}v=${version}`

  return (
    <RepoAvatarImg
      src={src}
      alt={repo.name}
      className={cn('shrink-0 rounded-md object-cover', box)}
      fallback={letterTile}
    />
  )
}
