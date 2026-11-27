import type { ReactNode } from 'react'
import { BooksIcon } from '@phosphor-icons/react'
import { RepoAvatarImg } from './repo-avatar'
import { IconPopover } from './icon-popover'
import { useHostResolved } from '@/lib/host-resolve'
import { projectAssetURL, projectBase } from '@/lib/project-url'
import { cn } from '@/lib/utils'

type ProjectIconSource = {
  name: string
  avatarEmoji?: string
} & ({ id: string; avatarUrl?: string } | { id?: string; avatarUrl?: undefined })

const SIZES = {
  md: { box: 'size-4', emoji: 'text-sm', glyph: 14 },
  lg: { box: 'size-5', emoji: 'text-lg', glyph: 16 },
  xl: { box: 'size-6', emoji: 'text-xl', glyph: 24 },
}

interface ProjectIconMarkProps {
  project: ProjectIconSource
  size: keyof typeof SIZES
  version?: number
}

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

function ProjectIconImage({
  projectId,
  avatarUrl,
  name,
  version,
  className,
  fallback,
}: ProjectIconImageProps) {
  const hostResolved = useHostResolved({ projectId })
  if (avatarUrl.startsWith('data:')) {
    return <RepoAvatarImg src={avatarUrl} alt={name} className={className} fallback={fallback} />
  }
  if (!hostResolved) return fallback

  const url = projectAssetURL(projectId, avatarUrl)
  const src = version === undefined ? url : `${url}${url.includes('?') ? '&' : '?'}v=${version}`

  return <RepoAvatarImg src={src} alt={name} className={className} fallback={fallback} />
}

interface EditableProjectIconProps {
  project: ProjectIconSource & { id: string }
  size: keyof typeof SIZES
}

export function EditableProjectIcon({ project, size }: EditableProjectIconProps) {
  const hostResolved = useHostResolved({ projectId: project.id })
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
    />
  )
}
