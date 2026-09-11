import { useState } from 'react'
import { FolderOpen } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ProjectIconMark } from './project-icon-mark'
import { IconPopover, type StagedIcon } from './icon-popover'
import { getFilenameFromPath } from '@/features/file-system/controllers/file-utils'
import { toast } from '@/features/window/stores/toast-store'
import { openNativeDialog as openDialog } from '@/lib/native-dialog'
import { apiFetch, postProject } from '@/lib/api'
import { awaitEntity } from '@/lib/ws/await-entity'
import { projectFromDTO } from '@/lib/store/project-from-dto'
import type { Project, ProjectDTO } from '@/lib/types'

interface CreateSpacePanelProps {
  onCreate: (project: Project) => void
  onCancel: () => void
}

/** Applies a staged icon pick against a real, just-created project — the
 *  exact same three routes IconPopover itself would have hit had the space
 *  already existed when the pick was made. */
async function applyStagedIcon(projectId: string, staged: StagedIcon): Promise<void> {
  const base = `/v0/projects/${projectId}`
  switch (staged.kind) {
    case 'emoji':
      await apiFetch(`${base}/icon/emoji`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emoji: staged.emoji }),
      })
      return
    case 'path':
      await apiFetch(`${base}/icon`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: staged.path }),
      })
      return
    case 'file': {
      const form = new FormData()
      form.append('icon', staged.file)
      await apiFetch(`${base}/icon`, { method: 'PUT', body: form })
      return
    }
    case 'reset':
      // A brand-new project has no icon set yet — nothing to reset.
      return
  }
}

/**
 * Zen Browser's "Create a Space" sheet (spec: same name+icon row, a
 * folder-picker row standing in for its Profile row, no theme row), reached
 * only via the `+` mark in SidebarFooter — it replaces SpaceScroller
 * entirely rather than joining its scroll-snap track, so the touchpad swipe
 * between real spaces never lands here by accident.
 *
 * The icon square is the real `IconPopover` — same Upload/Emoji/Reset sheet
 * every other project and repo icon opens — in its `onStage` mode: a pick is
 * held locally (`stagedIcon`) and fed straight back in as `emoji`/`iconUrl`
 * for the live preview, since there is no project id yet to mutate against.
 * `applyStagedIcon` persists it for real right after `postProject` answers,
 * before `onCreate` fires — the new space arrives with its icon already set,
 * not as a follow-up edit.
 */
export function CreateSpacePanel({ onCreate, onCancel }: CreateSpacePanelProps) {
  const [name, setName] = useState('')
  const [selectedPath, setSelectedPath] = useState('')
  const [stagedIcon, setStagedIcon] = useState<StagedIcon | null>(null)
  const [loading, setLoading] = useState(false)

  const stagedEmoji = stagedIcon?.kind === 'emoji' ? stagedIcon.emoji : undefined
  const stagedIconUrl =
    stagedIcon?.kind === 'path' || stagedIcon?.kind === 'file' ? stagedIcon.previewUrl : undefined

  async function handleBrowse() {
    const selected = await openDialog({ directory: true, multiple: false })
    if (typeof selected === 'string') setSelectedPath(selected)
  }

  async function handleCreate() {
    if (!selectedPath || loading) return
    setLoading(true)
    try {
      // Same subscribe-before-POST shape as the retired ImportProjectModal:
      // postProject answers 202 with no body, so the real Project (with its
      // daemon-assigned id) is read back off the /v0/projects WS stream,
      // matched by the path we just submitted.
      const trimmedName = name.trim() || getFilenameFromPath(selectedPath)
      const dto = await awaitEntity<ProjectDTO>({
        endpoint: '/v0/projects',
        match: (p) => p.path === selectedPath && p.status !== 'deleted',
        action: () => postProject(trimmedName, selectedPath),
      })
      if (stagedIcon) {
        // Best-effort: the space itself was created successfully, so a
        // failed icon mutation surfaces its own toast rather than blocking
        // entry to the new space.
        try {
          await applyStagedIcon(dto.id, stagedIcon)
        } catch {
          toast.error('Failed to set icon')
        }
      }
      onCreate(projectFromDTO(dto))
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create space')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div
      data-testid="create-space-panel"
      className="flex min-h-0 flex-1 flex-col gap-5 overflow-hidden px-3 pt-4"
    >
      <div>
        <h2 className="font-semibold text-base text-foreground">Create a Space</h2>
        <p className="mt-1 text-muted-foreground text-sm">
          Spaces are used to organize your tabs and sessions.
        </p>
      </div>

      <div className="flex items-center gap-2.5">
        <div className="flex size-9 shrink-0 items-center justify-center rounded-lg border border-border border-dashed">
          <IconPopover
            name={name || 'Space'}
            emoji={stagedEmoji}
            iconUrl={stagedIconUrl}
            fallback={
              <ProjectIconMark
                project={{ name: name || 'Space', avatarUrl: undefined, avatarEmoji: undefined }}
                size="lg"
              />
            }
            fallbackLarge={
              <ProjectIconMark
                project={{ name: name || 'Space', avatarUrl: undefined, avatarEmoji: undefined }}
                size="xl"
              />
            }
            onStage={setStagedIcon}
          />
        </div>
        <Input
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Space Name"
          autoFocus
        />
      </div>

      <button
        type="button"
        onClick={() => void handleBrowse()}
        className="flex items-center justify-between rounded-lg border border-border bg-muted px-3 py-2.5 text-sm hover:bg-sidebar-element-hover"
      >
        <span className="flex items-center gap-2 text-foreground">
          <FolderOpen size={16} className="text-muted-foreground" />
          Folder
        </span>
        <span className="min-w-0 truncate text-muted-foreground">
          {selectedPath ? getFilenameFromPath(selectedPath) : 'Choose…'}
        </span>
      </button>

      <div className="flex-1" />

      <div className="flex shrink-0 flex-col gap-2 pb-4">
        <Button
          onClick={() => void handleCreate()}
          disabled={!selectedPath || loading}
          className="w-full"
        >
          {loading ? 'Creating…' : 'Create Space'}
        </Button>
        <Button variant="ghost" onClick={onCancel} className="w-full">
          Cancel
        </Button>
      </div>
    </div>
  )
}
