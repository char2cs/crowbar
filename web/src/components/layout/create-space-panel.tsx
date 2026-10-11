import { useId, useState } from 'react'
import { FolderOpen } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { CreateSpaceDaemonRow } from './create-space-daemon-row'
import { ProjectIconMark } from './project-icon-mark'
import { IconPopover, type StagedIcon } from './icon-popover'
import { putIconFile } from './icon-upload'
import { getFilenameFromPath } from '@/features/file-system/controllers/file-utils'
import { toast } from '@/features/window/stores/toast-store'
import { toastActionFailure, toastHostFailure } from '@/features/hosts/lib/host-failure-notice'
import { apiFetch, postProject } from '@/lib/api'
import { awaitEntity } from '@/lib/ws/await-entity'
import { qualify } from '@/lib/host-path'
import { projectBaseOnHost } from '@/lib/project-url'
import { projectFromDTO } from '@/lib/store/project-from-dto'
import { hostIsUnreachable } from '@/features/hosts/components/host-status-dot'
import { useDaemonFolderPicker } from '@/features/hosts/hooks/use-daemon-folder-picker'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { LOCAL_HOST, type HostDotState } from '@/features/hosts/types'
import type { Project, ProjectDTO } from '@/lib/types'

/** Why Create waits on a remote host, or null when it doesn't: the host first, then the folder. */
function remoteCreateHint(
  { name, state }: { name: string | undefined; state: HostDotState },
  selectedPath: string,
): string | null {
  if (name === undefined) return null
  if (state === 'needs-repair') return `${name} needs pairing again.`
  if (hostIsUnreachable(state)) return `${name} is not answering.`
  return selectedPath ? null : `Choose a folder on ${name} first.`
}

interface CreateSpacePanelProps {
  onCreate: (project: Project) => void
  onCancel: () => void
}

/** Applies a staged icon pick to the just-created project, on the three routes IconPopover would
 *  have hit. It is not in the project list yet, so the host it was created on is passed in. */
async function applyStagedIcon(
  hostId: string,
  projectId: string,
  staged: StagedIcon,
): Promise<void> {
  const base = projectBaseOnHost(hostId, projectId)
  switch (staged.kind) {
    case 'emoji':
      await apiFetch(`${base}/icon/emoji`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emoji: staged.emoji }),
      })
      return
    case 'path':
      await putIconFile(`${base}/icon`, staged.path)
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

// Local keeps its one line; for a remote host the reason (too large, outside what this app may
// read) is the user's only clue.
function toastIconFailure(hostId: string, err: unknown): void {
  if (toastHostFailure(err)) return
  if (hostId === LOCAL_HOST.id) toast.error('Failed to set icon')
  else toast.error('Failed to set icon', err instanceof Error ? err.message : String(err))
}

/** The "Create a Space" sheet, replacing SpaceScroller so a swipe never lands on it. Its icon
 *  pick is staged (no project id yet) and applied right after the create answers, before
 *  `onCreate`: the new space arrives with its icon already set. */
export function CreateSpacePanel({ onCreate, onCancel }: CreateSpacePanelProps) {
  const [name, setName] = useState('')
  const [folder, setFolder] = useState<{ hostId: string; path: string } | null>(null)
  const [stagedIcon, setStagedIcon] = useState<StagedIcon | null>(null)
  const [loading, setLoading] = useState(false)
  const [hostId, setHostId] = useState('local')

  const stagedEmoji = stagedIcon?.kind === 'emoji' ? stagedIcon.emoji : undefined
  const stagedIconUrl =
    stagedIcon?.kind === 'path' || stagedIcon?.kind === 'file' ? stagedIcon.previewUrl : undefined
  const selectedHost = useHostView(hostId)
  // Local keeps today's flow: only a remote that cannot be reached stops a create.
  const hostRefusesCreate = hostId !== LOCAL_HOST.id && hostIsUnreachable(selectedHost.state)
  // A folder names a path on the daemon it was picked on, and on no other.
  const selectedPath = folder?.hostId === hostId ? folder.path : ''
  const folderPicker = useDaemonFolderPicker(hostId, (path, pickedOn) =>
    setFolder({ hostId: pickedOn, path }),
  )
  const createHintId = useId()
  const createHint = hostId === LOCAL_HOST.id ? null : remoteCreateHint(selectedHost, selectedPath)

  async function handleCreate() {
    if (!selectedPath || loading || hostRefusesCreate) return
    setLoading(true)
    try {
      // postProject answers 202 with no body: the project is read back off the stream of the
      // host it is created on, matched by path, so the DTO stamped with that host came from it.
      const trimmedName = name.trim() || getFilenameFromPath(selectedPath)
      const dto = await awaitEntity<ProjectDTO>({
        endpoint: qualify(hostId, '/v0/projects'),
        match: (p) => p.path === selectedPath && p.status !== 'deleted',
        action: () => postProject(trimmedName, selectedPath, hostId),
      })
      if (stagedIcon) {
        // Best-effort: the space itself was created successfully, so a
        // failed icon mutation surfaces its own toast rather than blocking
        // entry to the new space.
        try {
          await applyStagedIcon(hostId, dto.id, stagedIcon)
        } catch (err) {
          toastIconFailure(hostId, err)
        }
      }
      onCreate(projectFromDTO(dto, hostId))
    } catch (err) {
      toastActionFailure(err, 'Failed to create space')
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

      <CreateSpaceDaemonRow hostId={hostId} onHostIdChange={setHostId} />

      <button
        type="button"
        onClick={folderPicker.pick}
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

      {folderPicker.picker}

      <div className="flex-1" />

      <div className="flex shrink-0 flex-col gap-2 pb-4">
        {createHint && (
          <p id={createHintId} className="text-center text-muted-foreground text-xs">
            {createHint}
          </p>
        )}
        <Button
          onClick={() => void handleCreate()}
          disabled={!selectedPath || loading || hostRefusesCreate}
          aria-describedby={createHint ? createHintId : undefined}
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
