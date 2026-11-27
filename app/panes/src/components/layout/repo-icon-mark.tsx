import { RepoAvatarImg } from './repo-avatar'
import { IconPopover } from './icon-popover'
import { useHostResolved } from '@/lib/host-resolve'
import { repoBase } from '@/lib/project-url'
import { cn } from '@/lib/utils'

export interface RepoIconSource {
  name: string
  avatarURL?: string
  avatarLabel: string
  avatarColor: string
}

const EMOJI_PREFIX = 'emoji:'

const SIZES = {
  sm: { box: 'size-4', text: 'text-[10px]', emoji: 'text-base' },
  lg: { box: 'size-5', text: 'text-[11px]', emoji: 'text-lg' },
  xl: { box: 'size-6', text: 'text-[13px]', emoji: 'text-xl' },
}

interface RepoIconMarkProps {
  repo: RepoIconSource
  size: keyof typeof SIZES
  version?: number
}

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

export function EditableRepoIcon({ repo, projectId, repoId, size }: EditableRepoIconProps) {
  const hostResolved = useHostResolved({ projectId })
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
