import { IconPopover } from './icon-popover'
import { RepoIconMark } from './repo-icon-mark'
import { WorkspaceAgentSpinner } from './workspace-branch-icon'
import { cn } from '@/lib/utils'
import type { Repo } from '@/lib/store/sidebar'

/**
 * The repo avatar, as an editable icon.
 *
 * Everything but the default lives in IconPopover, which the project row shares:
 * the daemon exposes the same four icon routes under a repo and under a project,
 * so the only things that differ are the REST base, what "no icon" draws, and
 * whether the GitHub owner-avatar button is on the menu.
 *
 * A repo's default is the generated letter tile — its initial over the colour
 * derived from its name. A project's is the Library glyph; see
 * project-icon-popover.tsx.
 *
 * While an agent turn is running the avatar is replaced by the spinner and is
 * not editable, so the row keeps its navigation click.
 */
export function RepoIconPopover({ repo }: { repo: Repo }) {
  // `avatarURL` carries BOTH shapes: an `emoji:<char>` marker or a real proxy
  // URL. Splitting them here is what lets IconPopover take two plain props.
  const isEmoji = repo.avatarURL?.startsWith('emoji:')
  const emoji = isEmoji ? repo.avatarURL!.slice(6) : undefined
  const iconUrl = isEmoji ? undefined : repo.avatarURL

  const letterTile = (size: string) => (
    <span
      className={cn(
        'inline-flex items-center justify-center rounded-md px-1 font-bold text-primary-foreground',
        size,
        repo.avatarColor,
      )}
    >
      {repo.avatarLabel}
    </span>
  )

  if (repo.defaultWorking) {
    // pointer-events-none lets the click fall through to the row (navigate).
    return (
      <span className="pointer-events-none inline-flex h-5 w-5 shrink-0 items-center justify-center">
        <WorkspaceAgentSpinner />
      </span>
    )
  }

  return (
    <IconPopover
      base={`/v0/projects/${repo.projectId ?? ''}/repos/${repo.id}`}
      name={repo.name}
      emoji={emoji}
      iconUrl={iconUrl}
      // The row, the context pill and the New Tab heading draw ONE component,
      // so a repo with an emoji icon cannot show the emoji here and a letter
      // tile in the pill directly above it.
      trigger={(version) => <RepoIconMark repo={repo} size="lg" version={version} />}
      fallback={letterTile('h-5 w-5 text-[11px]')}
      fallbackLarge={letterTile('size-14 rounded-xl text-sm')}
      github
    />
  )
}
