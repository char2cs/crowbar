import { Library } from 'lucide-react'
import { IconPopover } from './icon-popover'
import { WorkspaceAgentSpinner } from './workspace-branch-icon'
import { assetURL } from '@/lib/api'

interface ProjectIconPopoverProps {
  project: { id: string; name: string; avatarUrl?: string; avatarEmoji?: string }
  /** An agent turn is running in project home — the mark yields to the spinner. */
  working?: boolean
}

/**
 * The project row's mark, as an editable icon — the same mechanism the repo
 * avatar has always had, on the same four daemon routes, through the shared
 * IconPopover.
 *
 * The default is the Library glyph rather than a generated letter tile: a
 * project is a shelf of repos, and a roof would say "dwelling" where a shelf of
 * spines says "collection". That is also why there is no avatarLabel/avatarColor
 * pair on a project the way there is on a repo — "no icon" is a mark the client
 * already knows how to draw, so the daemon has nothing to store for it.
 */
export function ProjectIconPopover({ project, working = false }: ProjectIconPopoverProps) {
  if (working) {
    // pointer-events-none lets the click fall through to the row (navigate).
    return (
      <span className="pointer-events-none inline-flex size-5 shrink-0 items-center justify-center text-foreground">
        <WorkspaceAgentSpinner />
      </span>
    )
  }

  return (
    <IconPopover
      base={`/v0/projects/${project.id}`}
      name={project.name}
      emoji={project.avatarEmoji}
      // Resolved for the BROWSER, not for apiFetch. A ProjectDTO's icon URL is a
      // bare `/v0/...` path, and the webview's own origin is the dev server or
      // the app bundle — never the daemon. Unresolved, the <img> 404s and falls
      // back to the Library glyph, so an uploaded icon looks exactly like no
      // icon at all. The repo avatar goes through the same helper in
      // build-repo-tree.ts; a project has no DTO→domain mapper to do it in.
      iconUrl={project.avatarUrl ? assetURL(project.avatarUrl) : undefined}
      fallback={
        <span className="inline-flex size-5 items-center justify-center text-foreground">
          <Library size={16} />
        </span>
      }
      fallbackLarge={<Library size={28} className="text-foreground" />}
    />
  )
}
