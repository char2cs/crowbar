import { RepoAvatarImg } from './repo-avatar'
import { IconPopover } from './icon-popover'
import { useHostResolved } from '@/lib/host-resolve'
import { repoBase } from '@/lib/project-url'
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

// The repo home row (where the icon is set) is the reference every surface matches: rounded-md,
// px-1, and an emoji one step larger than the letter tile's text.
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

// A repo's mark, read-only: emoji, then uploaded image, then its letter tile. One component for
// every repo surface (like ProjectIconMark) so the row, pill and New Tab heading never disagree.
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

interface EditableRepoIconProps {
  repo: RepoIconSource
  projectId: string
  repoId: string
  size: keyof typeof SIZES
}

// The repo home row's click-to-edit icon: supplies IconPopover's REST base and splits
// `avatarURL` into `emoji`/`iconUrl`. RepoIconMark draws every state, previews included, since the
// fallbacks are reached only when its own letter-tile default applies anyway.
export function EditableRepoIcon({ repo, projectId, repoId, size }: EditableRepoIconProps) {
  const hostResolved = useHostResolved({ projectId })
  // Nothing to edit against until the project's host is known.
  if (!hostResolved) return <RepoIconMark repo={repo} size={size} />
  const isEmoji = repo.avatarURL?.startsWith(EMOJI_PREFIX)
  return (
    <IconPopover
      base={repoBase(projectId, repoId)}
      name={repo.name}
      emoji={isEmoji ? repo.avatarURL!.slice(EMOJI_PREFIX.length) : undefined}
      iconUrl={isEmoji ? undefined : repo.avatarURL}
      fallback={<RepoIconMark repo={repo} size={size} />}
      fallbackLarge={<RepoIconMark repo={repo} size="xl" />}
      trigger={(version) => <RepoIconMark repo={repo} size={size} version={version} />}
      github
    />
  )
}
