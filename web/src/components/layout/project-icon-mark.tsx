import type { ReactNode } from 'react'
import { BooksIcon } from '@phosphor-icons/react'
import { RepoAvatarImg } from './repo-avatar'
import { IconPopover } from './icon-popover'
import { useHostResolved } from '@/lib/host-resolve'
import { projectAssetURL, projectBase } from '@/lib/project-url'
import { cn } from '@/lib/utils'

/** What a project carries for its mark. Both custom states are optional; when
 *  neither is set the mark is the Library glyph. An uploaded image names its
 *  project's `id`: that is what places its URL on the project's host. */
export type ProjectIconSource = {
  name: string
  avatarEmoji?: string
} & ({ id: string; avatarUrl?: string } | { id?: string; avatarUrl?: undefined })

// Sized the way repo-avatar.tsx sizes its marks — by class, not by an inline
// style — so the two stay comparable. `lg` is the sidebar row's 20px mark; `md`
// is the context pill, which sits under a smaller type stack.
const SIZES = {
  md: { box: 'size-4', emoji: 'text-sm', glyph: 14 },
  lg: { box: 'size-5', emoji: 'text-lg', glyph: 16 },
  // Headline size, matching RepoAvatar's own `xl`: on the New Tab surface the
  // repo/branch stack is the largest type in the pane, and a 20px mark beside it
  // reads as a stray favicon.
  xl: { box: 'size-6', emoji: 'text-xl', glyph: 24 },
}

interface ProjectIconMarkProps {
  project: ProjectIconSource
  size: keyof typeof SIZES
  /**
   * Cache-buster for surfaces that can also CHANGE the icon, i.e. the popover
   * trigger. It stacks with the `?v=<AvatarVersion>` the daemon already bakes
   * into avatarUrl (dto/project.go), giving `?v=2&v=1` — deliberate: the daemon's
   * copy only moves once the updated ProjectDTO has come back over the WS
   * stream, and this one moves the instant the upload resolves. Read-only
   * surfaces omit it and ride the daemon's.
   */
  version?: number
}

/**
 * A project's mark, read-only: emoji, then uploaded image, then the Library
 * default — the same precedence the daemon enforces (setting an emoji clears a
 * stored image and vice versa).
 *
 * This exists so the mark is defined once and every surface showing a project
 * agrees on it. The context pill used to hardcode its glyph, so a project with
 * an icon showed that icon in the sidebar and the default glyph in the pill
 * directly above it.
 */
export function ProjectIconMark({ project, size, version }: ProjectIconMarkProps) {
  const { box, emoji, glyph } = SIZES[size]

  if (project.avatarEmoji) {
    return (
      <span className={cn('inline-flex items-center justify-center leading-none', box, emoji)}>
        {project.avatarEmoji}
      </span>
    )
  }

  const fallback = (
    <span className={cn('inline-flex items-center justify-center text-foreground', box)}>
      <BooksIcon size={glyph} />
    </span>
  )

  if (!project.avatarUrl) return fallback

  return (
    <ProjectIconImage
      projectId={project.id}
      avatarUrl={project.avatarUrl}
      name={project.name}
      version={version}
      className={cn('rounded-md object-cover', box)}
      fallback={fallback}
    />
  )
}

interface ProjectIconImageProps {
  projectId: string
  avatarUrl: string
  name: string
  version?: number
  className: string
  fallback: ReactNode
}

// A host that is not resolved yet gets the default mark: the URL is built from the project's
// own host during render, and the local daemon's is never the answer for a remote project.
function ProjectIconImage({
  projectId,
  avatarUrl,
  name,
  version,
  className,
  fallback,
}: ProjectIconImageProps) {
  const hostResolved = useHostResolved({ projectId })
  if (!hostResolved) return fallback

  // Resolved for the BROWSER, not for apiFetch: a ProjectDTO's icon URL is a
  // bare `/v0/...` path and the webview's origin is the dev server or the app
  // bundle, never the daemon. Unresolved it 404s and silently degrades to the
  // Library glyph — an uploaded icon then looks exactly like no icon at all.
  const url = projectAssetURL(projectId, avatarUrl)
  const src = version === undefined ? url : `${url}${url.includes('?') ? '&' : '?'}v=${version}`

  return <RepoAvatarImg src={src} alt={name} className={className} fallback={fallback} />
}

interface EditableProjectIconProps {
  project: ProjectIconSource & { id: string }
  size: keyof typeof SIZES
  /** Forwarded to IconPopover — see its own doc. */
  onOpenChange?: (open: boolean) => void
}

/**
 * The space header's own icon, click-to-edit — mirrors EditableRepoIcon
 * (repo-icon-mark.tsx). The daemon's four icon routes are identical under a
 * project, minus the GitHub owner-avatar button: a project has no origin
 * remote to read one from.
 */
export function EditableProjectIcon({ project, size, onOpenChange }: EditableProjectIconProps) {
  const hostResolved = useHostResolved({ projectId: project.id })
  // Nothing to edit against until the project's host is known.
  if (!hostResolved) return <ProjectIconMark project={project} size={size} />
  return (
    <IconPopover
      base={projectBase(project.id)}
      name={project.name}
      emoji={project.avatarEmoji}
      iconUrl={project.avatarUrl ? projectAssetURL(project.id, project.avatarUrl) : undefined}
      fallback={<ProjectIconMark project={project} size={size} />}
      fallbackLarge={<ProjectIconMark project={project} size="xl" />}
      trigger={(version) => <ProjectIconMark project={project} size={size} version={version} />}
      onOpenChange={onOpenChange}
    />
  )
}
